# v1alpha4

Package v1alpha4 is one of the schemas for VM Operator.


---

## Kinds


### ClusterVirtualMachineImage



ClusterVirtualMachineImage is the schema for the clustervirtualmachineimages
API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `ClusterVirtualMachineImage`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineImageSpec](#virtualmachineimagespec)_ |  |
| `status` _[VirtualMachineImageStatus](#virtualmachineimagestatus)_ |  |

### VirtualMachine



VirtualMachine is the schema for the virtualmachines API and represents the
desired state and observed status of a virtualmachines resource.

_Appears in:_
- [VirtualMachineTemplate](#virtualmachinetemplate)

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachine`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineSpec](#virtualmachinespec)_ |  |
| `status` _[VirtualMachineStatus](#virtualmachinestatus)_ |  |

### VirtualMachineClass



VirtualMachineClass is the schema for the virtualmachineclasses API and
represents the desired state and observed status of a virtualmachineclasses
resource.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachineClass`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineClassSpec](#virtualmachineclassspec)_ |  |
| `status` _[VirtualMachineClassStatus](#virtualmachineclassstatus)_ |  |

### VirtualMachineClassInstance



VirtualMachineClassInstance is the schema for the virtualmachineclassinstances API and
represents the desired state and observed status of a virtualmachineclassinstance
resource.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachineClassInstance`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineClassInstanceSpec](#virtualmachineclassinstancespec)_ |  |
| `status` _[VirtualMachineClassInstanceStatus](#virtualmachineclassinstancestatus)_ |  |

### VirtualMachineGroup



VirtualMachineGroup is the schema for the VirtualMachineGroup API and
represents the desired state and observed status of a VirtualMachineGroup
resource.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachineGroup`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineGroupSpec](#virtualmachinegroupspec)_ |  |
| `status` _[VirtualMachineGroupStatus](#virtualmachinegroupstatus)_ |  |

### VirtualMachineImage



VirtualMachineImage is the schema for the virtualmachineimages API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachineImage`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineImageSpec](#virtualmachineimagespec)_ |  |
| `status` _[VirtualMachineImageStatus](#virtualmachineimagestatus)_ |  |

### VirtualMachineImageCache



VirtualMachineImageCache is the schema for the
virtualmachineimagecaches API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachineImageCache`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineImageCacheSpec](#virtualmachineimagecachespec)_ |  |
| `status` _[VirtualMachineImageCacheStatus](#virtualmachineimagecachestatus)_ |  |

### VirtualMachinePublishRequest



VirtualMachinePublishRequest defines the information necessary to publish a
VirtualMachine as a VirtualMachineImage to an image registry.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachinePublishRequest`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachinePublishRequestSpec](#virtualmachinepublishrequestspec)_ |  |
| `status` _[VirtualMachinePublishRequestStatus](#virtualmachinepublishrequeststatus)_ |  |

### VirtualMachineReplicaSet



VirtualMachineReplicaSet is the schema for the virtualmachinereplicasets API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachineReplicaSet`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineReplicaSetSpec](#virtualmachinereplicasetspec)_ |  |
| `status` _[VirtualMachineReplicaSetStatus](#virtualmachinereplicasetstatus)_ |  |

### VirtualMachineService



VirtualMachineService is the Schema for the virtualmachineservices API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachineService`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineServiceSpec](#virtualmachineservicespec)_ |  |
| `status` _[VirtualMachineServiceStatus](#virtualmachineservicestatus)_ |  |

### VirtualMachineSetResourcePolicy



VirtualMachineSetResourcePolicy is the Schema for the virtualmachinesetresourcepolicies API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachineSetResourcePolicy`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineSetResourcePolicySpec](#virtualmachinesetresourcepolicyspec)_ |  |
| `status` _[VirtualMachineSetResourcePolicyStatus](#virtualmachinesetresourcepolicystatus)_ |  |

### VirtualMachineSnapshot



VirtualMachineSnapshot is the schema for the virtualmachinesnapshot API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachineSnapshot`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineSnapshotSpec](#virtualmachinesnapshotspec)_ |  |
| `status` _[VirtualMachineSnapshotStatus](#virtualmachinesnapshotstatus)_ |  |

### VirtualMachineWebConsoleRequest



VirtualMachineWebConsoleRequest allows the creation of a one-time, web
console connection to a VM.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha4`
| `kind` _string_ | `VirtualMachineWebConsoleRequest`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineWebConsoleRequestSpec](#virtualmachinewebconsolerequestspec)_ |  |
| `status` _[VirtualMachineWebConsoleRequestStatus](#virtualmachinewebconsolerequeststatus)_ |  |


## Types
### DynamicDirectPathIODevice



DynamicDirectPathIODevice contains the configuration corresponding to a
Dynamic DirectPath I/O device.

_Appears in:_
- [VirtualDevices](#virtualdevices)

| Field | Description |
| --- | --- |
| `vendorID` _integer_ |  |
| `deviceID` _integer_ |  |
| `customLabel` _string_ |  |

### GroupMember



GroupMember describes a member of a VirtualMachineGroup.

_Appears in:_
- [VirtualMachineGroupSpec](#virtualmachinegroupspec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of member of this group. |
| `kind` _string_ | Kind is the kind of member of this group, which can be either
VirtualMachine or VirtualMachineGroup.

If omitted, it defaults to VirtualMachine. |
| `powerOnDelay` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#duration-v1-meta)_ | PowerOnDelay is the amount of time to wait before powering on the member.

If omitted, the member will be powered on immediately when the group's
power state changes to PoweredOn. |

### GuestHeartbeatAction



GuestHeartbeatAction describes an action based on the guest heartbeat.

_Appears in:_
- [VirtualMachineReadinessProbeSpec](#virtualmachinereadinessprobespec)

| Field | Description |
| --- | --- |
| `thresholdStatus` _[GuestHeartbeatStatus](#guestheartbeatstatus)_ | ThresholdStatus is the value that the guest heartbeat status must be at or above to be
considered successful. |

### GuestHeartbeatStatus

_Underlying type:_ `string`

GuestHeartbeatStatus is the guest heartbeat status.

_Appears in:_
- [GuestHeartbeatAction](#guestheartbeataction)


### GuestInfoAction



GuestInfoAction describes a key from GuestInfo that must match the associated
value expression.

_Appears in:_
- [VirtualMachineReadinessProbeSpec](#virtualmachinereadinessprobespec)

| Field | Description |
| --- | --- |
| `key` _string_ | Key is the name of the GuestInfo key.

The key is automatically prefixed with "guestinfo." before being
evaluated. Thus if the key "guestinfo.mykey" is provided, it will be
evaluated as "guestinfo.guestinfo.mykey". |
| `value` _string_ | Value is a regular expression that is matched against the value of the
specified key.

An empty value is the equivalent of "match any" or ".*".

All values must adhere to the RE2 regular expression syntax as documented
at https://golang.org/s/re2syntax. Invalid values may be rejected or
ignored depending on the implementation of this API. Either way, invalid
values will not be considered when evaluating the ready state of a VM. |

### InstanceStorage



InstanceStorage provides information used to configure instance
storage volumes for a VirtualMachine.

_Appears in:_
- [VirtualMachineClassHardware](#virtualmachineclasshardware)

| Field | Description |
| --- | --- |
| `storageClass` _string_ | StorageClass refers to the name of a StorageClass resource used to
provide the storage for the configured instance storage volumes.
The value of this field has no relationship to or bearing on the field
virtualMachine.spec.storageClass. Please note the referred StorageClass
must be available in the same namespace as the VirtualMachineClass that
uses it for configuring instance storage. |
| `volumes` _[InstanceStorageVolume](#instancestoragevolume) array_ | Volumes describes instance storage volumes created for a VirtualMachine
instance that use this VirtualMachineClass. |

### InstanceStorageVolume



InstanceStorageVolume contains information required to create an
instance storage volume on a VirtualMachine.

_Appears in:_
- [InstanceStorage](#instancestorage)

| Field | Description |
| --- | --- |
| `size` _[Quantity](#quantity)_ |  |

### InstanceVolumeClaimVolumeSource



InstanceVolumeClaimVolumeSource contains information about the instance
storage volume claimed as a PVC.

_Appears in:_
- [PersistentVolumeClaimVolumeSource](#persistentvolumeclaimvolumesource)

| Field | Description |
| --- | --- |
| `storageClass` _string_ | StorageClass is the name of the Kubernetes StorageClass that provides
the backing storage for this instance storage volume. |
| `size` _[Quantity](#quantity)_ | Size is the size of the requested instance storage volume. |

### LoadBalancerIngress



LoadBalancerIngress represents the status of a load balancer ingress point:
traffic intended for the service should be sent to an ingress point.
IP or Hostname may both be set in this structure. It is up to the consumer to
determine which field should be used when accessing this LoadBalancer.

_Appears in:_
- [LoadBalancerStatus](#loadbalancerstatus)

| Field | Description |
| --- | --- |
| `ip` _string_ | IP is set for load balancer ingress points that are specified by an IP
address. |
| `hostname` _string_ | Hostname is set for load balancer ingress points that are specified by a
DNS address. |

### LoadBalancerStatus



LoadBalancerStatus represents the status of a load balancer.

_Appears in:_
- [VirtualMachineServiceStatus](#virtualmachineservicestatus)

| Field | Description |
| --- | --- |
| `ingress` _[LoadBalancerIngress](#loadbalanceringress) array_ | Ingress is a list containing ingress addresses for the load balancer.
Traffic intended for the service should be sent to any of these ingress
points. |

### NetworkDeviceStatus



NetworkDeviceStatus defines the network interface IP configuration including
gateway, subnet mask and IP address as seen by OVF properties.

_Appears in:_
- [NetworkStatus](#networkstatus)

| Field | Description |
| --- | --- |
| `Gateway4` _string_ | Gateway4 is the gateway for the IPv4 address family for this device. |
| `MacAddress` _string_ | MacAddress is the MAC address of the network device. |
| `IPAddresses` _string array_ | IpAddresses represents one or more IP addresses assigned to the network
device in CIDR notation, ex. "192.0.2.1/16". |

### NetworkStatus



NetworkStatus describes the observed state of the VM's network configuration.

_Appears in:_
- [VirtualMachineTemplate](#virtualmachinetemplate)

| Field | Description |
| --- | --- |
| `Devices` _[NetworkDeviceStatus](#networkdevicestatus) array_ | Devices describe a list of current status information for each
network interface that is desired to be attached to the
VirtualMachineTemplate. |
| `Nameservers` _string array_ | Nameservers describe a list of the DNS servers accessible by one of the
VM's configured network devices. |

### OVFProperty



OVFProperty describes an OVF property associated with an image.
OVF properties may be used in conjunction with the vAppConfig bootstrap
provider to customize a VM during its creation.

_Appears in:_
- [VirtualMachineImageStatus](#virtualmachineimagestatus)

| Field | Description |
| --- | --- |
| `key` _string_ | Key describes the OVF property's key. |
| `type` _string_ | Type describes the OVF property's type. |
| `default` _string_ | Default describes the OVF property's default value. |

### PersistentVolumeClaimVolumeSource



PersistentVolumeClaimVolumeSource is a composite for the Kubernetes
corev1.PersistentVolumeClaimVolumeSource and instance storage options.

_Appears in:_
- [VirtualMachineVolume](#virtualmachinevolume)
- [VirtualMachineVolumeSource](#virtualmachinevolumesource)

| Field | Description |
| --- | --- |
| `claimName` _string_ | claimName is the name of a PersistentVolumeClaim in the same namespace as the pod using this volume.
More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims |
| `readOnly` _boolean_ | readOnly Will force the ReadOnly setting in VolumeMounts.
Default false. |
| `instanceVolumeClaim` _[InstanceVolumeClaimVolumeSource](#instancevolumeclaimvolumesource)_ | InstanceVolumeClaim is set if the PVC is backed by instance storage. |

### QuiesceSpec



QuiesceSpec represents specifications that will be used to quiesce
the guest when taking a snapshot.

_Appears in:_
- [VirtualMachineSnapshotSpec](#virtualmachinesnapshotspec)

| Field | Description |
| --- | --- |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#duration-v1-meta)_ | Timeout represents the maximum time in minutes for snapshot
operation to be performed on the virtual machine. The timeout
can not be less than 5 minutes or more than 240 minutes. |

### ResourcePoolSpec



ResourcePoolSpec defines a Logical Grouping of workloads that share resource
policies.

_Appears in:_
- [VirtualMachineSetResourcePolicySpec](#virtualmachinesetresourcepolicyspec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the name of the ResourcePool grouping. |
| `reservations` _[VirtualMachineResourceSpec](#virtualmachineresourcespec)_ | Reservations describes the guaranteed resources reserved for the
ResourcePool. |
| `limits` _[VirtualMachineResourceSpec](#virtualmachineresourcespec)_ | Limits describes the limit to resources available to the ResourcePool. |

### TCPSocketAction



TCPSocketAction describes an action based on opening a socket.

_Appears in:_
- [VirtualMachineReadinessProbeSpec](#virtualmachinereadinessprobespec)

| Field | Description |
| --- | --- |
| `port` _[IntOrString](#intorstring)_ | Port specifies a number or name of the port to access on the VM.
If the format of port is a number, it must be in the range 1 to 65535.
If the format of name is a string, it must be an IANA_SVC_NAME. |
| `host` _string_ | Host is an optional host name to connect to. Host defaults to the VM IP. |

### VGPUDevice



VGPUDevice contains the configuration corresponding to a vGPU device.

_Appears in:_
- [VirtualDevices](#virtualdevices)

| Field | Description |
| --- | --- |
| `profileName` _string_ |  |

### VMAffinityTerm



VMAffinityTerm defines the VM affinity/anti-affinity term.

_Appears in:_
- [VirtualMachineAffinityVMAffinitySpec](#virtualmachineaffinityvmaffinityspec)
- [VirtualMachineAntiAffinityVMAffinitySpec](#virtualmachineantiaffinityvmaffinityspec)

| Field | Description |
| --- | --- |
| `labelSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#labelselector-v1-meta)_ | LabelSelector is a label query over a set of VMs.
When omitted, this term matches with no VMs. |
| `topologyKey` _string_ | TopologyKey describes where this VM should be co-located (affinity) or not
co-located (anti-affinity).
Commonly used values include:
`kubernetes.io/hostname` -- The rule is executed in the context of a node/host.
`topology.kubernetes.io/zone` -- This rule is executed in the context of a zone.

Please note, The following rules apply when specifying the topology key in the context of a zone/host.

- When topology key is in the context of a zone, the only supported verbs are
  PreferredDuringSchedulingIgnoredDuringExecution and RequiredDuringSchedulingIgnoredDuringExecution.
- When topology key is in the context of a host, the only supported verbs are
  PreferredDuringSchedulingPreferredDuringExecution and RequiredDuringSchedulingPreferredDuringExecution
  for VM-VM node-level anti-affinity scheduling.
- When topology key is in the context of a host, the only supported verbs are
  PreferredDuringSchedulingIgnoredDuringExecution and RequiredDuringSchedulingIgnoredDuringExecution
  for VM-VM node-level anti-affinity scheduling. |

### VSphereClusterModuleStatus



VSphereClusterModuleStatus describes the observed state of a vSphere
cluster module.

_Appears in:_
- [VirtualMachineSetResourcePolicyStatus](#virtualmachinesetresourcepolicystatus)

| Field | Description |
| --- | --- |
| `groupName` _string_ |  |
| `moduleUUID` _string_ |  |
| `clusterMoID` _string_ |  |

### VirtualDevices



VirtualDevices contains information about the virtual devices associated
with a VirtualMachineClass.

_Appears in:_
- [VirtualMachineClassHardware](#virtualmachineclasshardware)

| Field | Description |
| --- | --- |
| `vgpuDevices` _[VGPUDevice](#vgpudevice) array_ |  |
| `dynamicDirectPathIODevices` _[DynamicDirectPathIODevice](#dynamicdirectpathiodevice) array_ |  |

### VirtualMachineAdvancedSpec



VirtualMachineAdvancedSpec describes a set of optional, advanced VM
configuration options.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `bootDiskCapacity` _[Quantity](#quantity)_ | BootDiskCapacity is the capacity of the VM's boot disk -- the first disk
from the VirtualMachineImage from which the VM was deployed.

Please note it is not advised to change this value while the VM is
running. Also, resizing the VM's boot disk may require actions inside of
the guest to take advantage of the additional capacity. Finally, changing
the size of the VM's boot disk, even increasing it, could adversely
affect the VM.

Please note this field is ignored if the VM is deployed from an ISO with
CD-ROM devices attached. |
| `defaultVolumeProvisioningMode` _[VirtualMachineVolumeProvisioningMode](#virtualmachinevolumeprovisioningmode)_ | DefaultVolumeProvisioningMode specifies the default provisioning mode for
persistent volumes managed by this VM. |
| `changeBlockTracking` _boolean_ | ChangeBlockTracking is a flag that enables incremental backup support
for this VM, a feature utilized by external backup systems such as
VMware Data Recovery. |

### VirtualMachineAffinitySpec



VirtualMachineAffinitySpec defines the group of affinity scheduling rules.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `zoneAffinity` _[VirtualMachineAffinityZoneAffinitySpec](#virtualmachineaffinityzoneaffinityspec)_ | ZoneAffinity describes affinity scheduling rules related to a zone. |
| `zoneAntiAffinity` _[VirtualMachineAntiAffinityZoneAffinitySpec](#virtualmachineantiaffinityzoneaffinityspec)_ | ZoneAntiAffinity describes anti-affinity scheduling rules related to a zone. |
| `vmAffinity` _[VirtualMachineAffinityVMAffinitySpec](#virtualmachineaffinityvmaffinityspec)_ | VMAffinity describes affinity scheduling rules related to other VMs. |
| `vmAntiAffinity` _[VirtualMachineAntiAffinityVMAffinitySpec](#virtualmachineantiaffinityvmaffinityspec)_ | VMAntiAffinity describes anti-affinity scheduling rules related to other VMs. |

### VirtualMachineAffinityVMAffinitySpec



VirtualMachineAffinityVMAffinitySpec defines the affinity requirements for scheduling
rules related to other VMs.

_Appears in:_
- [VirtualMachineAffinitySpec](#virtualmachineaffinityspec)

| Field | Description |
| --- | --- |
| `requiredDuringSchedulingIgnoredDuringExecution` _[VMAffinityTerm](#vmaffinityterm) array_ | RequiredDuringSchedulingIgnoredDuringExecution describes affinity
requirements that must be met or the VM will not be scheduled.

When there are multiple elements, the lists of nodes corresponding to
each term are intersected, i.e. all terms must be satisfied. |
| `preferredDuringSchedulingIgnoredDuringExecution` _[VMAffinityTerm](#vmaffinityterm) array_ | PreferredDuringSchedulingIgnoredDuringExecution describes affinity
requirements that should be met, but the VM can still be scheduled if
the requirement cannot be satisfied. The scheduler will prefer to schedule VMs
that satisfy the anti-affinity expressions specified by this field, but it may choose to
violate one or more of the expressions.

When there are multiple elements, the lists of nodes corresponding to
each term are intersected, i.e. all terms must be satisfied. |

### VirtualMachineAffinityZoneAffinitySpec



VirtualMachineAffinityZoneAffinitySpec defines the affinity scheduling rules
related to zones.

_Appears in:_
- [VirtualMachineAffinitySpec](#virtualmachineaffinityspec)

| Field | Description |
| --- | --- |
| `requiredDuringSchedulingIgnoredDuringExecution` _[ZoneSelectorTerm](#zoneselectorterm) array_ | RequiredDuringSchedulingIgnoredDuringExecution describes affinity
requirements that must be met or the VM will not be scheduled.

When there are multiple elements, the lists of zones corresponding to
each term are intersected, i.e. all terms must be satisfied. |
| `preferredDuringSchedulingIgnoredDuringExecution` _[ZoneSelectorTerm](#zoneselectorterm) array_ | PreferredDuringSchedulingIgnoredDuringExecution describes affinity
requirements that should be met, but the VM can still be scheduled if
the requirement cannot be satisfied. The scheduler will prefer to schedule VMs
that satisfy the anti-affinity expressions specified by this field, but it may choose to
violate one or more of the expressions.

When there are multiple elements, the lists of zones corresponding to
each term are intersected, i.e. all terms must be satisfied. |

### VirtualMachineAntiAffinityVMAffinitySpec



VirtualMachineAntiAffinityVMAffinitySpec defines the anti-affinity requirements for scheduling
rules related to other VMs.

_Appears in:_
- [VirtualMachineAffinitySpec](#virtualmachineaffinityspec)

| Field | Description |
| --- | --- |
| `requiredDuringSchedulingIgnoredDuringExecution` _[VMAffinityTerm](#vmaffinityterm) array_ | RequiredDuringSchedulingIgnoredDuringExecution describes anti-affinity
requirements that must be met or the VM will not be scheduled.

When there are multiple elements, the lists of nodes corresponding to
each term are intersected, i.e. all terms must be satisfied. |
| `preferredDuringSchedulingIgnoredDuringExecution` _[VMAffinityTerm](#vmaffinityterm) array_ | PreferredDuringSchedulingIgnoredDuringExecution describes anti-affinity
requirements that should be met, but the VM can still be scheduled if
the requirement cannot be satisfied. The scheduler will prefer to schedule VMs
that satisfy the affinity expressions specified by this field, but it may choose to
violate one or more of the expressions.

When there are multiple elements, the lists of nodes corresponding to
each term are intersected, i.e. all terms must be satisfied. |
| `requiredDuringSchedulingPreferredDuringExecution` _[VMAffinityTerm](#vmaffinityterm) array_ | RequiredDuringSchedulingPreferredExecution describes anti-affinity
requirements that must be met or the VM will not be scheduled. Additionally,
it also describes the anti-affinity requirements that should be met during run-time,
but the VM can still be run if the requirements cannot be satisfied.

When there are multiple elements, the lists of nodes corresponding to
each term are intersected, i.e. all terms must be satisfied. |
| `preferredDuringSchedulingPreferredDuringExecution` _[VMAffinityTerm](#vmaffinityterm) array_ | PreferredDuringSchedulingPreferredDuringExecution describes anti-affinity
requirements that should be met, but the VM can still be scheduled if
the requirement cannot be satisfied. The scheduler will prefer to schedule VMs
that satisfy the affinity expressions specified by this field, but it may choose to
violate one or more of the expressions. Additionally,
it also describes the anti-affinity requirements that should be met during run-time,
but the VM can still be run if the requirements cannot be satisfied.

When there are multiple elements, the lists of nodes corresponding to
each term are intersected, i.e. all terms must be satisfied. |

### VirtualMachineAntiAffinityZoneAffinitySpec



VirtualMachineAntiAffinityZoneAffinitySpec defines the anti-affinity scheduling rules
related to zones.

_Appears in:_
- [VirtualMachineAffinitySpec](#virtualmachineaffinityspec)

| Field | Description |
| --- | --- |
| `requiredDuringSchedulingIgnoredDuringExecution` _[ZoneSelectorTerm](#zoneselectorterm) array_ | RequiredDuringSchedulingIgnoredDuringExecution describes affinity
requirements that must be met or the VM will not be scheduled.

When there are multiple elements, the lists of zones corresponding to
each term are intersected, i.e. all terms must be satisfied. |
| `preferredDuringSchedulingIgnoredDuringExecution` _[ZoneSelectorTerm](#zoneselectorterm) array_ | PreferredDuringSchedulingIgnoredDuringExecution describes affinity
requirements that should be met, but the VM can still be scheduled if
the requirement cannot be satisfied. The scheduler will prefer to schedule VMs to
that satisfy the anti-affinity expressions specified by this field, but it may choose to
violate one or more of the expressions.

When there are multiple elements, the lists of zones corresponding to
each term are intersected, i.e. all terms must be satisfied. |

### VirtualMachineBootOptions



VirtualMachineBootOptions defines the boot-time behavior of a virtual machine.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `bootDelay` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#duration-v1-meta)_ | BootDelay is the delay before starting the boot sequence. The boot delay
specifies a time interval between virtual machine power on or restart and
the beginning of the boot sequence. |
| `bootOrder` _[VirtualMachineBootOptionsBootableDevice](#virtualmachinebootoptionsbootabledevice) array_ | BootOrder represents the boot order of the virtual machine. After list is exhausted,
default BIOS boot device algorithm is used for booting. Note that order of the entries
in the list is important: device listed first is used for boot first, if that one
fails second entry is used, and so on. Platform may have some internal limit on the
number of devices it supports. If bootable device is not reached before platform's limit
is hit, boot will fail. At least single entry is supported by all products supporting
boot order settings.

The available devices are:

- Disk    -- If there are classic and managed disks, the first classic disk is selected.
             If there are only managed disks, the first disk is selected.
- Network -- The first interface listed in spec.network.interfaces.
- CDRom   -- The first bootable CD-ROM device. |
| `bootRetryEnabled` _boolean_ | BootRetryEnabled specifies whether a virtual machine that fails to boot
will try again. If set to true, a virtual machine that fails to boot will
try again after BootRetryDelay time period has expired. If set to false, the
virtual machine waits indefinitely for you to initiate boot retry. |
| `bootRetryDelay` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#duration-v1-meta)_ | BootRetryDelay specifies a time interval between virtual machine boot failure
and the subsequent attempt to boot again. The virtual machine uses this value
only if BootRetryEnabled is true. |
| `enterBIOSSetup` _boolean_ | EnterBIOSSetup specifies whether to automatically enter BIOS setup the next
time the virtual machine boots. The virtual machine resets this flag to false
so that subsequent boots proceed normally. |
| `efiSecureBootEnabled` _boolean_ | EFISecureBootEnabled specifies whether the virtual machine's firmware will
perform signature checks of any EFI images loaded during startup. If set to
true, signature checks will be performed and the virtual machine's firmware
will refuse to start any images which do not pass those signature checks.

Please note, this field will not be honored unless the value of spec.firmware
is "EFI". |
| `networkBootProtocol` _[VirtualMachineBootOptionsNetworkBootProtocol](#virtualmachinebootoptionsnetworkbootprotocol)_ | NetworkBootProtocol is the protocol to attempt during PXE network boot or NetBoot.
The available protocols are:

- IP4 -- PXE (or Apple NetBoot) over IPv4. The default.
- IP6 -- PXE over IPv6. Only meaningful for EFI virtual machines. |

### VirtualMachineBootOptionsBootableDevice

_Underlying type:_ `string`

VirtualMachineBootOptionsBootableDevice represents the type of bootable device
that a VM may be booted from.

_Appears in:_
- [VirtualMachineBootOptions](#virtualmachinebootoptions)


### VirtualMachineBootOptionsNetworkBootProtocol

_Underlying type:_ `string`

VirtualMachineBootOptionsNetworkBootProtocol represents the protocol to
use during PXE network boot or NetBoot.

_Appears in:_
- [VirtualMachineBootOptions](#virtualmachinebootoptions)


### VirtualMachineBootstrapCloudInitSpec



VirtualMachineBootstrapCloudInitSpec describes the CloudInit configuration
used to bootstrap the VM.

_Appears in:_
- [VirtualMachineBootstrapSpec](#virtualmachinebootstrapspec)

| Field | Description |
| --- | --- |
| `instanceID` _string_ | InstanceID is the cloud-init metadata instance ID.
If omitted, this field defaults to the VM's BiosUUID. |
| `cloudConfig` _[CloudConfig](#cloudconfig)_ | CloudConfig describes a subset of a Cloud-Init CloudConfig, used to
bootstrap the VM.

Please note this field and RawCloudConfig are mutually exclusive. |
| `rawCloudConfig` _[SecretKeySelector](#secretkeyselector)_ | RawCloudConfig describes a key in a Secret resource that contains the
CloudConfig data used to bootstrap the VM.

The CloudConfig data specified by the key may be plain-text,
base64-encoded, or gzipped and base64-encoded.

Please note this field and CloudConfig are mutually exclusive. |
| `sshAuthorizedKeys` _string array_ | SSHAuthorizedKeys is a list of public keys that CloudInit will apply to
the guest's default user. |
| `useGlobalNameserversAsDefault` _boolean_ | UseGlobalNameserversAsDefault will use the global nameservers specified in
the NetworkSpec as the per-interface nameservers when the per-interface
nameservers is not provided.

Defaults to true if omitted. |
| `useGlobalSearchDomainsAsDefault` _boolean_ | UseGlobalSearchDomainsAsDefault will use the global search domains specified
in the NetworkSpec as the per-interface search domains when the per-interface
search domains is not provided.

Defaults to true if omitted. |
| `waitOnNetwork4` _boolean_ | WaitOnNetwork4 indicates whether the cloud-init datasource should wait
for an IPv4 address to be available before writing the instance-data.

When set to true, the cloud-init datasource will sleep for a second,
check network status, and repeat until an IPv4 address is available. |
| `waitOnNetwork6` _boolean_ | WaitOnNetwork6 indicates whether the cloud-init datasource should wait
for an IPv6 address to be available before writing the instance-data.

When set to true, the cloud-init datasource will sleep for a second,
check network status, and repeat until an IPv6 address is available. |

### VirtualMachineBootstrapLinuxPrepSpec



VirtualMachineBootstrapLinuxPrepSpec describes the LinuxPrep configuration
used to bootstrap the VM.

_Appears in:_
- [VirtualMachineBootstrapSpec](#virtualmachinebootstrapspec)

| Field | Description |
| --- | --- |
| `hardwareClockIsUTC` _boolean_ | HardwareClockIsUTC specifies whether the hardware clock is in UTC or
local time. |
| `timeZone` _string_ | TimeZone is a case-sensitive timezone, such as Europe/Sofia.

Valid values are based on the tz (timezone) database used by Linux and
other Unix systems. The values are strings in the form of
"Area/Location," in which Area is a continent or ocean name, and
Location is the city, island, or other regional designation.

Please see https://kb.vmware.com/s/article/2145518 for a list of valid
time zones for Linux systems. |

### VirtualMachineBootstrapSpec



VirtualMachineBootstrapSpec defines the desired state of a VM's bootstrap
configuration.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `cloudInit` _[VirtualMachineBootstrapCloudInitSpec](#virtualmachinebootstrapcloudinitspec)_ | CloudInit may be used to bootstrap Linux guests with Cloud-Init or
Windows guests that support Cloudbase-Init.

The guest's networking stack is configured by Cloud-Init on Linux guests
and Cloudbase-Init on Windows guests.

Please note this bootstrap provider may not be used in conjunction with
the other bootstrap providers. |
| `linuxPrep` _[VirtualMachineBootstrapLinuxPrepSpec](#virtualmachinebootstraplinuxprepspec)_ | LinuxPrep may be used to bootstrap Linux guests.

The guest's networking stack is configured by Guest OS Customization
(GOSC).

Please note this bootstrap provider may be used in conjunction with the
VAppConfig bootstrap provider when wanting to configure the guest's
network with GOSC but also send vApp/OVF properties into the guest.

This bootstrap provider may not be used in conjunction with the CloudInit
or Sysprep bootstrap providers. |
| `sysprep` _[VirtualMachineBootstrapSysprepSpec](#virtualmachinebootstrapsysprepspec)_ | Sysprep may be used to bootstrap Windows guests.

The guest's networking stack is configured by Guest OS Customization
(GOSC).

Please note this bootstrap provider may be used in conjunction with the
VAppConfig bootstrap provider when wanting to configure the guest's
network with GOSC but also send vApp/OVF properties into the guest.

This bootstrap provider may not be used in conjunction with the CloudInit
or LinuxPrep bootstrap providers. |
| `vAppConfig` _[VirtualMachineBootstrapVAppConfigSpec](#virtualmachinebootstrapvappconfigspec)_ | VAppConfig may be used to bootstrap guests that rely on vApp properties
(how VMware surfaces OVF properties on guests) to transport data into the
guest.

The guest's networking stack may be configured using either vApp
properties or GOSC.

Many OVFs define one or more properties that are used by the guest to
bootstrap its networking stack. If the VirtualMachineImage defines one or
more properties like this, then they can be configured to use the network
data provided for this VM at runtime by setting these properties to Go
template strings.

It is also possible to use GOSC to bootstrap this VM's network stack by
configuring either the LinuxPrep or Sysprep bootstrap providers.

Please note the VAppConfig bootstrap provider in conjunction with the
LinuxPrep bootstrap provider is the equivalent of setting the v1alpha1
VM metadata transport to "OvfEnv".

This bootstrap provider may not be used in conjunction with the CloudInit
bootstrap provider. |

### VirtualMachineBootstrapSysprepSpec



VirtualMachineBootstrapSysprepSpec describes the Sysprep configuration used
to bootstrap the VM.

_Appears in:_
- [VirtualMachineBootstrapSpec](#virtualmachinebootstrapspec)

| Field | Description |
| --- | --- |
| `sysprep` _[Sysprep](#sysprep)_ | Sysprep is an object representation of a Windows sysprep.xml answer file.

This field encloses all the individual keys listed in a sysprep.xml file.

For more detailed information please see
https://technet.microsoft.com/en-us/library/cc771830(v=ws.10).aspx.

Please note this field and RawSysprep are mutually exclusive. |
| `rawSysprep` _[SecretKeySelector](#secretkeyselector)_ | RawSysprep describes a key in a Secret resource that contains an XML
string of the Sysprep text used to bootstrap the VM.

The data specified by the Secret key may be plain-text, base64-encoded,
or gzipped and base64-encoded.

Please note this field and Sysprep are mutually exclusive. |

### VirtualMachineBootstrapVAppConfigSpec



VirtualMachineBootstrapVAppConfigSpec describes the vApp configuration
used to bootstrap the VM.

_Appears in:_
- [VirtualMachineBootstrapSpec](#virtualmachinebootstrapspec)

| Field | Description |
| --- | --- |
| `properties` _KeyValueOrSecretKeySelectorPair array_ | Properties is a list of vApp/OVF property key/value pairs.

Please note this field and RawProperties are mutually exclusive. |
| `rawProperties` _string_ | RawProperties is the name of a Secret resource in the same Namespace as
this VM where each key/value pair from the Secret is used as a vApp
key/value pair.

Please note this field and Properties are mutually exclusive. |

### VirtualMachineCdromSpec



VirtualMachineCdromSpec describes the desired state of a CD-ROM device.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name consists of at least two lowercase letters or digits of this CD-ROM.
It must be unique among all CD-ROM devices attached to the VM.

This field is immutable when the VM is powered on. |
| `image` _[VirtualMachineImageRef](#virtualmachineimageref)_ | Image describes the reference to an ISO type VirtualMachineImage or
ClusterVirtualMachineImage resource used as the backing for the CD-ROM.
If the image kind is omitted, it defaults to VirtualMachineImage.

This field is immutable when the VM is powered on.

Please note, unlike the spec.imageName field, the value of this
spec.cdrom.image.name MUST be a Kubernetes object name. |
| `connected` _boolean_ | Connected describes the desired connection state of the CD-ROM device.

When true, the CD-ROM device is added and connected to the VM.
If the device already exists, it is updated to a connected state.

When explicitly set to false, the CD-ROM device is added but remains
disconnected from the VM. If the CD-ROM device already exists, it is
updated to a disconnected state.

Note: Before disconnecting a CD-ROM, the device may need to be unmounted
in the guest OS. Refer to the following KB article for more details:
https://knowledge.broadcom.com/external/article?legacyId=2144053

Defaults to true if omitted. |
| `allowGuestControl` _boolean_ | AllowGuestControl describes whether or not a web console connection
may be used to connect/disconnect the CD-ROM device.

Defaults to true if omitted. |

### VirtualMachineClassHardware



VirtualMachineClassHardware describes a virtual hardware resource
specification.

_Appears in:_
- [VirtualMachineClassInstanceSpec](#virtualmachineclassinstancespec)
- [VirtualMachineClassSpec](#virtualmachineclassspec)

| Field | Description |
| --- | --- |
| `cpus` _integer_ |  |
| `memory` _[Quantity](#quantity)_ |  |
| `devices` _[VirtualDevices](#virtualdevices)_ |  |
| `instanceStorage` _[InstanceStorage](#instancestorage)_ |  |

### VirtualMachineClassInstanceSpec



VirtualMachineClassInstanceSpec defines the desired state of VirtualMachineClassInstance.
It is a composite of VirtualMachineClassSpec.

_Appears in:_
- [VirtualMachineClassInstance](#virtualmachineclassinstance)

| Field | Description |
| --- | --- |
| `controllerName` _string_ | ControllerName describes the name of the controller responsible for
reconciling VirtualMachine resources that are realized from this
VirtualMachineClass.

When omitted, controllers reconciling VirtualMachine resources determine
the default controller name from the environment variable
DEFAULT_VM_CLASS_CONTROLLER_NAME. If this environment variable is not
defined or empty, it defaults to vmoperator.vmware.com/vsphere.

Once a non-empty value is assigned to this field, attempts to set this
field to an empty value will be silently ignored. |
| `hardware` _[VirtualMachineClassHardware](#virtualmachineclasshardware)_ | Hardware describes the configuration of the VirtualMachineClass
attributes related to virtual hardware. The configuration specified in
this field is used to customize the virtual hardware characteristics of
any VirtualMachine associated with this VirtualMachineClass. |
| `policies` _[VirtualMachineClassPolicies](#virtualmachineclasspolicies)_ | Policies describes the configuration of the VirtualMachineClass
attributes related to virtual infrastructure policy. The configuration
specified in this field is used to customize various policies related to
infrastructure resource consumption. |
| `description` _string_ | Description describes the configuration of the VirtualMachineClass which
is not related to virtual hardware or infrastructure policy. This field
is used to address remaining specs about this VirtualMachineClass. |
| `configSpec` _[RawMessage](#rawmessage)_ | ConfigSpec describes additional configuration information for a
VirtualMachine.
The contents of this field are the VirtualMachineConfigSpec data object
(https://bit.ly/3HDtiRu) marshaled to JSON using the discriminator
field "_typeName" to preserve type information. |
| `reservedProfileID` _string_ | ReservedProfileID describes the reservation profile associated with
the namespace-scoped VirtualMachineClass object. |
| `reservedSlots` _integer_ | ReservedSlots describes the number of slots reserved for VMs that use
this VirtualMachineClass.
This field is only valid in conjunction with reservedProfileID. |

### VirtualMachineClassInstanceStatus



VirtualMachineClassInstanceStatus defines the observed state of VirtualMachineClassInstance.

_Appears in:_
- [VirtualMachineClassInstance](#virtualmachineclassinstance)


### VirtualMachineClassPolicies



VirtualMachineClassPolicies describes the policy configuration to be used by
a VirtualMachineClass.

_Appears in:_
- [VirtualMachineClassInstanceSpec](#virtualmachineclassinstancespec)
- [VirtualMachineClassSpec](#virtualmachineclassspec)

| Field | Description |
| --- | --- |
| `resources` _[VirtualMachineClassResources](#virtualmachineclassresources)_ |  |

### VirtualMachineClassResources



VirtualMachineClassResources describes the virtual hardware resource
reservations and limits configuration to be used by a VirtualMachineClass.

_Appears in:_
- [VirtualMachineClassPolicies](#virtualmachineclasspolicies)

| Field | Description |
| --- | --- |
| `requests` _[VirtualMachineResourceSpec](#virtualmachineresourcespec)_ |  |
| `limits` _[VirtualMachineResourceSpec](#virtualmachineresourcespec)_ |  |

### VirtualMachineClassSpec



VirtualMachineClassSpec defines the desired state of VirtualMachineClass.

_Appears in:_
- [VirtualMachineClass](#virtualmachineclass)
- [VirtualMachineClassInstanceSpec](#virtualmachineclassinstancespec)

| Field | Description |
| --- | --- |
| `controllerName` _string_ | ControllerName describes the name of the controller responsible for
reconciling VirtualMachine resources that are realized from this
VirtualMachineClass.

When omitted, controllers reconciling VirtualMachine resources determine
the default controller name from the environment variable
DEFAULT_VM_CLASS_CONTROLLER_NAME. If this environment variable is not
defined or empty, it defaults to vmoperator.vmware.com/vsphere.

Once a non-empty value is assigned to this field, attempts to set this
field to an empty value will be silently ignored. |
| `hardware` _[VirtualMachineClassHardware](#virtualmachineclasshardware)_ | Hardware describes the configuration of the VirtualMachineClass
attributes related to virtual hardware. The configuration specified in
this field is used to customize the virtual hardware characteristics of
any VirtualMachine associated with this VirtualMachineClass. |
| `policies` _[VirtualMachineClassPolicies](#virtualmachineclasspolicies)_ | Policies describes the configuration of the VirtualMachineClass
attributes related to virtual infrastructure policy. The configuration
specified in this field is used to customize various policies related to
infrastructure resource consumption. |
| `description` _string_ | Description describes the configuration of the VirtualMachineClass which
is not related to virtual hardware or infrastructure policy. This field
is used to address remaining specs about this VirtualMachineClass. |
| `configSpec` _[RawMessage](#rawmessage)_ | ConfigSpec describes additional configuration information for a
VirtualMachine.
The contents of this field are the VirtualMachineConfigSpec data object
(https://bit.ly/3HDtiRu) marshaled to JSON using the discriminator
field "_typeName" to preserve type information. |
| `reservedProfileID` _string_ | ReservedProfileID describes the reservation profile associated with
the namespace-scoped VirtualMachineClass object. |
| `reservedSlots` _integer_ | ReservedSlots describes the number of slots reserved for VMs that use
this VirtualMachineClass.
This field is only valid in conjunction with reservedProfileID. |

### VirtualMachineClassStatus



VirtualMachineClassStatus defines the observed state of VirtualMachineClass.

_Appears in:_
- [VirtualMachineClass](#virtualmachineclass)


### VirtualMachineCryptoSpec



VirtualMachineCryptoSpec defines the desired state of a VirtualMachine's
encryption state.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `encryptionClassName` _string_ | EncryptionClassName describes the name of the EncryptionClass resource
used to encrypt this VM.

Please note, this field is not required to encrypt the VM. If the
underlying platform has a default key provider, the VM may still be fully
or partially encrypted depending on the specified storage and VM classes.

If there is a default key provider and an encryption storage class is
selected, the files in the VM's home directory and non-PVC virtual disks
will be encrypted

If there is a default key provider and a VM Class with a virtual, trusted
platform module (vTPM) is selected, the files in the VM's home directory,
minus any virtual disks, will be encrypted.

If the underlying vSphere platform does not have a default key provider,
then this field is required when specifying an encryption storage class
and/or a VM Class with a vTPM.

If this field is set, spec.storageClass must use an encryption-enabled
storage class. |
| `useDefaultKeyProvider` _boolean_ | UseDefaultKeyProvider describes the desired behavior for when an explicit
EncryptionClass is not provided.

When an explicit EncryptionClass is not provided and this value is true:

- Deploying a VirtualMachine with an encryption storage policy or vTPM
  will be encrypted using the default key provider.

- If a VirtualMachine is not encrypted, uses an encryption storage
  policy or has a virtual, trusted platform module (vTPM), there is a
  default key provider, the VM will be encrypted using the default key
  provider.

- If a VirtualMachine is encrypted with a provider other than the default
  key provider, the VM will be rekeyed using the default key provider.

When an explicit EncryptionClass is not provided and this value is false:

- Deploying a VirtualMachine with an encryption storage policy or vTPM
  will fail.

- If a VirtualMachine is encrypted with a provider other than the default
  key provider, the VM will be not be rekeyed.

  Please note, this could result in a VirtualMachine that cannot be
  powered on since it is encrypted using a provider or key that may have
  been removed. Without the key, the VM cannot be decrypted and thus
  cannot be powered on.

Defaults to true if omitted. |

### VirtualMachineCryptoStatus





_Appears in:_
- [VirtualMachineStatus](#virtualmachinestatus)

| Field | Description |
| --- | --- |
| `encrypted` _[VirtualMachineEncryptionType](#virtualmachineencryptiontype) array_ | Encrypted describes the observed state of the VirtualMachine's
encryption. There may be two values in this list:

- Config -- This refers to all of the files related to a VM except any
            virtual disks.
- Disks  -- This refers to at least one of the VM's attached disks. To
            determine the encryption state of the individual disks,
            please refer to status.volumes[].crypto. |
| `providerID` _string_ | ProviderID describes the provider ID used to encrypt the VirtualMachine.
Please note, this field will be empty if the VirtualMachine is not
encrypted. |
| `keyID` _string_ | KeyID describes the key ID used to encrypt the VirtualMachine.
Please note, this field will be empty if the VirtualMachine is not
encrypted. |


### VirtualMachineEncryptionType

_Underlying type:_ `string`



_Appears in:_
- [VirtualMachineCryptoStatus](#virtualmachinecryptostatus)


### VirtualMachineGroupMemberStatus



VirtualMachineGroupMemberStatus describes the observed status of a group
member.

_Appears in:_
- [VirtualMachineGroupStatus](#virtualmachinegroupstatus)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of this member. |
| `kind` _string_ | Kind is the kind of this member, which can be either VirtualMachine or
VirtualMachineGroup. |
| `placement` _[VirtualMachinePlacementStatus](#virtualmachineplacementstatus)_ | Placement describes the placement results for this member.

Please note this field is only set for VirtualMachine members. |
| `powerState` _[VirtualMachinePowerState](#virtualmachinepowerstate)_ | PowerState describes the observed power state of this member.

Please note this field is only set for VirtualMachine members. |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions describes any conditions associated with this member.

- The GroupLinked condition is True when the member exists and has its
  "Spec.GroupName" field set to the group's name.
- The PowerStateSynced condition is True for the VirtualMachine member
  when the member's power state matches the group's power state.
- The PlacementReady condition is True for the VirtualMachine member
  when the member has a placement decision ready.
- The ReadyType condition is True for the VirtualMachineGroup member
  when all of its members' conditions are True. |

### VirtualMachineGroupPlacementDatastoreStatus





_Appears in:_
- [VirtualMachinePlacementStatus](#virtualmachineplacementstatus)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the name of a datastore. |
| `id` _string_ | ID describes the datastore ID. |
| `url` _string_ | URL describes the datastore URL. |
| `supportedDiskFormats` _string array_ | SupportedDiskFormat describes the list of disk formats supported by this
datastore. |
| `diskKey` _integer_ | DiskKey describes the device key to which this recommendation applies.
When omitted, this recommendation is for the VM's home directory. |

### VirtualMachineGroupSpec



VirtualMachineGroupSpec defines the desired state of VirtualMachineGroup.

_Appears in:_
- [VirtualMachineGroup](#virtualmachinegroup)

| Field | Description |
| --- | --- |
| `groupName` _string_ | GroupName describes the name of the group that this group belongs to.

If omitted, this group is not a member of any other group. |
| `members` _[GroupMember](#groupmember) array_ | Members describes the names of VirtualMachine or VirtualMachineGroup
objects that are members of this group. The VM or VM Group objects must
be in the same namespace as this group. |
| `powerState` _[VirtualMachinePowerState](#virtualmachinepowerstate)_ | PowerState describes the desired power state of a VirtualMachineGroup.

Please note this field may be omitted when creating a new VM group. This
ensures that the power states of any existing VMs that are added to the
group do not have their power states changed until the group's power
state is explicitly altered.

However, once the field is set to a non-empty value, it may no longer be
set to an empty value. This means that if the group's power state is
PoweredOn, and a VM whose power state is PoweredOff is added to the
group, that VM will be powered on. |
| `powerOffMode` _[VirtualMachinePowerOpMode](#virtualmachinepoweropmode)_ | PowerOffMode describes the desired behavior when powering off a VM Group.
Refer to the VirtualMachine.PowerOffMode field for more details.

Please note this field is only propagated to the group's members when
the group's power state is changed.

If omitted, the mode defaults to TrySoft. |
| `suspendMode` _[VirtualMachinePowerOpMode](#virtualmachinepoweropmode)_ | SuspendMode describes the desired behavior when suspending a VM Group.
Refer to the VirtualMachine.SuspendMode field for more details.

Please note this field is only propagated to the group's members when
the group's power state is changed.

If omitted, the mode defaults to TrySoft. |

### VirtualMachineGroupStatus



VirtualMachineGroupStatus defines the observed state of VirtualMachineGroup.

_Appears in:_
- [VirtualMachineGroup](#virtualmachinegroup)

| Field | Description |
| --- | --- |
| `members` _[VirtualMachineGroupMemberStatus](#virtualmachinegroupmemberstatus) array_ | Members describes the observed status of group members. |
| `lastUpdatedPowerStateTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | LastUpdatedPowerStateTime describes the observed time when the power
state of the group was last updated. |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions describes any conditions associated with this VM Group.

- The ReadyType condition is True when all of the group members have
  all of their expected conditions set to True. |


### VirtualMachineImageCacheFileStatus





_Appears in:_
- [VirtualMachineImageCacheLocationStatus](#virtualmachineimagecachelocationstatus)

| Field | Description |
| --- | --- |
| `id` _string_ | ID describes the value used to locate the file.
The value of this field depends on the type of file:

- Type=Other                  -- The ID value describes a datastore path,
                                 ex. "[my-datastore-1] .contentlib-cache/1234/5678/my-disk-1.vmdk"
- Type=Disk, DiskType=Classic -- The ID value describes a datastore
                                 path.
- Type=Disk, DiskType=Managed -- The ID value describes a First Class
                                 Disk (FCD). |
| `type` _[VirtualMachineImageCacheFileType](#virtualmachineimagecachefiletype)_ | Type describes the type of file. |
| `diskType` _[VirtualMachineVolumeType](#virtualmachinevolumetype)_ | DiskType describes the type of disk.
This field is only non-empty when Type=Disk. |

### VirtualMachineImageCacheFileType

_Underlying type:_ `string`

VirtualMachineImageCacheFileType describes the types of files that may be
cached.

_Appears in:_
- [VirtualMachineImageCacheFileStatus](#virtualmachineimagecachefilestatus)


### VirtualMachineImageCacheLocationSpec





_Appears in:_
- [VirtualMachineImageCacheSpec](#virtualmachineimagecachespec)

| Field | Description |
| --- | --- |
| `datacenterID` _string_ | DatacenterID describes the ID of the datacenter to which the image should
be cached. |
| `datastoreID` _string_ | DatastoreID describes the ID of the datastore to which the image should
be cached. |

### VirtualMachineImageCacheLocationStatus





_Appears in:_
- [VirtualMachineImageCacheStatus](#virtualmachineimagecachestatus)

| Field | Description |
| --- | --- |
| `datacenterID` _string_ | DatacenterID describes the ID of the datacenter to which the image should
be cached. |
| `datastoreID` _string_ | DatastoreID describes the ID of the datastore to which the image should
be cached. |
| `files` _[VirtualMachineImageCacheFileStatus](#virtualmachineimagecachefilestatus) array_ | Files describes the image's files cached on this datastore. |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions describes any conditions associated with this cache location.

Generally this should just include the ReadyType condition. |

### VirtualMachineImageCacheOVFStatus





_Appears in:_
- [VirtualMachineImageCacheStatus](#virtualmachineimagecachestatus)

| Field | Description |
| --- | --- |
| `configMapName` _string_ | ConfigMapName describes the name of the ConfigMap resource that contains
the image's OVF envelope encoded as YAML. The data is located in the
ConfigMap key "value". |
| `providerVersion` _string_ | ProviderVersion describes the observed provider version at which the OVF
is cached.
The provider is Content Library, the version is the content version. |

### VirtualMachineImageCacheSpec



VirtualMachineImageCacheSpec defines the desired state of
VirtualMachineImageCache.

_Appears in:_
- [VirtualMachineImageCache](#virtualmachineimagecache)

| Field | Description |
| --- | --- |
| `providerID` _string_ | ProviderID describes the ID of the provider item to which the image
corresponds.
If the provider is Content Library, the ID refers to a Content Library
item. |
| `providerVersion` _string_ | ProviderVersion describes the version of the provider item to which the
image corresponds.
The provider is Content Library, the version is the content version. |
| `locations` _[VirtualMachineImageCacheLocationSpec](#virtualmachineimagecachelocationspec) array_ | Locations describes the locations where the image should be cached. |

### VirtualMachineImageCacheStatus



VirtualMachineImageCacheStatus defines the observed state of
VirtualMachineImageCache.

_Appears in:_
- [VirtualMachineImageCache](#virtualmachineimagecache)

| Field | Description |
| --- | --- |
| `locations` _[VirtualMachineImageCacheLocationStatus](#virtualmachineimagecachelocationstatus) array_ | Locations describe the observed locations where the image is cached. |
| `ovf` _[VirtualMachineImageCacheOVFStatus](#virtualmachineimagecacheovfstatus)_ | OVF describes the observed status of the cached OVF content. |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions describes any conditions associated with this cached image.

Generally this should just include the ReadyType condition, which will
only be True if all of the cached locations also have True ReadyType
condition. |

### VirtualMachineImageDiskInfo



VirtualMachineImageDiskInfo describes information about any disks associated with
this image.

_Appears in:_
- [VirtualMachineImageStatus](#virtualmachineimagestatus)

| Field | Description |
| --- | --- |
| `capacity` _[Quantity](#quantity)_ | Capacity is the virtual disk capacity in bytes. |
| `size` _[Quantity](#quantity)_ | Size is the estimated populated size of the virtual disk in bytes. |

### VirtualMachineImageOSInfo



VirtualMachineImageOSInfo describes the image's guest operating system.

_Appears in:_
- [VirtualMachineImageStatus](#virtualmachineimagestatus)

| Field | Description |
| --- | --- |
| `id` _string_ | ID describes the operating system ID.

This value is also added to the image resource's labels as
VirtualMachineImageOSIDLabel. |
| `type` _string_ | Type describes the operating system type.

This value is also added to the image resource's labels as
VirtualMachineImageOSTypeLabel. |
| `version` _string_ | Version describes the operating system version.

This value is also added to the image resource's labels as
VirtualMachineImageOSVersionLabel. |

### VirtualMachineImageProductInfo



VirtualMachineImageProductInfo describes product information for an image.

_Appears in:_
- [VirtualMachineImageStatus](#virtualmachineimagestatus)

| Field | Description |
| --- | --- |
| `product` _string_ | Product is a general descriptor for the image. |
| `vendor` _string_ | Vendor describes the organization/user that produced the image. |
| `version` _string_ | Version describes the short-form version of the image. |
| `fullVersion` _string_ | FullVersion describes the long-form version of the image. |

### VirtualMachineImageRef





_Appears in:_
- [VirtualMachineCdromSpec](#virtualmachinecdromspec)
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `kind` _string_ | Kind describes the type of image, either a namespace-scoped
VirtualMachineImage or cluster-scoped ClusterVirtualMachineImage. |
| `name` _string_ | Name refers to the name of a VirtualMachineImage resource in the same
namespace as this VM or a cluster-scoped ClusterVirtualMachineImage. |

### VirtualMachineImageSpec



VirtualMachineImageSpec defines the desired state of VirtualMachineImage.

_Appears in:_
- [ClusterVirtualMachineImage](#clustervirtualmachineimage)
- [VirtualMachineImage](#virtualmachineimage)

| Field | Description |
| --- | --- |
| `providerRef` _[LocalObjectRef](#localobjectref)_ | ProviderRef is a reference to the resource that contains the source of
this image's information. |

### VirtualMachineImageStatus



VirtualMachineImageStatus defines the observed state of VirtualMachineImage.

_Appears in:_
- [ClusterVirtualMachineImage](#clustervirtualmachineimage)
- [VirtualMachineImage](#virtualmachineimage)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the display name of this image. |
| `capabilities` _string array_ | Capabilities describes the image's observed capabilities.

The capabilities are discerned when VM Operator reconciles an image.
If the source of an image is an OVF in Content Library, then the
capabilities are parsed from the OVF property
capabilities.image.vmoperator.vmware.com as a comma-separated list of
values. Well-known capabilities include:

* cloud-init
* nvidia-gpu
* sriov-net

Every capability is also added to the resource's labels as
VirtualMachineImageCapabilityLabel + Value. For example, if the
capability is "cloud-init" then the following label will be added to the
resource: capability.image.vmoperator.vmware.com/cloud-init. |
| `firmware` _string_ | Firmware describe the firmware type used by this image, ex. BIOS, EFI. |
| `hardwareVersion` _integer_ | HardwareVersion describes the observed hardware version of this image. |
| `osInfo` _[VirtualMachineImageOSInfo](#virtualmachineimageosinfo)_ | OSInfo describes the observed operating system information for this
image.

The OS information is also added to the image resource's labels. Please
refer to VirtualMachineImageOSInfo for more information. |
| `ovfProperties` _[OVFProperty](#ovfproperty) array_ | OVFProperties describes the observed user configurable OVF properties defined for this
image. |
| `vmwareSystemProperties` _KeyValuePair array_ | VMwareSystemProperties describes the observed VMware system properties defined for
this image. |
| `productInfo` _[VirtualMachineImageProductInfo](#virtualmachineimageproductinfo)_ | ProductInfo describes the observed product information for this image. |
| `disks` _[VirtualMachineImageDiskInfo](#virtualmachineimagediskinfo) array_ | Disks describes the observed disk information for this image. |
| `providerContentVersion` _string_ | ProviderContentVersion describes the content version from the provider item
that this image corresponds to. If the provider of this image is a Content
Library, this will be the version of the corresponding Content Library item. |
| `providerItemID` _string_ | ProviderItemID describes the ID of the provider item that this image corresponds to.
If the provider of this image is a Content Library, this ID will be that of the
corresponding Content Library item. |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions describes the observed conditions for this image. |
| `type` _string_ | Type describes the content library item type (OVF or ISO) of the image. |

### VirtualMachineNetworkConfigDHCPOptionsStatus



VirtualMachineNetworkConfigDHCPOptionsStatus describes the configured
DHCP options.

_Appears in:_
- [VirtualMachineNetworkConfigDHCPStatus](#virtualmachinenetworkconfigdhcpstatus)

| Field | Description |
| --- | --- |
| `enabled` _boolean_ | Enabled describes whether DHCP is enabled. |

### VirtualMachineNetworkConfigDHCPStatus



VirtualMachineNetworkConfigDHCPStatus describes the configured state of the
system-wide DHCP settings for IP4 and IP6.

_Appears in:_
- [VirtualMachineNetworkConfigInterfaceIPStatus](#virtualmachinenetworkconfiginterfaceipstatus)

| Field | Description |
| --- | --- |
| `ip4` _[VirtualMachineNetworkConfigDHCPOptionsStatus](#virtualmachinenetworkconfigdhcpoptionsstatus)_ | IP4 describes the configured state of the IP4 DHCP settings. |
| `ip6` _[VirtualMachineNetworkConfigDHCPOptionsStatus](#virtualmachinenetworkconfigdhcpoptionsstatus)_ | IP6 describes the configured state of the IP6 DHCP settings. |

### VirtualMachineNetworkConfigDNSStatus



VirtualMachineNetworkConfigDNSStatus describes the configured state of the
RFC 1034 client-side DNS settings.

_Appears in:_
- [VirtualMachineNetworkConfigInterfaceStatus](#virtualmachinenetworkconfiginterfacestatus)
- [VirtualMachineNetworkConfigStatus](#virtualmachinenetworkconfigstatus)

| Field | Description |
| --- | --- |
| `hostName` _string_ | HostName is the host name portion of the DNS name. For example,
the "my-vm" part of "my-vm.domain.local". |
| `domainName` _string_ | DomainName is the domain name portion of the DNS name. For example,
the "domain.local" part of "my-vm.domain.local". |
| `nameservers` _string array_ | Nameservers is a list of the IP addresses for the DNS servers to use.

IP4 addresses are specified using dotted decimal notation. For example,
"192.0.2.1".

IP6 addresses are 128-bit addresses represented as eight fields of up to
four hexadecimal digits. A colon separates each field (:). For example,
2001:DB8:101::230:6eff:fe04:d9ff. The address can also consist of the
symbol '::' to represent multiple 16-bit groups of contiguous 0's only
once in an address as described in RFC 2373. |
| `searchDomains` _string array_ | SearchDomains is a list of domains in which to search for hosts, in the
order of preference. |

### VirtualMachineNetworkConfigInterfaceIPStatus



VirtualMachineNetworkConfigInterfaceIPStatus describes the configured state
of a VM's network interface's IP configuration.

_Appears in:_
- [VirtualMachineNetworkConfigInterfaceStatus](#virtualmachinenetworkconfiginterfacestatus)

| Field | Description |
| --- | --- |
| `dhcp` _[VirtualMachineNetworkConfigDHCPStatus](#virtualmachinenetworkconfigdhcpstatus)_ | DHCP describes the interface's configured DHCP options. |
| `addresses` _string array_ | Addresses describes configured IP addresses for this interface.
Addresses include the network's prefix length, ex. 192.168.0.0/24 or
2001:DB8:101::230:6eff:fe04:d9ff::/64. |
| `gateway4` _string_ | Gateway4 describes the interface's configured, default, IP4 gateway.

Please note the IP address include the network prefix length, ex.
192.168.0.1/24. |
| `gateway6` _string_ | Gateway6 describes the interface's configured, default, IP6 gateway.

Please note the IP address includes the network prefix length, ex.
2001:db8:101::1/64. |

### VirtualMachineNetworkConfigInterfaceStatus



VirtualMachineNetworkConfigInterfaceStatus describes the configured state of
network interface.

_Appears in:_
- [VirtualMachineNetworkConfigStatus](#virtualmachinenetworkconfigstatus)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the corresponding network interface with the same name
in the VM's desired network interface list.

Please note this name is not necessarily related to the name of the
device as it is surfaced inside of the guest. |
| `ip` _[VirtualMachineNetworkConfigInterfaceIPStatus](#virtualmachinenetworkconfiginterfaceipstatus)_ | IP describes the interface's configured IP information. |
| `dns` _[VirtualMachineNetworkConfigDNSStatus](#virtualmachinenetworkconfigdnsstatus)_ | DNS describes the interface's configured DNS information. |

### VirtualMachineNetworkConfigStatus





_Appears in:_
- [VirtualMachineNetworkStatus](#virtualmachinenetworkstatus)

| Field | Description |
| --- | --- |
| `interfaces` _[VirtualMachineNetworkConfigInterfaceStatus](#virtualmachinenetworkconfiginterfacestatus) array_ | Interfaces describes the configured state of the network interfaces. |
| `dns` _[VirtualMachineNetworkConfigDNSStatus](#virtualmachinenetworkconfigdnsstatus)_ | DNS describes the configured state of client-side DNS. |

### VirtualMachineNetworkDHCPOptionsStatus



VirtualMachineNetworkDHCPOptionsStatus describes the observed state of
DHCP options.

_Appears in:_
- [VirtualMachineNetworkDHCPStatus](#virtualmachinenetworkdhcpstatus)

| Field | Description |
| --- | --- |
| `config` _KeyValuePair array_ | Config describes platform-dependent settings for the DHCP client.

The key part is a unique number while the value part is the platform
specific configuration command. For example on Linux and BSD systems
using the file dhclient.conf output would be reported at system scope:
key='1', value='timeout 60;' key='2', value='reboot 10;'. The output
reported per interface would be:
key='1', value='prepend domain-name-servers 192.0.2.1;'
key='2', value='require subnet-mask, domain-name-servers;'. |
| `enabled` _boolean_ | Enabled reports the status of the DHCP client services. |

### VirtualMachineNetworkDHCPStatus



VirtualMachineNetworkDHCPStatus describes the observed state of the
client-side, system-wide DHCP settings for IP4 and IP6.

_Appears in:_
- [VirtualMachineNetworkIPStackStatus](#virtualmachinenetworkipstackstatus)
- [VirtualMachineNetworkInterfaceIPStatus](#virtualmachinenetworkinterfaceipstatus)

| Field | Description |
| --- | --- |
| `ip4` _[VirtualMachineNetworkDHCPOptionsStatus](#virtualmachinenetworkdhcpoptionsstatus)_ | IP4 describes the observed state of the IP4 DHCP client settings. |
| `ip6` _[VirtualMachineNetworkDHCPOptionsStatus](#virtualmachinenetworkdhcpoptionsstatus)_ | IP6 describes the observed state of the IP6 DHCP client settings. |

### VirtualMachineNetworkDNSStatus



VirtualMachineNetworkDNSStatus describes the observed state of the guest's
RFC 1034 client-side DNS settings.

_Appears in:_
- [VirtualMachineNetworkIPStackStatus](#virtualmachinenetworkipstackstatus)
- [VirtualMachineNetworkInterfaceStatus](#virtualmachinenetworkinterfacestatus)

| Field | Description |
| --- | --- |
| `dhcp` _boolean_ | DHCP indicates whether or not dynamic host control protocol (DHCP) was
used to configure DNS configuration. |
| `hostName` _string_ | HostName is the host name portion of the DNS name. For example,
the "my-vm" part of "my-vm.domain.local". |
| `domainName` _string_ | DomainName is the domain name portion of the DNS name. For example,
the "domain.local" part of "my-vm.domain.local". |
| `nameservers` _string array_ | Nameservers is a list of the IP addresses for the DNS servers to use.

IP4 addresses are specified using dotted decimal notation. For example,
"192.0.2.1".

IP6 addresses are 128-bit addresses represented as eight fields of up to
four hexadecimal digits. A colon separates each field (:). For example,
2001:DB8:101::230:6eff:fe04:d9ff. The address can also consist of the
symbol '::' to represent multiple 16-bit groups of contiguous 0's only
once in an address as described in RFC 2373. |
| `searchDomains` _string array_ | SearchDomains is a list of domains in which to search for hosts, in the
order of preference. |

### VirtualMachineNetworkIPRouteGatewayStatus



VirtualMachineNetworkIPRouteGatewayStatus describes the observed state of
a guest network's IP route's next hop gateway.

_Appears in:_
- [VirtualMachineNetworkIPRouteStatus](#virtualmachinenetworkiproutestatus)

| Field | Description |
| --- | --- |
| `device` _string_ | Device is the name of the device in the guest for which this gateway
applies. |
| `address` _string_ | Address is the IP4 or IP6 address of the gateway. |

### VirtualMachineNetworkIPRouteStatus



VirtualMachineNetworkIPRouteStatus describes the observed state of a
guest network's IP routes.

_Appears in:_
- [VirtualMachineNetworkIPStackStatus](#virtualmachinenetworkipstackstatus)
- [VirtualMachineNetworkRouteStatus](#virtualmachinenetworkroutestatus)

| Field | Description |
| --- | --- |
| `gateway` _[VirtualMachineNetworkIPRouteGatewayStatus](#virtualmachinenetworkiproutegatewaystatus)_ | Gateway describes where to send the packets to next. |
| `networkAddress` _string_ | NetworkAddress is the IP4 or IP6 address of the destination network.

Addresses include the network's prefix length, ex. 192.168.0.0/24 or
2001:DB8:101::230:6eff:fe04:d9ff::/64.

IP6 addresses are 128-bit addresses represented as eight fields of up to
four hexadecimal digits. A colon separates each field (:). For example,
2001:DB8:101::230:6eff:fe04:d9ff. The address can also consist of symbol
'::' to represent multiple 16-bit groups of contiguous 0's only once in
an address as described in RFC 2373. |

### VirtualMachineNetworkIPStackStatus



VirtualMachineNetworkIPStackStatus describes the observed state of a
VM's IP stack.

_Appears in:_
- [VirtualMachineNetworkStatus](#virtualmachinenetworkstatus)

| Field | Description |
| --- | --- |
| `dhcp` _[VirtualMachineNetworkDHCPStatus](#virtualmachinenetworkdhcpstatus)_ | DHCP describes the VM's observed, client-side, system-wide DHCP options. |
| `dns` _[VirtualMachineNetworkDNSStatus](#virtualmachinenetworkdnsstatus)_ | DNS describes the VM's observed, client-side DNS configuration. |
| `ipRoutes` _[VirtualMachineNetworkIPRouteStatus](#virtualmachinenetworkiproutestatus) array_ | IPRoutes contain the VM's routing tables for all address families. |
| `kernelConfig` _KeyValuePair array_ | KernelConfig describes the observed state of the VM's kernel IP
configuration settings.

The key part contains a unique number while the value part contains the
'key=value' as provided by the underlying provider. For example, on
Linux and/or BSD, the systcl -a output would be reported as:
key='5', value='net.ipv4.tcp_keepalive_time = 7200'. |

### VirtualMachineNetworkInterfaceIPAddrStatus



VirtualMachineNetworkInterfaceIPAddrStatus describes information about a
specific IP address.

_Appears in:_
- [VirtualMachineNetworkInterfaceIPStatus](#virtualmachinenetworkinterfaceipstatus)

| Field | Description |
| --- | --- |
| `address` _string_ | Address is an IP4 or IP6 address and their network prefix length.

An IP4 address is specified using dotted decimal notation. For example,
"192.0.2.1".

IP6 addresses are 128-bit addresses represented as eight fields of up to
four hexadecimal digits. A colon separates each field (:). For example,
2001:DB8:101::230:6eff:fe04:d9ff. The address can also consist of the
symbol '::' to represent multiple 16-bit groups of contiguous 0's only
once in an address as described in RFC 2373. |
| `lifetime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | Lifetime describes when this address will expire. |
| `origin` _string_ | Origin describes how this address was configured. |
| `state` _string_ | State describes the state of this IP address. |

### VirtualMachineNetworkInterfaceIPStatus



VirtualMachineNetworkInterfaceIPStatus describes the observed state of a
VM's network interface's IP configuration.

_Appears in:_
- [VirtualMachineNetworkInterfaceStatus](#virtualmachinenetworkinterfacestatus)

| Field | Description |
| --- | --- |
| `autoConfigurationEnabled` _boolean_ | AutoConfigurationEnabled describes whether or not ICMPv6 router
solicitation requests are enabled or disabled from a given interface.

These requests acquire an IP6 address and default gateway route from
zero-to-many routers on the connected network.

If not set then ICMPv6 is not available on this VM. |
| `dhcp` _[VirtualMachineNetworkDHCPStatus](#virtualmachinenetworkdhcpstatus)_ | DHCP describes the VM's observed, client-side, interface-specific DHCP
options. |
| `addresses` _[VirtualMachineNetworkInterfaceIPAddrStatus](#virtualmachinenetworkinterfaceipaddrstatus) array_ | Addresses describes observed IP addresses for this interface. |
| `macAddr` _string_ | MACAddr describes the observed MAC address for this interface. |

### VirtualMachineNetworkInterfaceSpec



VirtualMachineNetworkInterfaceSpec describes the desired state of a VM's
network interface.

_Appears in:_
- [VirtualMachineNetworkSpec](#virtualmachinenetworkspec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the unique name of this network interface, used to
distinguish it from other network interfaces attached to this VM.

When the bootstrap provider is Cloud-Init and GuestDeviceName is not
specified, the device inside the guest will be renamed to this value.
Please note it is up to the user to ensure the provided name does not
conflict with any other devices inside the guest, ex. dvd, cdrom, sda, etc. |
| `network` _[PartialObjectRef](#partialobjectref)_ | Network is the name of the network resource to which this interface is
connected.

If no network is provided, then this interface will be connected to the
Namespace's default network. |
| `guestDeviceName` _string_ | GuestDeviceName is used to rename the device inside the guest when the
bootstrap provider is Cloud-Init. Please note it is up to the user to
ensure the provided device name does not conflict with any other devices
inside the guest, ex. dvd, cdrom, sda, etc. |
| `addresses` _string array_ | Addresses is an optional list of IP4 or IP6 addresses to assign to this
interface.

Please note this field is only supported if the connected network
supports manual IP allocation.

Please note IP4 and IP6 addresses must include the network prefix length,
ex. 192.168.0.10/24 or 2001:db8:101::a/64.

Please note this field may not contain IP4 addresses if DHCP4 is set
to true or IP6 addresses if DHCP6 is set to true. |
| `dhcp4` _boolean_ | DHCP4 indicates whether or not this interface uses DHCP for IP4
networking.

Please note this field is only supported if the network connection
supports DHCP.

Please note this field is mutually exclusive with IP4 addresses in the
Addresses field and the Gateway4 field. |
| `dhcp6` _boolean_ | DHCP6 indicates whether or not this interface uses DHCP for IP6
networking.

Please note this field is only supported if the network connection
supports DHCP.

Please note this field is mutually exclusive with IP6 addresses in the
Addresses field and the Gateway6 field. |
| `gateway4` _string_ | Gateway4 is the default, IP4 gateway for this interface.

If unset, the gateway from the network provider will be used. However,
if set to "None", the network provider gateway will be ignored.

Please note this field is only supported if the network connection
supports manual IP allocation.

Please note the IP address must include the network prefix length, ex.
192.168.0.1/24.

Please note this field is mutually exclusive with DHCP4. |
| `gateway6` _string_ | Gateway6 is the primary IP6 gateway for this interface.

If unset, the gateway from the network provider will be used. However,
if set to "None", the network provider gateway will be ignored.

Please note this field is only supported if the network connection
supports manual IP allocation.

Please note the IP address must include the network prefix length, ex.
2001:db8:101::1/64.

Please note this field is mutually exclusive with DHCP6. |
| `mtu` _integer_ | MTU is the Maximum Transmission Unit size in bytes.

Please note this feature is available only with the following bootstrap
providers: CloudInit. |
| `nameservers` _string array_ | Nameservers is a list of IP4 and/or IP6 addresses used as DNS
nameservers.

Please note this feature is available only with the following bootstrap
providers: CloudInit and Sysprep.

When using CloudInit and UseGlobalNameserversAsDefault is either unset or
true, if nameservers is not provided, the global nameservers will be used
instead.

Please note that Linux allows only three nameservers
(https://linux.die.net/man/5/resolv.conf). |
| `routes` _[VirtualMachineNetworkRouteSpec](#virtualmachinenetworkroutespec) array_ | Routes is a list of optional, static routes.

Please note this feature is available only with the following bootstrap
providers: CloudInit. |
| `searchDomains` _string array_ | SearchDomains is a list of search domains used when resolving IP
addresses with DNS.

Please note this feature is available only with the following bootstrap
providers: CloudInit.

When using CloudInit and UseGlobalSearchDomainsAsDefault is either unset
or true, if search domains is not provided, the global search domains
will be used instead. |

### VirtualMachineNetworkInterfaceStatus



VirtualMachineNetworkInterfaceStatus describes the observed state of a
VM's network interface.

_Appears in:_
- [VirtualMachineNetworkStatus](#virtualmachinenetworkstatus)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the corresponding network interface with the same name
in the VM's desired network interface list. If unset, then there is no
corresponding entry for this interface.

Please note this name is not necessarily related to the name of the
device as it is surfaced inside of the guest. |
| `deviceKey` _integer_ | DeviceKey describes the unique hardware device key of this network
interface. |
| `ip` _[VirtualMachineNetworkInterfaceIPStatus](#virtualmachinenetworkinterfaceipstatus)_ | IP describes the observed state of the interface's IP configuration. |
| `dns` _[VirtualMachineNetworkDNSStatus](#virtualmachinenetworkdnsstatus)_ | DNS describes the observed state of the interface's DNS configuration. |

### VirtualMachineNetworkRouteSpec



VirtualMachineNetworkRouteSpec defines a static route for a guest.

_Appears in:_
- [VirtualMachineNetworkInterfaceSpec](#virtualmachinenetworkinterfacespec)

| Field | Description |
| --- | --- |
| `to` _string_ | To is either "default", or an IP4 or IP6 address. |
| `via` _string_ | Via is an IP4 or IP6 address. |
| `metric` _integer_ | Metric is the weight/priority of the route. |


### VirtualMachineNetworkSpec



VirtualMachineNetworkSpec defines a VM's desired network configuration.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `hostName` _string_ | HostName describes the value the guest uses as its host name. If omitted,
the name of the VM will be used.

Please note, this feature is available with the following bootstrap
providers: CloudInit, LinuxPrep, and Sysprep.

This field must adhere to the format specified in RFC-1034, Section 3.5
for DNS labels:

  * The total length is restricted to 63 characters or less.
  * The total length is restricted to 15 characters or less on Windows
    systems.
  * The value may begin with a digit per RFC-1123.
  * Underscores are not allowed.
  * Dashes are permitted, but not at the start or end of the value.
  * Symbol unicode points, such as emoji, are permitted, ex. ✓. However,
    please note that the use of emoji, even where allowed, may not
    compatible with the guest operating system, so it recommended to
    stick with more common characters for this value.
  * The value may be a valid IP4 or IP6 address. Please note, the use of
    an IP address for a host name is not compatible with all guest
    operating systems and is discouraged. Additionally, using an IP
    address for the host name is disallowed if spec.network.domainName is
    non-empty.

Please note, the combined values of spec.network.hostName and
spec.network.domainName may not exceed 255 characters in length. |
| `domainName` _string_ | DomainName describes the value the guest uses as its domain name.

Please note, this feature is available with the following bootstrap
providers: CloudInit, LinuxPrep, and Sysprep.

This field must adhere to the format specified in RFC-1034, Section 3.5
for DNS names:

  * When joined with the host name, the total length is restricted to 255
    characters or less.
  * Individual segments must be 63 characters or less.
  * The top-level domain( ex. ".com"), is at least two letters with no
    special characters.
  * Underscores are not allowed.
  * Dashes are permitted, but not at the start or end of the value.
  * Long, top-level domain names (ex. ".london") are permitted.
  * Symbol unicode points, such as emoji, are disallowed in the top-level
    domain.

Please note, the combined values of spec.network.hostName and
spec.network.domainName may not exceed 255 characters in length.

When deploying a guest running Microsoft Windows, this field describes
the domain the computer should join. |
| `disabled` _boolean_ | Disabled is a flag that indicates whether or not to disable networking
for this VM.

When set to true, the VM is not configured with a default interface nor
any specified from the Interfaces field. |
| `nameservers` _string array_ | Nameservers is a list of IP4 and/or IP6 addresses used as DNS
nameservers. These are applied globally.

Please note global nameservers are only available with the following
bootstrap providers: LinuxPrep and Sysprep. The Cloud-Init bootstrap
provider supports per-interface nameservers. However, when Cloud-Init
is used and UseGlobalNameserversAsDefault is true, the global
nameservers will be used when the per-interface nameservers is not
provided.

Please note that Linux allows only three nameservers
(https://linux.die.net/man/5/resolv.conf). |
| `searchDomains` _string array_ | SearchDomains is a list of search domains used when resolving IP
addresses with DNS. These are applied globally.

Please note global search domains are only available with the following
bootstrap providers: LinuxPrep and Sysprep. The Cloud-Init bootstrap
provider supports per-interface search domains. However, when Cloud-Init
is used and UseGlobalSearchDomainsAsDefault is true, the global search
domains will be used when the per-interface search domains is not provided. |
| `interfaces` _[VirtualMachineNetworkInterfaceSpec](#virtualmachinenetworkinterfacespec) array_ | Interfaces is the list of network interfaces used by this VM.

If the Interfaces field is empty and the Disabled field is false, then
a default interface with the name eth0 will be created.

The maximum number of network interface allowed is 10 because a vSphere
virtual machine may not have more than 10 virtual ethernet card devices. |

### VirtualMachineNetworkStatus



VirtualMachineNetworkStatus defines the observed state of a VM's
network configuration.

_Appears in:_
- [VirtualMachineStatus](#virtualmachinestatus)

| Field | Description |
| --- | --- |
| `config` _[VirtualMachineNetworkConfigStatus](#virtualmachinenetworkconfigstatus)_ | Config describes the resolved, configured network settings for the VM,
such as an interface's IP address obtained from IPAM, or global DNS
settings.

Please note this information does *not* represent the *observed* network
state of the VM, but is intended for situations where someone boots a VM
with no appropriate bootstrap engine and needs to know the network config
valid for the deployed VM. |
| `hostName` _string_ | HostName describes the observed hostname reported by the VirtualMachine's
guest operating system.

Please note, this value is only reported if VMware Tools is installed in
the guest, and the value may or may not be a fully qualified domain name
(FQDN), it simply depends on what is reported by the guest. |
| `interfaces` _[VirtualMachineNetworkInterfaceStatus](#virtualmachinenetworkinterfacestatus) array_ | Interfaces describes the status of the VM's network interfaces. |
| `ipStacks` _[VirtualMachineNetworkIPStackStatus](#virtualmachinenetworkipstackstatus) array_ | IPStacks describes information about the guest's configured IP networking
stacks. |
| `primaryIP4` _string_ | PrimaryIP4 describes the VM's primary IP4 address.

If the bootstrap provider is CloudInit then this value is set to the
value of the VM's "guestinfo.local-ipv4" property. Please see
https://bit.ly/3NJB534 for more information on how this value is
calculated.

If the bootstrap provider is anything else then this field is set to the
value of the infrastructure VM's "guest.ipAddress" field. Please see
https://bit.ly/3Au0jM4 for more information. |
| `primaryIP6` _string_ | PrimaryIP6 describes the VM's primary IP6 address.

If the bootstrap provider is CloudInit then this value is set to the
value of the VM's "guestinfo.local-ipv6" property. Please see
https://bit.ly/3NJB534 for more information on how this value is
calculated.

If the bootstrap provider is anything else then this field is set to the
value of the infrastructure VM's "guest.ipAddress" field. Please see
https://bit.ly/3Au0jM4 for more information. |

### VirtualMachinePlacementStatus





_Appears in:_
- [VirtualMachineGroupMemberStatus](#virtualmachinegroupmemberstatus)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of VirtualMachine member of this group. |
| `zoneID` _string_ | Zone describes the recommended zone for this VM. |
| `node` _string_ | Node describes the recommended node for this VM. |
| `pool` _string_ | Pool describes the recommended resource pool for this VM. |
| `datastores` _[VirtualMachineGroupPlacementDatastoreStatus](#virtualmachinegroupplacementdatastorestatus) array_ | Datastores describe the recommended datastores for this VM. |

### VirtualMachinePowerOpMode

_Underlying type:_ `string`

VirtualMachinePowerOpMode represents the various power operation modes when
powering off or suspending a VM.

_Appears in:_
- [VirtualMachineGroupSpec](#virtualmachinegroupspec)
- [VirtualMachineSpec](#virtualmachinespec)


### VirtualMachinePowerState

_Underlying type:_ `string`

VirtualMachinePowerState defines a VM's desired and observed power states.

_Appears in:_
- [VirtualMachineGroupMemberStatus](#virtualmachinegroupmemberstatus)
- [VirtualMachineGroupSpec](#virtualmachinegroupspec)
- [VirtualMachineSnapshotStatus](#virtualmachinesnapshotstatus)
- [VirtualMachineSpec](#virtualmachinespec)
- [VirtualMachineStatus](#virtualmachinestatus)


### VirtualMachinePromoteDisksMode

_Underlying type:_ `string`

VirtualMachinePromoteDisksMode represents the available modes for promoting
child disks to full clones.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)


### VirtualMachinePublishRequestSource



VirtualMachinePublishRequestSource is the source of a publication request,
typically a VirtualMachine resource.

_Appears in:_
- [VirtualMachinePublishRequestSpec](#virtualmachinepublishrequestspec)
- [VirtualMachinePublishRequestStatus](#virtualmachinepublishrequeststatus)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of the referenced object.

If omitted this value defaults to the name of the
VirtualMachinePublishRequest resource. |
| `apiVersion` _string_ | APIVersion is the API version of the referenced object. |
| `kind` _string_ | Kind is the kind of referenced object. |

### VirtualMachinePublishRequestSpec



VirtualMachinePublishRequestSpec defines the desired state of a
VirtualMachinePublishRequest.

All the fields in this spec are optional. This is especially useful when a
DevOps persona wants to publish a VM without doing anything more than
applying a VirtualMachinePublishRequest resource that has the same name
as said VM in the same namespace as said VM.

_Appears in:_
- [VirtualMachinePublishRequest](#virtualmachinepublishrequest)

| Field | Description |
| --- | --- |
| `source` _[VirtualMachinePublishRequestSource](#virtualmachinepublishrequestsource)_ | Source is the source of the publication request, ex. a VirtualMachine
resource.

If this value is omitted then the publication controller checks to
see if there is a resource with the same name as this
VirtualMachinePublishRequest resource, an API version equal to
spec.source.apiVersion, and a kind equal to spec.source.kind. If such
a resource exists, then it is the source of the publication. |
| `target` _[VirtualMachinePublishRequestTarget](#virtualmachinepublishrequesttarget)_ | Target is the target of the publication request, ex. item
information and a ContentLibrary resource.

If this value is omitted, the controller uses spec.source.name + "-image"
as the name of the published item. Additionally, when omitted the
controller attempts to identify the target location by matching a
resource with an API version equal to spec.target.location.apiVersion, a
kind equal to spec.target.location.kind, w/ the label
"imageregistry.vmware.com/default".

Please note that while optional, if a VirtualMachinePublishRequest sans
target information is applied to a namespace without a default
publication target, then the VirtualMachinePublishRequest resource
will be marked in error. |
| `ttlSecondsAfterFinished` _integer_ | TTLSecondsAfterFinished is the time-to-live duration for how long this
resource will be allowed to exist once the publication operation
completes. After the TTL expires, the resource will be automatically
deleted without the user having to take any direct action.

If this field is unset then the request resource will not be
automatically deleted. If this field is set to zero then the request
resource is eligible for deletion immediately after it finishes. |

### VirtualMachinePublishRequestStatus



VirtualMachinePublishRequestStatus defines the observed state of a
VirtualMachinePublishRequest.

_Appears in:_
- [VirtualMachinePublishRequest](#virtualmachinepublishrequest)

| Field | Description |
| --- | --- |
| `sourceRef` _[VirtualMachinePublishRequestSource](#virtualmachinepublishrequestsource)_ | SourceRef is the reference to the source of the publication request,
ex. a VirtualMachine resource. |
| `targetRef` _[VirtualMachinePublishRequestTarget](#virtualmachinepublishrequesttarget)_ | TargetRef is the reference to the target of the publication request,
ex. item information and a ContentLibrary resource. |
| `completionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | CompletionTime represents time when the request was completed. It is not
guaranteed to be set in happens-before order across separate operations.
It is represented in RFC3339 form and is in UTC.

The value of this field should be equal to the value of the
LastTransitionTime for the status condition Type=Complete. |
| `startTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | StartTime represents time when the request was acknowledged by the
controller. It is not guaranteed to be set in happens-before order
across separate operations. It is represented in RFC3339 form and is
in UTC. |
| `attempts` _integer_ | Attempts represents the number of times the request to publish the VM
has been attempted. |
| `lastAttemptTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | LastAttemptTime represents the time when the latest request was sent. |
| `imageName` _string_ | ImageName is the name of the VirtualMachineImage resource that is
eventually realized in the same namespace as the VM and publication
request after the publication operation completes.

This field will not be set until the VirtualMachineImage resource
is realized. |
| `ready` _boolean_ | Ready is set to true only when the VM has been published successfully
and the new VirtualMachineImage resource is ready.

Readiness is determined by waiting until there is status condition
Type=Complete and ensuring it and all other status conditions present
have a Status=True. The conditions present will be:

  * SourceValid
  * TargetValid
  * Uploaded
  * ImageAvailable
  * Complete |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions is a list of the latest, available observations of the
request's current state. |

### VirtualMachinePublishRequestTarget



VirtualMachinePublishRequestTarget is the target of a publication request,
typically a ContentLibrary resource.

_Appears in:_
- [VirtualMachinePublishRequestSpec](#virtualmachinepublishrequestspec)
- [VirtualMachinePublishRequestStatus](#virtualmachinepublishrequeststatus)

| Field | Description |
| --- | --- |
| `item` _[VirtualMachinePublishRequestTargetItem](#virtualmachinepublishrequesttargetitem)_ | Item contains information about the name of the object to which
the VM is published.

Please note this value is optional and if omitted, the controller
will use spec.source.name + "-image" as the name of the published
item. |
| `location` _[VirtualMachinePublishRequestTargetLocation](#virtualmachinepublishrequesttargetlocation)_ | Location contains information about the location to which to publish
the VM. |

### VirtualMachinePublishRequestTargetItem



VirtualMachinePublishRequestTargetItem is the item part of a
publication request's target.

_Appears in:_
- [VirtualMachinePublishRequestTarget](#virtualmachinepublishrequesttarget)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of the published object.

If the spec.target.location.apiVersion equals
imageregistry.vmware.com/v1alpha1 and the spec.target.location.kind
equals ContentLibrary, then this should be the name that will
show up in vCenter Content Library, not the custom resource name
in the namespace.

If omitted then the controller will use spec.source.name + "-image". |
| `description` _string_ | Description is the description to assign to the published object. |

### VirtualMachinePublishRequestTargetLocation



VirtualMachinePublishRequestTargetLocation is the location part of a
publication request's target.

_Appears in:_
- [VirtualMachinePublishRequestTarget](#virtualmachinepublishrequesttarget)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of the referenced object.

Please note an error will be returned if this field is not
set in a namespace that lacks a default publication target.

A default publication target is a resource with an API version
equal to spec.target.location.apiVersion, a kind equal to
spec.target.location.kind, and has the label
"imageregistry.vmware.com/default". |
| `apiVersion` _string_ | APIVersion is the API version of the referenced object. |
| `kind` _string_ | Kind is the kind of referenced object. |

### VirtualMachineReadinessProbeSpec



VirtualMachineReadinessProbeSpec describes a probe used to determine if a VM
is in a ready state. All probe actions are mutually exclusive.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `tcpSocket` _[TCPSocketAction](#tcpsocketaction)_ | TCPSocket specifies an action involving a TCP port.

Deprecated: The TCPSocket action requires network connectivity that is not supported in all environments.
This field will be removed in a later API version. |
| `guestHeartbeat` _[GuestHeartbeatAction](#guestheartbeataction)_ | GuestHeartbeat specifies an action involving the guest heartbeat status. |
| `guestInfo` _[GuestInfoAction](#guestinfoaction) array_ | GuestInfo specifies an action involving key/value pairs from GuestInfo.

The elements are evaluated with the logical AND operator, meaning
all expressions must evaluate as true for the probe to succeed.

For example, a VM resource's probe definition could be specified as the
following:

        guestInfo:
        - key:   ready
          value: true

With the above configuration in place, the VM would not be considered
ready until the GuestInfo key "ready" was set to the value "true".

From within the guest operating system it is possible to set GuestInfo
key/value pairs using the program "vmware-rpctool," which is included
with VM Tools. For example, the following command will set the key
"guestinfo.ready" to the value "true":

        vmware-rpctool "info-set guestinfo.ready true"

Once executed, the VM's readiness probe will be signaled and the
VM resource will be marked as ready. |
| `timeoutSeconds` _integer_ | TimeoutSeconds specifies a number of seconds after which the probe times out.
Defaults to 10 seconds. Minimum value is 1. |
| `periodSeconds` _integer_ | PeriodSeconds specifics how often (in seconds) to perform the probe.
Defaults to 10 seconds. Minimum value is 1. |

### VirtualMachineReplicaSetSpec



VirtualMachineReplicaSetSpec is the specification of a VirtualMachineReplicaSet.

_Appears in:_
- [VirtualMachineReplicaSet](#virtualmachinereplicaset)

| Field | Description |
| --- | --- |
| `replicas` _integer_ | Replicas is the number of desired replicas.
This is a pointer to distinguish between explicit zero and unspecified.
Defaults to 1. |
| `deletePolicy` _string_ | DeletePolicy defines the policy used to identify nodes to delete when downscaling.
Only supported deletion policy is "Random". |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#labelselector-v1-meta)_ | Selector is a label to query over virtual machines that should match the
replica count. A virtual machine's label keys and values must match in order
to be controlled by this VirtualMachineReplicaSet.

It must match the VirtualMachine template's labels.
More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors |
| `template` _[VirtualMachineTemplateSpec](#virtualmachinetemplatespec)_ | Template is the object that describes the virtual machine that will be
created if insufficient replicas are detected. |

### VirtualMachineReplicaSetStatus



VirtualMachineReplicaSetStatus represents the observed state of a
VirtualMachineReplicaSet resource.

_Appears in:_
- [VirtualMachineReplicaSet](#virtualmachinereplicaset)

| Field | Description |
| --- | --- |
| `replicas` _integer_ | Replicas is the most recently observed number of replicas. |
| `fullyLabeledReplicas` _integer_ | FullyLabeledReplicas is the number of replicas that have labels matching the
labels of the virtual machine template of the VirtualMachineReplicaSet. |
| `readyReplicas` _integer_ | ReadyReplicas is the number of ready replicas for this VirtualMachineReplicaSet. A
virtual machine is considered ready when it's "Ready" condition is marked as
true. |
| `observedGeneration` _integer_ | ObservedGeneration reflects the generation of the most recently observed
VirtualMachineReplicaSet. |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions represents the latest available observations of a
VirtualMachineReplicaSet's current state. |

### VirtualMachineReservedSpec



VirtualMachineReservedSpec describes a set of VM configuration options
reserved for system use. Modification attempts by DevOps users will result
in a validation error.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `resourcePolicyName` _string_ |  |

### VirtualMachineResourceSpec



VirtualMachineResourceSpec describes a virtual hardware policy specification.

_Appears in:_
- [ResourcePoolSpec](#resourcepoolspec)
- [VirtualMachineClassResources](#virtualmachineclassresources)

| Field | Description |
| --- | --- |
| `cpu` _[Quantity](#quantity)_ |  |
| `memory` _[Quantity](#quantity)_ |  |

### VirtualMachineServicePort



VirtualMachineServicePort describes the specification of a service port to
be exposed by a VirtualMachineService. This VirtualMachineServicePort
specification includes attributes that define the external and internal
representation of the service port.

_Appears in:_
- [VirtualMachineServiceSpec](#virtualmachineservicespec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the name to be used to identify this
VirtualMachineServicePort. |
| `protocol` _string_ | Protocol describes the Layer 4 transport protocol for this port.
Supports "TCP", "UDP", and "SCTP". |
| `port` _integer_ | Port describes the external port that will be exposed by the service. |
| `targetPort` _integer_ | TargetPort describes the internal port open on a VirtualMachine that
should be mapped to the external Port. |

### VirtualMachineServiceSpec



VirtualMachineServiceSpec defines the desired state of VirtualMachineService.

_Appears in:_
- [VirtualMachineService](#virtualmachineservice)

| Field | Description |
| --- | --- |
| `type` _[VirtualMachineServiceType](#virtualmachineservicetype)_ | Type specifies a desired VirtualMachineServiceType for this
VirtualMachineService. Supported types are ClusterIP, LoadBalancer,
ExternalName. |
| `ports` _[VirtualMachineServicePort](#virtualmachineserviceport) array_ | Ports specifies a list of VirtualMachineServicePort to expose with this
VirtualMachineService. Each of these ports will be an accessible network
entry point to access this service by. |
| `selector` _object (keys:string, values:string)_ | Selector specifies a map of key-value pairs, also known as a Label
Selector, that is used to match this VirtualMachineService with the set
of VirtualMachines that should back this VirtualMachineService. |
| `loadBalancerIP` _string_ | LoadBalancer will get created with the IP specified in this field.
Only applies to VirtualMachineService Type: LoadBalancer
This feature depends on whether the underlying load balancer provider
supports specifying the loadBalancerIP when a load balancer is created.
This field will be ignored if the provider does not support the feature.
Deprecated: This field was under-specified and its meaning varies across implementations.
Using it is non-portable and it may not support dual-stack.
Users are encouraged to use implementation-specific annotations when available. |
| `loadBalancerSourceRanges` _string array_ | LoadBalancerSourceRanges is an array of IP addresses in the format of
CIDRs, for example: 103.21.244.0/22 and 10.0.0.0/24.
If specified and supported by the load balancer provider, this will
restrict ingress traffic to the specified client IPs. This field will be
ignored if the provider does not support the feature. |
| `clusterIp` _string_ | ClusterIP is the IP address of the service and is usually assigned
randomly by the master. If an address is specified manually and is not in
use by others, it will be allocated to the service; otherwise, creation
of the service will fail. This field can not be changed through updates.
Valid values are "None", empty string (""), or a valid IP address. "None"
can be specified for headless services when proxying is not required.
Only applies to types ClusterIP and LoadBalancer.
Ignored if type is ExternalName.
More info: https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies |
| `externalName` _string_ | ExternalName is the external reference that kubedns or equivalent will
return as a CNAME record for this service. No proxying will be involved.
Must be a valid RFC-1123 hostname (https://tools.ietf.org/html/rfc1123)
and requires Type to be ExternalName. |

### VirtualMachineServiceStatus



VirtualMachineServiceStatus defines the observed state of
VirtualMachineService.

_Appears in:_
- [VirtualMachineService](#virtualmachineservice)

| Field | Description |
| --- | --- |
| `loadBalancer` _[LoadBalancerStatus](#loadbalancerstatus)_ | LoadBalancer contains the current status of the load balancer,
if one is present. |

### VirtualMachineServiceType

_Underlying type:_ `string`

VirtualMachineServiceType string describes ingress methods for a service.

_Appears in:_
- [VirtualMachineServiceSpec](#virtualmachineservicespec)


### VirtualMachineSetResourcePolicySpec



VirtualMachineSetResourcePolicySpec defines the desired state of
VirtualMachineSetResourcePolicy.

_Appears in:_
- [VirtualMachineSetResourcePolicy](#virtualmachinesetresourcepolicy)

| Field | Description |
| --- | --- |
| `resourcePool` _[ResourcePoolSpec](#resourcepoolspec)_ |  |
| `folder` _string_ |  |
| `clusterModuleGroups` _string array_ |  |

### VirtualMachineSetResourcePolicyStatus



VirtualMachineSetResourcePolicyStatus defines the observed state of
VirtualMachineSetResourcePolicy.

_Appears in:_
- [VirtualMachineSetResourcePolicy](#virtualmachinesetresourcepolicy)

| Field | Description |
| --- | --- |
| `clustermodules` _[VSphereClusterModuleStatus](#vsphereclustermodulestatus) array_ |  |

### VirtualMachineSnapshotSpec



VirtualMachineSnapshotSpec defines the desired state of VirtualMachineSnapshot.

_Appears in:_
- [VirtualMachineSnapshot](#virtualmachinesnapshot)

| Field | Description |
| --- | --- |
| `memory` _boolean_ | Memory represents whether the snapshot includes the VM's
memory. If true, a dump of the internal state of the virtual
machine (a memory dump) is included in the snapshot. Memory
snapshots consume time and resources and thus, take longer to
create.
The virtual machine must support this capability.
When set to false, the power state of the snapshot is set to
false.
For a VM in suspended state, memory is always included
in the snashot. |
| `quiesce` _[QuiesceSpec](#quiescespec)_ | Quiesce represents the spec used for granular control over
quiesce details. If quiesceSpec is set and the virtual machine
is powered on when the snapshot is taken, VMware Tools is used
to quiesce the file system in the virtual machine. This assures
that a disk snapshot represents a consistent state of the guest
file systems. If the virtual machine is powered off or VMware
Tools are not available, the quiesce spec is ignored. |
| `description` _string_ | Description represents a description of the snapshot. |
| `vmRef` _[LocalObjectRef](#localobjectref)_ | VMRef represents the name of the virtual machine for which the
snapshot is requested. |

### VirtualMachineSnapshotStatus



VirtualMachineSnapshotStatus defines the observed state of VirtualMachineSnapshot.

_Appears in:_
- [VirtualMachineSnapshot](#virtualmachinesnapshot)

| Field | Description |
| --- | --- |
| `powerState` _[VirtualMachinePowerState](#virtualmachinepowerstate)_ | PowerState represents the observed power state of the virtual
machine when the snapshot was taken. |
| `quiesced` _boolean_ | Quiesced represents whether or not the snapshot was created
with the quiesce option to ensure a snapshot with a consistent
state of the guest file system. |
| `uniqueID` _string_ | UniqueID describes a unique identifier provider by the backing
infrastructure (e.g., vSphere) that can be used to distinguish
this snapshot from other snapshots of this virtual machine. |
| `children` _LocalObjectRef array_ | Children represents the snapshots for which this snapshot is
the parent. |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions describes the observed conditions of the VirtualMachine. |

### VirtualMachineSpec



VirtualMachineSpec defines the desired state of a VirtualMachine.

_Appears in:_
- [VirtualMachine](#virtualmachine)
- [VirtualMachineTemplateSpec](#virtualmachinetemplatespec)

| Field | Description |
| --- | --- |
| `cdrom` _[VirtualMachineCdromSpec](#virtualmachinecdromspec) array_ | Cdrom describes the desired state of the VM's CD-ROM devices.

Each CD-ROM device requires a reference to an ISO-type
VirtualMachineImage or ClusterVirtualMachineImage resource as backing.

Multiple CD-ROM devices using the same backing image, regardless of image
kinds (namespace or cluster scope), are not allowed.

CD-ROM devices can be added, updated, or removed when the VM is powered
off. When the VM is powered on, only the connection state of existing
CD-ROM devices can be changed.
CD-ROM devices are attached to the VM in the specified list-order. |
| `image` _[VirtualMachineImageRef](#virtualmachineimageref)_ | Image describes the reference to the VirtualMachineImage or
ClusterVirtualMachineImage resource used to deploy this VM.

Please note, unlike the field spec.imageName, the value of
spec.image.name MUST be a Kubernetes object name.

Please also note, when creating a new VirtualMachine, if this field and
spec.imageName are both non-empty, then they must refer to the same
resource or an error is returned.

Please note, this field *may* be empty if the VM was imported instead of
deployed by VM Operator. An imported VirtualMachine resource references
an existing VM on the underlying platform that was not deployed from a
VM image. |
| `imageName` _string_ | ImageName describes the name of the image resource used to deploy this
VM.

This field may be used to specify the name of a VirtualMachineImage
or ClusterVirtualMachineImage resource. The resolver first checks to see
if there is a VirtualMachineImage with the specified name in the
same namespace as the VM being deployed. If no such resource exists, the
resolver then checks to see if there is a ClusterVirtualMachineImage
resource with the specified name.

This field may also be used to specify the display name (vSphere name) of
a VirtualMachineImage or ClusterVirtualMachineImage resource. If the
display name unambiguously resolves to a distinct VM image (among all
existing VirtualMachineImages in the VM's namespace and all existing
ClusterVirtualMachineImages), then a mutation webhook updates the
spec.image field with the reference to the resolved VM image. If the
display name resolves to multiple or no VM images, then the mutation
webhook denies the request and returns an error.

Please also note, when creating a new VirtualMachine, if this field and
spec.image are both non-empty, then they must refer to the same
resource or an error is returned.

Please note, this field *may* be empty if the VM was imported instead of
deployed by VM Operator. An imported VirtualMachine resource references
an existing VM on the underlying platform that was not deployed from a
VM image. |
| `className` _string_ | ClassName describes the name of the VirtualMachineClass resource used to
deploy this VM.

Please note, this field *may* be empty if the VM was imported instead of
deployed by VM Operator. An imported VirtualMachine resource references
an existing VM on the underlying platform that was not deployed from a
VM class. |
| `affinity` _[VirtualMachineAffinitySpec](#virtualmachineaffinityspec)_ | Affinity describes the VM's scheduling constraints. |
| `crypto` _[VirtualMachineCryptoSpec](#virtualmachinecryptospec)_ | Crypto describes the desired encryption state of the VirtualMachine. |
| `storageClass` _string_ | StorageClass describes the name of a Kubernetes StorageClass resource
used to configure this VM's storage-related attributes.

Please see https://kubernetes.io/docs/concepts/storage/storage-classes/
for more information on Kubernetes storage classes. |
| `bootstrap` _[VirtualMachineBootstrapSpec](#virtualmachinebootstrapspec)_ | Bootstrap describes the desired state of the guest's bootstrap
configuration.

If omitted, a default bootstrap method may be selected based on the
guest OS identifier. If Linux, then the LinuxPrep method is used. |
| `network` _[VirtualMachineNetworkSpec](#virtualmachinenetworkspec)_ | Network describes the desired network configuration for the VM.

Please note this value may be omitted entirely and the VM will be
assigned a single, virtual network interface that is connected to the
Namespace's default network. |
| `powerState` _[VirtualMachinePowerState](#virtualmachinepowerstate)_ | PowerState describes the desired power state of a VirtualMachine.

Please note this field may be omitted when creating a new VM and will
default to "PoweredOn." However, once the field is set to a non-empty
value, it may no longer be set to an empty value.

Additionally, setting this value to "Suspended" is not supported when
creating a new VM. The valid values when creating a new VM are
"PoweredOn" and "PoweredOff." An empty value is also allowed on create
since this value defaults to "PoweredOn" for new VMs. |
| `powerOffMode` _[VirtualMachinePowerOpMode](#virtualmachinepoweropmode)_ | PowerOffMode describes the desired behavior when powering off a VM.

There are three, supported power off modes: Hard, Soft, and
TrySoft. The first mode, Hard, is the equivalent of a physical
system's power cord being ripped from the wall. The Soft mode
requires the VM's guest to have VM Tools installed and attempts to
gracefully shutdown the VM. Its variant, TrySoft, first attempts
a graceful shutdown, and if that fails or the VM is not in a powered off
state after five minutes, the VM is halted.

If omitted, the mode defaults to TrySoft. |
| `suspendMode` _[VirtualMachinePowerOpMode](#virtualmachinepoweropmode)_ | SuspendMode describes the desired behavior when suspending a VM.

There are three, supported suspend modes: Hard, Soft, and
TrySoft. The first mode, Hard, is where vSphere suspends the VM to
disk without any interaction inside of the guest. The Soft mode
requires the VM's guest to have VM Tools installed and attempts to
gracefully suspend the VM. Its variant, TrySoft, first attempts
a graceful suspend, and if that fails or the VM is not in a put into
standby by the guest after five minutes, the VM is suspended.

If omitted, the mode defaults to TrySoft. |
| `nextRestartTime` _string_ | NextRestartTime may be used to restart the VM, in accordance with
RestartMode, by setting the value of this field to "now"
(case-insensitive).

A mutating webhook changes this value to the current time (UTC), which
the VM controller then uses to determine the VM should be restarted by
comparing the value to the timestamp of the last time the VM was
restarted.

Please note it is not possible to schedule future restarts using this
field. The only value that users may set is the string "now"
(case-insensitive). |
| `restartMode` _[VirtualMachinePowerOpMode](#virtualmachinepoweropmode)_ | RestartMode describes the desired behavior for restarting a VM when
spec.nextRestartTime is set to "now" (case-insensitive).

There are three, supported suspend modes: Hard, Soft, and
TrySoft. The first mode, Hard, is where vSphere resets the VM without any
interaction inside of the guest. The Soft mode requires the VM's guest to
have VM Tools installed and asks the guest to restart the VM. Its
variant, TrySoft, first attempts a soft restart, and if that fails or
does not complete within five minutes, the VM is hard reset.

If omitted, the mode defaults to TrySoft. |
| `volumes` _[VirtualMachineVolume](#virtualmachinevolume) array_ | Volumes describes a list of volumes that can be mounted to the VM. |
| `readinessProbe` _[VirtualMachineReadinessProbeSpec](#virtualmachinereadinessprobespec)_ | ReadinessProbe describes a probe used to determine the VM's ready state. |
| `advanced` _[VirtualMachineAdvancedSpec](#virtualmachineadvancedspec)_ | Advanced describes a set of optional, advanced VM configuration options. |
| `reserved` _[VirtualMachineReservedSpec](#virtualmachinereservedspec)_ | Reserved describes a set of VM configuration options reserved for system
use.

Please note attempts to modify the value of this field by a DevOps user
will result in a validation error. |
| `minHardwareVersion` _integer_ | MinHardwareVersion describes the desired, minimum hardware version.

The logic that determines the hardware version is as follows:

1. If this field is set, then its value is used.
2. Otherwise, if the VirtualMachineClass used to deploy the VM contains a
   non-empty hardware version, then it is used.
3. Finally, if the hardware version is still undetermined, the value is
   set to the default hardware version for the Datacenter/Cluster/Host
   where the VM is provisioned.

This field is never updated to reflect the derived hardware version.
Instead, VirtualMachineStatus.HardwareVersion surfaces
the observed hardware version.

Please note, setting this field's value to N ensures a VM's hardware
version is equal to or greater than N. For example, if a VM's observed
hardware version is 10 and this field's value is 13, then the VM will be
upgraded to hardware version 13. However, if the observed hardware
version is 17 and this field's value is 13, no change will occur.

Several features are hardware version dependent, for example:

* NVMe Controllers                >= 14
* Dynamic Direct Path I/O devices >= 17

Please refer to https://kb.vmware.com/s/article/1003746 for a list of VM
hardware versions.

It is important to remember that a VM's hardware version may not be
downgraded and upgrading a VM deployed from an image based on an older
hardware version to a more recent one may result in unpredictable
behavior. In other words, please be careful when choosing to upgrade a
VM to a newer hardware version. |
| `instanceUUID` _string_ | InstanceUUID describes the desired Instance UUID for a VM.
If omitted, this field defaults to a random UUID.
This value is only used for the VM Instance UUID,
it is not used within cloudInit.
This identifier is used by VirtualCenter to uniquely identify all
virtual machine instances, including those that may share the same BIOS UUID. |
| `biosUUID` _string_ | BiosUUID describes the desired BIOS UUID for a VM.
If omitted, this field defaults to a random UUID.
When the bootstrap provider is Cloud-Init, this value is used as the
default value for spec.bootstrap.cloudInit.instanceID if it is omitted. |
| `guestID` _string_ | GuestID describes the desired guest operating system identifier for a VM.

The logic that determines the guest ID is as follows:

If this field is set, then its value is used.
Otherwise, if the VM is deployed from an OVF template that defines a
guest ID, then that value is used.
The guest ID from VirtualMachineClass used to deploy the VM is ignored.

For a complete list of supported values, please refer to
https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.vm.GuestOsDescriptor.GuestOsIdentifier.html.

Please note that some guest ID values may require a minimal hardware
version, which can be set using the `spec.minHardwareVersion` field.
To see the mapping between virtual hardware versions and the product
versions that support a specific guest ID, please refer to
https://knowledge.broadcom.com/external/article/315655/virtual-machine-hardware-versions.html.

Please note that this field is immutable after the VM is powered on.
To change the guest ID after the VM is powered on, the VM must be powered
off and then powered on again with the updated guest ID spec.

This field is required when the VM has any CD-ROM devices attached. |
| `promoteDisksMode` _[VirtualMachinePromoteDisksMode](#virtualmachinepromotedisksmode)_ | PromoteDisksMode describes the mode used to promote a VM's delta disks to
full disks. The available modes are:

- Disabled -- Do not promote disks.
- Online   -- Promote disks while the VM is powered on. VMs with
              snapshots do not support online promotion.
- Offline  -- Promote disks while the VM is powered off.

Defaults to Online. |
| `bootOptions` _[VirtualMachineBootOptions](#virtualmachinebootoptions)_ | BootOptions describes the settings that control the boot behavior of the
virtual machine. These settings take effect during the next power-on of the
virtual machine. |
| `currentSnapshot` _[LocalObjectRef](#localobjectref)_ | CurrentSnapshot represents the desired snapshot that the
virtual machine should point to. This field can have three
possible values:

- The value of this field is nil when the working snapshot is at
  the root of the snapshot tree.

- When a new snapshot is requested by creating a new
  VirtualMachineSnapshot, the value of this field is set to the
  new snapshot resource's name.

- If the value of this field is set to an existing snapshot that
  is different from the status.currentSnapshot, the virtual machine is
  reverted to the requested snapshot.

Reverting a virtual machine to a snapshot rolls back the data
and the configuration of the virtual machine to that of the
specified snapshot. The VirtualMachineSpec of the
VirtualMachine resource is replaced from the one stored with
the snapshot.

If the virtual machine is currently powered off, but you revert to
a snapshot that was taken while the VM was powered on, then the
VM will be automatically powered on during the revert.
Additionally, the VirtualMachineSpec will be updated to match
the power state from the snapshot (i.e., powered on). This can
be overridden by specifying the PowerState to PoweredOff in the
VirtualMachineSpec. |
| `groupName` _string_ | GroupName indicates the name of the VirtualMachineGroup to which this
VM belongs.

VMs that belong to a group do not drive their own placement, rather that
is handled by the group. |

### VirtualMachineStatus



VirtualMachineStatus defines the observed state of a VirtualMachine instance.

_Appears in:_
- [VirtualMachine](#virtualmachine)

| Field | Description |
| --- | --- |
| `class` _[LocalObjectRef](#localobjectref)_ | Class is a reference to the VirtualMachineClass resource used to deploy
this VM. |
| `nodeName` _string_ | NodeName describes the observed name of the node where the VirtualMachine
is scheduled. |
| `powerState` _[VirtualMachinePowerState](#virtualmachinepowerstate)_ | PowerState describes the observed power state of the VirtualMachine. |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions describes the observed conditions of the VirtualMachine. |
| `crypto` _[VirtualMachineCryptoStatus](#virtualmachinecryptostatus)_ | Crypto describes the observed state of the VirtualMachine's encryption
configuration. |
| `network` _[VirtualMachineNetworkStatus](#virtualmachinenetworkstatus)_ | Network describes the observed state of the VM's network configuration.
Please note much of the network status information is only available if
the guest has VM Tools installed. |
| `uniqueID` _string_ | UniqueID describes a unique identifier that is provided by the underlying
infrastructure provider, such as vSphere. |
| `biosUUID` _string_ | BiosUUID describes a unique identifier provided by the underlying
infrastructure provider that is exposed to the Guest OS BIOS as a unique
hardware identifier. |
| `instanceUUID` _string_ | InstanceUUID describes the unique instance UUID provided by the
underlying infrastructure provider, such as vSphere. |
| `volumes` _[VirtualMachineVolumeStatus](#virtualmachinevolumestatus) array_ | Volumes describes the observed state of the volumes that are intended to
be attached to the VirtualMachine. |
| `changeBlockTracking` _boolean_ | ChangeBlockTracking describes whether or not change block tracking is
enabled for the VirtualMachine. |
| `zone` _string_ | Zone describes the availability zone where the VirtualMachine has been
scheduled.

Please note this field may be empty when the cluster is not zone-aware. |
| `lastRestartTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | LastRestartTime describes the last time the VM was restarted. |
| `hardwareVersion` _integer_ | HardwareVersion describes the VirtualMachine resource's observed
hardware version.

Please refer to VirtualMachineSpec.MinHardwareVersion for more
information on the topic of a VM's hardware version. |
| `storage` _[VirtualMachineStorageStatus](#virtualmachinestoragestatus)_ | Storage describes the observed state of the VirtualMachine's storage. |
| `taskID` _string_ | TaskID describes the observed ID of the task created by VM Operator to
perform some long-running operation on the VM. |
| `currentSnapshot` _[LocalObjectRef](#localobjectref)_ | CurrentSnapshot describes the observed working snapshot of the VirtualMachine. |
| `rootSnapshots` _LocalObjectRef array_ | RootSnapshots represents the observed list of root snapshots of
a VM. Since each snapshot includes the list of its child
snapshots, these root snapshot references can effectively be
used to construct the entire snapshot chain of a virtual
machine. |

### VirtualMachineStorageStatus



VirtualMachineStorageStatus defines the observed state of a VirtualMachine's
storage.

_Appears in:_
- [VirtualMachineStatus](#virtualmachinestatus)

| Field | Description |
| --- | --- |
| `total` _[Quantity](#quantity)_ | Total describes the total storage space used by a VirtualMachine that
counts against the Namespace's storage quota.
This value is a sum of requested.disks + used.other. |
| `requested` _[VirtualMachineStorageStatusRequested](#virtualmachinestoragestatusrequested)_ | Requested describes the observed amount of storage requested by a
VirtualMachine. |
| `usage` _[VirtualMachineStorageStatusUsed](#virtualmachinestoragestatusused)_ | Used describes the observed amount of storage used by a VirtualMachine. |

### VirtualMachineStorageStatusRequested





_Appears in:_
- [VirtualMachineStorageStatus](#virtualmachinestoragestatus)

| Field | Description |
| --- | --- |
| `disks` _[Quantity](#quantity)_ | Disks describes the total storage space requested by a VirtualMachine's
non-PVC disks. |

### VirtualMachineStorageStatusUsed





_Appears in:_
- [VirtualMachineStorageStatus](#virtualmachinestoragestatus)

| Field | Description |
| --- | --- |
| `disks` _[Quantity](#quantity)_ | Disks describes the total storage space used by a VirtualMachine's
non-PVC disks. |
| `other` _[Quantity](#quantity)_ | Other describes the total storage space used by the VirtualMachine's
non disk files, ex. the configuration file, swap space, logs, snapshots,
etc. |


### VirtualMachineTemplateSpec



VirtualMachineTemplateSpec describes the data needed to create a VirtualMachine
from a template.

_Appears in:_
- [VirtualMachineReplicaSetSpec](#virtualmachinereplicasetspec)

| Field | Description |
| --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineSpec](#virtualmachinespec)_ | Specification of the desired behavior of each replica virtual machine. |

### VirtualMachineVolume



VirtualMachineVolume represents a named volume in a VM.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name represents the volume's name. Must be a DNS_LABEL and unique within
the VM. |
| `persistentVolumeClaim` _[PersistentVolumeClaimVolumeSource](#persistentvolumeclaimvolumesource)_ | PersistentVolumeClaim represents a reference to a PersistentVolumeClaim
in the same namespace.

More information is available at
https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims. |

### VirtualMachineVolumeCryptoStatus





_Appears in:_
- [VirtualMachineVolumeStatus](#virtualmachinevolumestatus)

| Field | Description |
| --- | --- |
| `providerID` _string_ | ProviderID describes the provider ID used to encrypt the volume.
Please note, this field will be empty if the volume is not
encrypted. |
| `keyID` _string_ | KeyID describes the key ID used to encrypt the volume.
Please note, this field will be empty if the volume is not
encrypted. |

### VirtualMachineVolumeProvisioningMode

_Underlying type:_ `string`

VirtualMachineVolumeProvisioningMode is the type used to express the
desired or observed provisioning mode for a virtual machine disk.

_Appears in:_
- [VirtualMachineAdvancedSpec](#virtualmachineadvancedspec)


### VirtualMachineVolumeSource



VirtualMachineVolumeSource represents the source location of a volume to
mount. Only one of its members may be specified.

_Appears in:_
- [VirtualMachineVolume](#virtualmachinevolume)

| Field | Description |
| --- | --- |
| `persistentVolumeClaim` _[PersistentVolumeClaimVolumeSource](#persistentvolumeclaimvolumesource)_ | PersistentVolumeClaim represents a reference to a PersistentVolumeClaim
in the same namespace.

More information is available at
https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims. |

### VirtualMachineVolumeStatus



VirtualMachineVolumeStatus defines the observed state of a
VirtualMachineVolume instance.

_Appears in:_
- [VirtualMachineStatus](#virtualmachinestatus)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of the attached volume. |
| `type` _[VirtualMachineVolumeType](#virtualmachinevolumetype)_ | Type is the type of the attached volume. |
| `crypto` _[VirtualMachineVolumeCryptoStatus](#virtualmachinevolumecryptostatus)_ | Crypto describes the volume's encryption status. |
| `limit` _[Quantity](#quantity)_ | Limit describes the maximum, requested capacity of the volume. |
| `requested` _[Quantity](#quantity)_ | Requested describes the minimum, requested capacity of the volume.

Please note, this value is used when calculating a VM's impact to a
namespace's storage quota. |
| `used` _[Quantity](#quantity)_ | Used describes the observed, non-shared size of the volume on disk.

For example, if this is a linked-clone's boot volume, this value
represents the space consumed by the linked clone, not the parent.

Another example is when a volume is thin-provisioned. The volume's
capacity may be 20Gi, but the actual usage on disk may only be a few
hundred mebibytes. |
| `attached` _boolean_ | Attached represents whether a volume has been successfully attached to
the VirtualMachine or not. |
| `diskUUID` _string_ | DiskUUID represents the underlying virtual disk UUID and is present when
attachment succeeds. |
| `error` _string_ | Error represents the last error seen when attaching or detaching a
volume.  Error will be empty if attachment succeeds. |

### VirtualMachineVolumeType

_Underlying type:_ `string`

VirtualMachineVolumeType describes the type of a VirtualMachine volume.

_Appears in:_
- [VirtualMachineImageCacheFileStatus](#virtualmachineimagecachefilestatus)
- [VirtualMachineVolumeStatus](#virtualmachinevolumestatus)


### VirtualMachineWebConsoleRequestSpec



VirtualMachineWebConsoleRequestSpec describes the desired state for a web
console request to a VM.

_Appears in:_
- [VirtualMachineWebConsoleRequest](#virtualmachinewebconsolerequest)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of a VM in the same Namespace as this web console
request. |
| `publicKey` _string_ | PublicKey is used to encrypt the status.response. This is expected to be a RSA OAEP public key in X.509 PEM format. |

### VirtualMachineWebConsoleRequestStatus



VirtualMachineWebConsoleRequestStatus describes the observed state of the
request.

_Appears in:_
- [VirtualMachineWebConsoleRequest](#virtualmachinewebconsolerequest)

| Field | Description |
| --- | --- |
| `response` _string_ | Response will be the authenticated ticket corresponding to this web console request. |
| `expiryTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | ExpiryTime is the time at which access via this request will expire. |
| `proxyAddr` _string_ | ProxyAddr describes the host address and optional port used to access
the VM's web console.

The value could be a DNS entry, IPv4, or IPv6 address, followed by an
optional port. For example, valid values include:

    DNS
        * host.com
        * host.com:6443

    IPv4
        * 1.2.3.4
        * 1.2.3.4:6443

    IPv6
        * 1234:1234:1234:1234:1234:1234:1234:1234
        * [1234:1234:1234:1234:1234:1234:1234:1234]:6443
        * 1234:1234:1234:0000:0000:0000:1234:1234
        * 1234:1234:1234::::1234:1234
        * [1234:1234:1234::::1234:1234]:6443

In other words, the field may be set to any value that is parsable
by Go's https://pkg.go.dev/net#ResolveIPAddr and
https://pkg.go.dev/net#ParseIP functions. |

### ZoneSelectorOperator

_Underlying type:_ `string`

ZoneSelectorOperator specifies the type of operator used by
the zone selector to represent key-value relationships.

_Appears in:_
- [ZoneSelectorRequirement](#zoneselectorrequirement)


### ZoneSelectorRequirement



ZoneSelectorRequirement defines the key value relationships for a matching zone selector.

_Appears in:_
- [ZoneSelectorTerm](#zoneselectorterm)

| Field | Description |
| --- | --- |
| `key` _string_ | Key is the label key to which the selector applies. |
| `operator` _[ZoneSelectorOperator](#zoneselectoroperator)_ | Operator represents a key's relationship to a set of values.
Valid operators are In, NotIn, Exists, DoesNotExist. Gt, and Lt. |
| `values` _string array_ | Values is a list of values to which the operator applies.
If the operator is In or NotIn, the values list must be non-empty.
If the operator is Exists or DoesNotExist, the values list must be empty.
If the operator is Gt or Lt, the values list must have a single element,
which will be interpreted as an integer. |

### ZoneSelectorTerm



ZoneSelectorTerm defines the matching zone selector requirements for zone based affinity/anti-affinity scheduling.

_Appears in:_
- [VirtualMachineAffinityZoneAffinitySpec](#virtualmachineaffinityzoneaffinityspec)
- [VirtualMachineAntiAffinityZoneAffinitySpec](#virtualmachineantiaffinityzoneaffinityspec)

| Field | Description |
| --- | --- |
| `matchExpressions` _[ZoneSelectorRequirement](#zoneselectorrequirement) array_ | MatchExpressions is a list of zone selector requirements by zone's
labels. |
| `matchFields` _[ZoneSelectorRequirement](#zoneselectorrequirement) array_ | MatchFields is a list of zone selector requirements by zone's fields. |
