# Quickstart

This page walks through the quickest way to try out VM Operator!

## Requirements

Currently VM Operator is only available with VMware vSphere 7.0+ and VM Service on Supervisor. Please refer to the following documentation for getting started with Supervisor and VM Service:

* Configuring and Managing a Supervisor ([7.0](https://docs.vmware.com/en/VMware-vSphere/7.0/vmware-vsphere-with-tanzu/GUID-21ABC792-0A23-40EF-8D37-0367B483585E.html), [8.0](https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-with-tanzu-installation-configuration/GUID-21ABC792-0A23-40EF-8D37-0367B483585E.html))
* Deploying and Managing Virtual Machines ([7.0](https://docs.vmware.com/en/VMware-vSphere/7.0/vmware-vsphere-with-tanzu/GUID-F81E3535-C275-4DDE-B35F-CE759EA3B4A0.html), [8.0](https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-with-tanzu-services-workloads/GUID-F81E3535-C275-4DDE-B35F-CE759EA3B4A0.html))

The following steps will also assume there is a `Namespace` ([7.0](https://docs.vmware.com/en/VMware-vSphere/7.0/vmware-vsphere-with-tanzu/GUID-1544C9FE-0B23-434E-B823-C59EFC2F7309.html), [8.0](https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-with-tanzu-installation-configuration/GUID-1544C9FE-0B23-434E-B823-C59EFC2F7309.html)) named `my-namespace` and:

* You have have write permissions in this namespace
* There is an NSX-T network or vSphere Distributed network available to this namespace
* There is a VM Class named `small` available to this namespace
* There is a VM Image named `photon4` available to this namespace
* There is a storage class named `iscsi` available to this namespace

## Create a VM

Once you are logged into the Supervisor with, a new VM may be realized using the following YAML:

```yaml title="vm-example.yaml"
--8<-- "./docs/concepts/workloads/vm-example.yaml"
```

Create the new VM with the following command:

```shell
kubectl apply -n my-namespace -f {{ config.repo_url_raw }}/main/docs/concepts/workloads/vm-example.yaml
```

And that's it! Use `kubectl` to watch the VM until it is powered on with an IP address, at which point you have successfully deployed a workload on Kubernetes with VM Operator.
