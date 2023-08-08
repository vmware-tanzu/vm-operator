# Get a Console Session
A console session can be helpful when VMs are not accessible through the normal network, for example, when the guest OS failed to configure the correct network settings during first boot. Follow the steps in this tutorial to get a console session to a VM using the `kubectl` command.

## Prerequisites
Have edit or owner permissions on the namespace where the problematic VM is deployed. To confirm that you have the required permissions, run the following command:

```console
$ kubectl auth can-i create webconsolerequests -n <namespace-name>
```

For more information, see [vSphere with Tanzu Identity and Access Management](https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-with-tanzu-concepts-planning/GUID-93B29112-4492-431F-958A-12323540C38D.html).

## Procedure

### 1. Access your namespace in the Kubernetes environment.

```console
$ kubectl config use-context <context-name>
```

See [Get and Use the Supervisor Context](https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-with-tanzu-services-workloads/GUID-63A1C273-DC75-420B-B7FD-47CB25A50A2C.html#GUID-63A1C273-DC75-420B-B7FD-47CB25A50A2C) if you need help accessing Supervisor clusters.

### 2. Verify that the VM is deployed.

```console
$ kubectl get vm -n <namespace-name>
```

The output is similar to the following:

```console
NAME      POWERSTATE        AGE
vm-name   poweredOn         175m
```

### 3. Obtain the URL to the VM web console.

```console
$ kubectl vsphere vm web-console vm-name -n <namespace-name>
```

Use `--short` to get only the URL as output, or `-v/--verbose <log-level>` to get more information about the command execution.

!!! note "Link Expiration"
    The command returns an authenticated URL to the VM's web console as output. If you don't use the URL within a non-changeable period of time, set to two minutes, the URL expires. After you open the URL to connect to the web console page, the session time is controlled by WebMKS and lasts longer.

### 4. Click the URL and perform any necessary troubleshooting actions for your VM.
