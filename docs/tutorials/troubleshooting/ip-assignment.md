# IP Assignment
This page describes how to troubleshoot when VM was created and powered on but was stuck in the status with no valid IP addresses.

## Procedure

### 1. Access your namespace in the Kubernetes environment.

```console
$ kubectl config use-context <context-name>
```

See [Get and Use the Supervisor Context](https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-with-tanzu-services-workloads/GUID-63A1C273-DC75-420B-B7FD-47CB25A50A2C.html#GUID-63A1C273-DC75-420B-B7FD-47CB25A50A2C) if you need help accessing Supervisor clusters.

### 2. Check VM network settings

```console
$ kubectl describe vm <vm-name> -n <namespace-name>

Spec:
  Network Interfaces:
    Network Type:  nsx-t
```

If it's NSX-T networking, check VirtualNetworkInterface status. We expect Conditions Type to be ready and Ip Addresses return valid addresses.
**Note** When no 
```console
$ kubectl describe virtualnetworkinterfaces <vnetif-name> -n <namespace-name>

Status:
  Conditions:
    Status:      True
    Type:        Ready
  Ip Addresses:
    Gateway:      172.26.0.33
    Ip:           172.26.0.34
    Subnet Mask:  255.255.255.240
Events:
  Type    Reason                        Age   From               Message
  ----    ------                        ----  ----               -------
  Normal  SuccessfulRealizeNSXResource  25m   nsx-container-ncp  Successfully realized NSX resource for VirtualNetworkInterface
```
