package main

import (
	"github.com/go-yaml/yaml"
	"github.com/golang/glog"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
	"golang.org/x/net/context"
	"io/ioutil"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
	"strconv"
	"strings"
	"time"
)

type Pool struct {
	Cpus string `yaml:"cpus"`
}

type PoolConfig struct {
	Pools map[string]Pool `yaml:"pools"`
}

type cpuDeviceManager struct {
	pool Pool
}

func (cdm *cpuDeviceManager) PreStartContainer(ctx context.Context, psRqt *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (cdm *cpuDeviceManager) Start() error {
	return nil
}

func (cdm *cpuDeviceManager) Stop() error {
	return nil
}

func (cdm *cpuDeviceManager) ListAndWatch(e *pluginapi.Empty, stream pluginapi.DevicePlugin_ListAndWatchServer) error {
	var updateNeeded = true
	for {
		if updateNeeded {
			resp := new(pluginapi.ListAndWatchResponse)
			for _, cpuId := range strings.Split(cdm.pool.Cpus, ",") {
				resp.Devices = append(resp.Devices, &pluginapi.Device{cpuId, pluginapi.Healthy})
			}
			glog.Infof("ListAndWatch: send devices %v\n", resp)
			if err := stream.Send(resp); err != nil {
				glog.Errorf("Error. Cannot update device states: %v\n", err)
				return err
			}

			updateNeeded = false
		}
		//TODO: When is update needed ?
		time.Sleep(10 * time.Second)
	}
	return nil

}

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
		containerResp := new(pluginapi.ContainerAllocateResponse)
		glog.Infof("CPUs allocated: %s: Num of CPUs %s", cpusAllocated[:len(cpusAllocated)-1],
			strconv.Itoa(len(container.DevicesIDs)))

		containerResp.Envs = envmap
		resp.ContainerResponses = append(resp.ContainerResponses, containerResp)
	}
	return resp, nil
}

func (cdm *cpuDeviceManager) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (l *Lister) GetResourceNamespace() string {
	return "nokia.com"
}

type Lister struct {
	poolConfig PoolConfig
}

// Monitors available resources
// CPU pools are static so return after reporting initial list of pools
func (l *Lister) Discover(pluginListCh chan dpm.PluginNameList) {
	list := dpm.PluginNameList{}
	for poolName, _ := range l.poolConfig.Pools {
		list = append(list, poolName)
	}
	pluginListCh <- list
	return
}

func (l *Lister) NewPlugin(resourceLastName string) dpm.PluginInterface {
	glog.Infof("Starting plugin for resource %d", resourceLastName)

	return &cpuDeviceManager{
		pool: l.poolConfig.Pools[resourceLastName],
	}

}

func readPoolConfig() (PoolConfig, error) {
	var pools PoolConfig
	file, err := ioutil.ReadFile("/etc/cpudp/poolconfig.yaml")
	if err != nil {
		glog.Errorf("Could not read poolconfig")
	} else {
		err = yaml.Unmarshal([]byte(file), &pools)
		if err != nil {
			glog.Errorf("Error in poolconfig file %v", err)
		}
	}
	return pools, err
}

func main() {
	poolsConf, err := readPoolConfig()
	if err != nil {
		panic("Configuration error")
	}
	lister := Lister{poolConfig: poolsConf}
	manager := dpm.NewManager(&lister)
	manager.Run()
}
