# FUSE device plugin for Kubernetes

This plugin allows the mount of a FUSE device without the need for escalating privileges. 

## Usage

### Deploy as Daemon Set:

```
kubectl apply -f infra/k8s-plugin/manifests/k8s-plugin.yml
```

### Deploy

Add resource limits to your pod:

```yaml
spec: 
  containers:
  - ...
    resources:
      limits:
        sandbox0.ai/fuse: 1
```

## Acknowledgements

This project is based on this [FUSE device plugin](https://github.com/kuberenetes-learning-group/fuse-device-plugin)

## Similar projects 

* https://github.com/kuberenetes-learning-group/fuse-device-plugin
* https://gitlab.com/arm-research/smarter/smarter-device-manager
* https://github.com/squat/generic-device-plugin
