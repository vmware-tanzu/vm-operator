# IP Assignment

This page describes how to troubleshoot a situation where a VM is created and is powered on, but not getting an IP address.

## Verify the Context

Ensure the correct Kubernetes context is selected by using the following command:

```shell
kubectl config use-context <CONTEXT_NAME>
```

## Verify the Network Interfaces

Check the underlying network interface resources created on behalf of the VM:

* NSX-T (NCP)

    ```shell
    kubectl get -n <NAMESPACE> virtualnetworkinterface | grep <VM_NAME>
    ```

* NSX-T (VPC)

    ```shell
    kubectl get -n <NAMESPACE> subnetport | grep <VM_NAME>
    ```

* vSphere Distributed Switch (VDS)

    ```shell
    kubectl get -n <NAMESPACE> networkinterface | grep <VM_NAME>
    ```

Note the name of each resource and use `kubectl describe` on it.

The expected condition type is `Ready` and `IP Addresses` should contain valid addresses.

If either of these are not the case, then please contact the vSphere administrator to verify the underlying networking.

## Bootstrap

If the network and interfaces are healthy, then it is time to look at the [bootstrap providers](../../concepts/workloads/guest.md).

### Cloud-Init

For VMs deployed with Cloud-Init, a powered on VM sans IP usually indicates that Cloud-Init failed. The following steps can help troubleshoot the issue:

1. Inspect the following values for the ExtraConfig keys on the VM:

      * `guestinfo.metadata`
      * `guestinfo.userdata`

    These are the keys used by VM Operator to send information into the guest used by Cloud-Init.

2. Inspect the Cloud-Init results by logging into the VM using the web console and examining the log files located at `/var/log/cloud-init.log` and `/var/log/cloud-init-output.log`.

### Sysprep

For VMs deployed with Sysprep, a powered on VM sans IP usually indicates that guest OS customization (GOSC) failed. The following steps can help troubleshoot the issue:

1. If the `GuestCustomization` condition on the VM is `false`, it means either GOSC or sysprep failed.

2. Inspect the GOSC results by logging into the VM using the web console and examining the log files located at `C:\Windows\TEMP\vmware-imc\guestcust`.

3. Validate the sysprep answers file by logging into the VM using the web console and examining `C:\sysprep1001\sysprep.xml`. For example, there should be evidence that `<Identifier>{{ V1alpha5_FirstNicMacAddr }}</Identifier>` was converted to the actual MAC addresses, ex. `<Identifier>00-11-22-33-aa-bb-cc</Identifier>`.

4. Inspect the sysprep logs by logging into the VM using the web console and examining the log files located in `C:\Windows\Panther\setuperr`, `C:\Windows\Panther\Unattendgc\setuperr`, and `C:\Windows\System32\Sysprep\Panther\setuperr`.

### vAppConfig

For VMs deployed with vAppConfig, a powered on VM sans IP usually indicates that there are issues transmitting the data into the guest or the bespoke bootstrap engine failed. The following steps can help troubleshoot the issue:

1. If the `GuestCustomization` condition on the VM is `false`, it means GOSC failed to transmit the data into the guest.
   
2. Inspect the VM using vSphere and verify all of the templating expressions have been parsed correctly by examining the property `config.vAppConfig.property`.

3. Validate the bootstrap details were transmitted into the guest by logging into the VM using the web console and verifying the log file `/var/log/vmware-imc/toolsDeployPkg.log` contains the string `Executing Traditional GOSC workflow`.
