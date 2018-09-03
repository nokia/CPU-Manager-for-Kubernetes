# k8s CPU device plugin

A simple poc for allocating cpus with device plugin. Uses CMK as 'backend' to manage the cpu pool.

## Build

cpu device plugin uses CMK so the cmk image has to be built first.

Building cpu device plugin image:
```
$ docker build -t cpudp .
```

## Installation

Install cmk to host filesystem:

```
docker run -v /opt/bin:/opt/bin cmk:v1.2.2 install
```

Create cpu-device-plugin daemonset:
```
kubectl create -f ./cpu-dev-ds.yaml
```
There is a helper script ```./util/generate_cert.sh``` that generates certificate and key for the cmk webhook admission controller. The script ```./util/inset-ca-bundle.sh``` can be used to insert ca bundle to the manifest for the webhook configuration.

Following steps create the webhook server with necessary configuration (including the certifcate and key)
```
$ ./util/generate-cert.sh
$ kubectl create -f ../resources/webhook/cmk-webhook-configmap.yaml
$ kubectl create -f ../resources/webhook/cmk-webhook-pod.yaml
$ kubectl create -f ../resources/webhook/cmk-webhook-service.yaml
$ ./util/insert-ca-bundle.sh resources/webhook/cmk-webhook-config.yaml | kubectl create -f -
```
The cpu-device-plugin with cmk webhook should be running now. Test the installation with a cpu test pod:
```
$ kubectl create -f ./util/cpu-test.yaml
```

## Notes / open issues
* (re)start of cpu-device-plugin
  * cmk configuration is stored in /etc/cmk. The directory must be empty before starting the plugin.

