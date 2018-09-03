package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"syscall"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	grpc "google.golang.org/grpc"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
)

const (
	pluginMountPath      = "/var/lib/kubelet/device-plugins"
	kubeletEndpoint      = "kubelet.sock"
	pluginEndpointPrefix = "cpuDevice"
)

type CpuDevIdMapType struct {
	CpuDevidMap map[string]string `json:"cpudevidmap"`
}

type cpuDeviceManager struct {
	k8ClientSet *kubernetes.Clientset
	socketFile  string
	grpcServer  *grpc.Server
	devices     CpuDevIdMapType
}

func NewCpuDeviceManager(dpCores int) *cpuDeviceManager {

	config, err := rest.InClusterConfig()
	if err != nil {
		glog.Errorf("Error. Could not get InClusterConfig to create K8s Client. %v", err)
		return nil
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Errorf("Error. Could not create K8s Client using supplied config. %v", err)
		return nil
	}
	dpCoresArg := "--num-dp-cores=" + strconv.Itoa(dpCores)
	cmd := exec.Command("/opt/bin/cmk", "init", "--conf-dir=/etc/cmk", dpCoresArg, "--num-cp-cores=1")

	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {

		glog.Errorf("CMK init finished with error: %v\n%s", err, stdoutStderr)
		return nil
	}
	glog.Infof("CMK init finished:\n%s", stdoutStderr)

	var devs CpuDevIdMapType
	devs.CpuDevidMap = make(map[string]string)

	for i := 0; i < dpCores; i++ {
		devs.CpuDevidMap[strconv.Itoa(i)] = ""
	}
	jsonOut, _ := json.MarshalIndent(devs, "", "    ")
	ioutil.WriteFile("/etc/cmk/cpudevmap.json", jsonOut, os.FileMode(0666))

	return &cpuDeviceManager{
		k8ClientSet: clientset,
		socketFile:  fmt.Sprintf("%s.sock", pluginEndpointPrefix),
		devices:     devs,
	}
}

func (cdm *cpuDeviceManager) Start() error {
	glog.Infof("Discovering CPUs")

	pluginEndpoint := filepath.Join(pluginapi.DevicePluginPath, cdm.socketFile)
	glog.Infof("Starting CPU Device Plugin server at: %s\n", pluginEndpoint)
	lis, err := net.Listen("unix", pluginEndpoint)
	if err != nil {
		glog.Errorf("Error. Starting CPU Device Plugin server failed: %v", err)
	}
	cdm.grpcServer = grpc.NewServer()

	// Register all services
	pluginapi.RegisterDevicePluginServer(cdm.grpcServer, cdm)

	go cdm.grpcServer.Serve(lis)

	// Wait for server to start by launching a blocking connection
	conn, err := grpc.Dial(pluginEndpoint, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		glog.Errorf("Error. Could not establish connection with gRPC server: %v", err)
		return err
	}
	glog.Infoln("CPU Device Plugin server started serving")
	conn.Close()
	return nil
}

func (cdm *cpuDeviceManager) Stop() error {
	glog.Infof("CPU Device Plugin gRPC server..")
	if cdm.grpcServer == nil {
		return nil
	}

	cdm.grpcServer.Stop()
	cdm.grpcServer = nil

	return cdm.cleanup()
}

// Removes existing socket if exists
// [adpoted from https://github.com/redhat-nfvpe/k8s-dummy-device-plugin/blob/master/dummy.go ]
func (cdm *cpuDeviceManager) cleanup() error {
	pluginEndpoint := filepath.Join(pluginapi.DevicePluginPath, cdm.socketFile)
	if err := os.Remove(pluginEndpoint); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func Register(kubeletEndpoint, pluginEndpoint, resourceName string) error {
	conn, err := grpc.Dial(kubeletEndpoint, grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))
	if err != nil {
		glog.Errorf("CPU Device Plugin cannot connect to Kubelet service: %v", err)
		return err
	}
	defer conn.Close()
	client := pluginapi.NewRegistrationClient(conn)

	request := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     pluginEndpoint,
		ResourceName: resourceName,
	}

	if _, err = client.Register(context.Background(), request); err != nil {
		glog.Errorf("CPU Device Plugin cannot register to Kubelet service: %v", err)
		return err
	}
	return nil
}

func (cdm *cpuDeviceManager) ListAndWatch(emtpy *pluginapi.Empty, stream pluginapi.DevicePlugin_ListAndWatchServer) error {

	var devMap CpuDevIdMapType
	updateNeeded := true
	for {
		if updateNeeded {
			resp := new(pluginapi.ListAndWatchResponse)
			for devId, cpuId := range cdm.devices.CpuDevidMap {
				if cpuId == "" {
					resp.Devices = append(resp.Devices, &pluginapi.Device{devId, pluginapi.Healthy})
				}
			}
			glog.Infof("ListAndWatch: send devices %v\n", resp)
			if err := stream.Send(resp); err != nil {
				glog.Errorf("Error. Cannot update device states: %v\n", err)
				cdm.grpcServer.Stop()
				return err
			}
			updateNeeded = false
		}
		time.Sleep(5 * time.Second)
		file, e := ioutil.ReadFile("/etc/cmk/cpudevmap.json")
		if e != nil {
			glog.Errorf("Cannot read cpudevmap.json")
		}
		json.Unmarshal([]byte(file), &devMap)
		if !reflect.DeepEqual(cdm.devices, devMap) {
			for key, value := range devMap.CpuDevidMap {
				cdm.devices.CpuDevidMap[key] = value
			}
			updateNeeded = true
		}
	}

	return nil
}

func (cdm *cpuDeviceManager) PreStartContainer(ctx context.Context, psRqt *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (cdm *cpuDeviceManager) GetDevicePluginOptions(ctx context.Context, empty *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{
		PreStartRequired: false,
	}, nil
}

//Allocate passes the CPU numbers as an env variable to the requesting container
func (cdm *cpuDeviceManager) Allocate(ctx context.Context, rqt *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	resp := new(pluginapi.AllocateResponse)
	for _, container := range rqt.ContainerRequests {
		envmap := make(map[string]string)
		cpusAllocated := ""
		for _, id := range container.DevicesIDs {
			glog.Infof("DeviceID in Allocate: %v", id)
			cpusAllocated = cpusAllocated + id + ","
		}
		envmap["CPUS"] = cpusAllocated[:len(cpusAllocated)-1]
		envmap["CMK_NUM_CORES"] = strconv.Itoa(len(container.DevicesIDs))
		containerResp := new(pluginapi.ContainerAllocateResponse)
		glog.Infof("CPUs allocated: %s: Num of CPUs %s", cpusAllocated[:len(cpusAllocated)-1],
			strconv.Itoa(len(container.DevicesIDs)))

		containerResp.Envs = envmap
		resp.ContainerResponses = append(resp.ContainerResponses, containerResp)
	}
	return resp, nil
}
func main() {
	dpcpus := flag.Int("dp-cores", 1, "Dataplane cores")
	flag.Parse()
	glog.Infof("CPU Device Plugin started...")
	cdm := NewCpuDeviceManager(*dpcpus)
	if cdm == nil {
		return
	}
	cdm.cleanup()

	// respond to syscalls for termination
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Start server
	if err := cdm.Start(); err != nil {
		glog.Errorf("cpuDeviceManager.Start() failed: %v", err)
		return
	}

	// Registers with Kubelet.
	err := Register(path.Join(pluginMountPath, kubeletEndpoint), cdm.socketFile, "nokia.com/cpupool1")
	if err != nil {
		// Stop server
		cdm.grpcServer.Stop()
		glog.Fatal(err)
		return
	}
	glog.Infof("CPU device plugin registered with the Kubelet")

	// Catch termination signals
	select {
	case sig := <-sigCh:
		glog.Infof("Received signal \"%v\", shutting down.", sig)
		cdm.Stop()
		return
	}
}
