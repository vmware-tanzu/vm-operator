# v1alpha1

Package v1alpha1 contains the VM Operator v1alpha1 APIs.



---

## Kinds


### ClusterVirtualMachineImage



ClusterVirtualMachineImage is the schema for the clustervirtualmachineimage API
A ClusterVirtualMachineImage represents the desired specification and the observed status of a
ClusterVirtualMachineImage instance.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `ClusterVirtualMachineImage`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineImageSpec](#virtualmachineimagespec)_ |  |
| `status` _[VirtualMachineImageStatus](#virtualmachineimagestatus)_ |  |

### ContentLibraryProvider



ContentLibraryProvider is the Schema for the contentlibraryproviders API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `ContentLibraryProvider`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[ContentLibraryProviderSpec](#contentlibraryproviderspec)_ |  |
| `status` _[ContentLibraryProviderStatus](#contentlibraryproviderstatus)_ |  |

### ContentSource



ContentSource is the Schema for the contentsources API.
A ContentSource represents the desired specification and the observed status of a ContentSource instance.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `ContentSource`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[ContentSourceSpec](#contentsourcespec)_ |  |
| `status` _[ContentSourceStatus](#contentsourcestatus)_ |  |

### ContentSourceBinding



ContentSourceBinding is an object that represents a ContentSource to Namespace mapping.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `ContentSourceBinding`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `contentSourceRef` _[ContentSourceReference](#contentsourcereference)_ | ContentSourceRef is a reference to a ContentSource object. |

### VirtualMachine



VirtualMachine is the Schema for the virtualmachines API.
A VirtualMachine represents the desired specification and the observed status of a VirtualMachine instance.  A
VirtualMachine is realized by the VirtualMachine controller on a backing Virtual Infrastructure provider such as
vSphere.

_Appears in:_
- [VirtualMachineTemplate](#virtualmachinetemplate)

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `VirtualMachine`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineSpec](#virtualmachinespec)_ |  |
| `status` _[VirtualMachineStatus](#virtualmachinestatus)_ |  |

### VirtualMachineClass



VirtualMachineClass is the Schema for the virtualmachineclasses API.
A VirtualMachineClass represents the desired specification and the observed status of a VirtualMachineClass
instance.  A VirtualMachineClass represents a policy and configuration resource which defines a set of attributes to
be used in the configuration of a VirtualMachine instance.  A VirtualMachine resource references a
VirtualMachineClass as a required input.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `VirtualMachineClass`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineClassSpec](#virtualmachineclassspec)_ |  |
| `status` _[VirtualMachineClassStatus](#virtualmachineclassstatus)_ |  |

### VirtualMachineClassBinding



VirtualMachineClassBinding is a binding object responsible for
defining a VirtualMachineClass and a Namespace associated with it.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `VirtualMachineClassBinding`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `classRef` _[ClassReference](#classreference)_ | ClassReference is a reference to a VirtualMachineClass object |

### VirtualMachineImage



VirtualMachineImage is the Schema for the virtualmachineimages API
A VirtualMachineImage represents a VirtualMachine image (e.g. VM template) that can be used as the base image
for creating a VirtualMachine instance.  The VirtualMachineImage is a required field of the VirtualMachine
spec.  Currently, VirtualMachineImages are immutable to end users.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `VirtualMachineImage`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineImageSpec](#virtualmachineimagespec)_ |  |
| `status` _[VirtualMachineImageStatus](#virtualmachineimagestatus)_ |  |

### VirtualMachinePublishRequest



VirtualMachinePublishRequest defines the information necessary to publish a
VirtualMachine as a VirtualMachineImage to an image registry.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `VirtualMachinePublishRequest`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachinePublishRequestSpec](#virtualmachinepublishrequestspec)_ |  |
| `status` _[VirtualMachinePublishRequestStatus](#virtualmachinepublishrequeststatus)_ |  |

### VirtualMachineService



VirtualMachineService is the Schema for the virtualmachineservices API.
A VirtualMachineService represents the desired specification and the observed status of a VirtualMachineService
instance. A VirtualMachineService represents a network service, provided by one or more VirtualMachines, that is
desired to be exposed to other workloads both internal and external to the cluster.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `VirtualMachineService`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineServiceSpec](#virtualmachineservicespec)_ |  |
| `status` _[VirtualMachineServiceStatus](#virtualmachineservicestatus)_ |  |

### VirtualMachineSetResourcePolicy



VirtualMachineSetResourcePolicy is the Schema for the virtualmachinesetresourcepolicies API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `VirtualMachineSetResourcePolicy`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineSetResourcePolicySpec](#virtualmachinesetresourcepolicyspec)_ |  |
| `status` _[VirtualMachineSetResourcePolicyStatus](#virtualmachinesetresourcepolicystatus)_ |  |

### WebConsoleRequest



WebConsoleRequest allows the creation of a one-time web console ticket that can be used to interact with the VM.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha1`
| `kind` _string_ | `WebConsoleRequest`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[WebConsoleRequestSpec](#webconsolerequestspec)_ |  |
| `status` _[WebConsoleRequestStatus](#webconsolerequeststatus)_ |  |


## Types
### ClassReference



ClassReference contains info to locate a Kind VirtualMachineClass object.

_Appears in:_
- [VirtualMachineClassBinding](#virtualmachineclassbinding)

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | API version of the referent. |
| `kind` _string_ | Kind is the type of resource being referenced. |
| `name` _string_ | Name is the name of resource being referenced. |

### ClusterModuleSpec



ClusterModuleSpec defines a grouping of VirtualMachines that are to be grouped together as a logical unit by
the infrastructure provider.  Within vSphere, the ClusterModuleSpec maps directly to a vSphere ClusterModule.

_Appears in:_
- [VirtualMachineSetResourcePolicySpec](#virtualmachinesetresourcepolicyspec)

| Field | Description |
| --- | --- |
| `groupname` _string_ | GroupName describes the name of the ClusterModule Group. |

### ClusterModuleStatus





_Appears in:_
- [VirtualMachineSetResourcePolicyStatus](#virtualmachinesetresourcepolicystatus)

| Field | Description |
| --- | --- |
| `groupname` _string_ |  |
| `moduleUUID` _string_ |  |
| `clusterMoID` _string_ |  |

### Condition



Condition defines an observation of a VM Operator API resource operational state.

_Appears in:_
- [VirtualMachineImageStatus](#virtualmachineimagestatus)
- [VirtualMachinePublishRequestStatus](#virtualmachinepublishrequeststatus)
- [VirtualMachineStatus](#virtualmachinestatus)

| Field | Description |
| --- | --- |
| `type` _ConditionType_ | Type of condition in CamelCase or in foo.example.com/CamelCase.
Many .condition.type values are consistent across resources like Available, but because arbitrary conditions
can be useful (see .node.status.conditions), the ability to disambiguate is important. |
| `status` _[ConditionStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#conditionstatus-v1-core)_ | Status of the condition, one of True, False, Unknown. |
| `severity` _ConditionSeverity_ | Severity provides an explicit classification of Reason code, so the users or machines can immediately
understand the current situation and act accordingly.
The Severity field MUST be set only when Status=False. |
| `lastTransitionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | Last time the condition transitioned from one status to another.
This should be when the underlying condition changed. If that is not known, then using the time when
the API field changed is acceptable. |
| `reason` _string_ | The reason for the condition's last transition in CamelCase.
The specific API may choose whether or not this field is considered a guaranteed API.
This field may not be empty. |
| `message` _string_ | A human readable message indicating details about the transition.
This field may be empty. |


### ContentLibraryProviderSpec



ContentLibraryProviderSpec defines the desired state of ContentLibraryProvider.

_Appears in:_
- [ContentLibraryProvider](#contentlibraryprovider)

| Field | Description |
| --- | --- |
| `uuid` _string_ | UUID describes the UUID of a vSphere content library. It is the unique identifier for a
vSphere content library. |


### ContentProviderReference



ContentProviderReference contains the info to locate a content provider resource.

_Appears in:_
- [ContentSourceSpec](#contentsourcespec)
- [VirtualMachineImageSpec](#virtualmachineimagespec)

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | API version of the referent. |
| `kind` _string_ | Kind is the type of resource being referenced. |
| `name` _string_ | Name is the name of resource being referenced. |
| `namespace` _string_ | Namespace of the resource being referenced. If empty, cluster scoped resource is assumed. |

### ContentSourceReference



ContentSourceReference contains info to locate a Kind ContentSource object.

_Appears in:_
- [ContentSourceBinding](#contentsourcebinding)

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | API version of the referent. |
| `kind` _string_ | Kind is the type of resource being referenced. |
| `name` _string_ | Name is the name of resource being referenced. |

### ContentSourceSpec



ContentSourceSpec defines the desired state of ContentSource.

_Appears in:_
- [ContentSource](#contentsource)

| Field | Description |
| --- | --- |
| `providerRef` _[ContentProviderReference](#contentproviderreference)_ | ProviderRef is a reference to a content provider object that describes a provider. |


### DynamicDirectPathIODevice



DynamicDirectPathIODevice contains the configuration corresponding to a Dynamic DirectPath I/O device.

_Appears in:_
- [VirtualDevices](#virtualdevices)

| Field | Description |
| --- | --- |
| `vendorID` _integer_ |  |
| `deviceID` _integer_ |  |
| `customLabel` _string_ |  |

### FolderSpec



FolderSpec defines a Folder.

_Appears in:_
- [VirtualMachineSetResourcePolicySpec](#virtualmachinesetresourcepolicyspec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the name of the Folder |

### GuestHeartbeatAction



GuestHeartbeatAction describes an action based on the guest heartbeat.

_Appears in:_
- [Probe](#probe)

| Field | Description |
| --- | --- |
| `thresholdStatus` _[GuestHeartbeatStatus](#guestheartbeatstatus)_ | ThresholdStatus is the value that the guest heartbeat status must be at or above to be
considered successful. |

### GuestHeartbeatStatus

_Underlying type:_ `string`

GuestHeartbeatStatus is the status type for a GuestHeartbeat.

_Appears in:_
- [GuestHeartbeatAction](#guestheartbeataction)


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
| `size` _Quantity_ |  |

### InstanceVolumeClaimVolumeSource



InstanceVolumeClaimVolumeSource contains information about the instance
storage volume claimed as a PVC.

_Appears in:_
- [PersistentVolumeClaimVolumeSource](#persistentvolumeclaimvolumesource)

| Field | Description |
| --- | --- |
| `storageClass` _string_ | StorageClass is the name of the Kubernetes StorageClass that provides
the backing storage for this instance storage volume. |
| `size` _Quantity_ | Size is the size of the requested instance storage volume. |

### LoadBalancerIngress



LoadBalancerIngress represents the status of a load balancer ingress point:
traffic intended for the service should be sent to an ingress point.
IP or Hostname may both be set in this structure. It is up to the consumer to determine which
field should be used when accessing this LoadBalancer.

_Appears in:_
- [LoadBalancerStatus](#loadbalancerstatus)

| Field | Description |
| --- | --- |
| `ip` _string_ | IP is set for load balancer ingress points that are specified by an IP address. |
| `hostname` _string_ | Hostname is set for load balancer ingress points that are specified by a DNS address. |

### LoadBalancerStatus



LoadBalancerStatus represents the status of a load balancer.

_Appears in:_
- [VirtualMachineServiceStatus](#virtualmachineservicestatus)

| Field | Description |
| --- | --- |
| `ingress` _[LoadBalancerIngress](#loadbalanceringress) array_ | Ingress is a list containing ingress addresses for the load balancer.
Traffic intended for the service should be sent to any of these ingress points. |

### NetworkDeviceStatus



NetworkDeviceStatus defines the network interface IP configuration including
gateway, subnetmask and IP address as seen by OVF properties.

_Appears in:_
- [NetworkStatus](#networkstatus)

| Field | Description |
| --- | --- |
| `Gateway4` _string_ | Gateway4 is the gateway for the IPv4 address family for this device. |
| `MacAddress` _string_ | MacAddress is the MAC address of the network device. |
| `IPAddresses` _string array_ | IpAddresses represents one or more IP addresses assigned to the network
device in CIDR notation, ex. "192.0.2.1/16". |

### NetworkInterfaceProviderReference



NetworkInterfaceProviderReference contains info to locate a network interface provider object.

_Appears in:_
- [VirtualMachineNetworkInterface](#virtualmachinenetworkinterface)

| Field | Description |
| --- | --- |
| `apiGroup` _string_ | APIGroup is the group for the resource being referenced. |
| `kind` _string_ | Kind is the type of resource being referenced |
| `name` _string_ | Name is the name of resource being referenced |
| `apiVersion` _string_ | API version of the referent. |

### NetworkInterfaceStatus



NetworkInterfaceStatus defines the observed state of network interfaces attached to the VirtualMachine
as seen by the Guest OS and VMware tools.

_Appears in:_
- [VirtualMachineStatus](#virtualmachinestatus)

| Field | Description |
| --- | --- |
| `connected` _boolean_ | Connected represents whether the network interface is connected or not. |
| `macAddress` _string_ | MAC address of the network adapter |
| `ipAddresses` _string array_ | IpAddresses represents zero, one or more IP addresses assigned to the network interface in CIDR notation.
For eg, "192.0.2.1/16". |

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

### OvfProperty



OvfProperty describes information related to a user configurable property element that is supported by
VirtualMachineImage and can be customized during VirtualMachine creation.

_Appears in:_
- [VirtualMachineImageSpec](#virtualmachineimagespec)

| Field | Description |
| --- | --- |
| `key` _string_ | Key describes the key of the ovf property. |
| `type` _string_ | Type describes the type of the ovf property. |
| `default` _string_ | Default describes the default value of the ovf key. |
| `description` _string_ | Description contains the value of the OVF property's optional
"Description" element. |
| `label` _string_ | Label contains the value of the OVF property's optional
"Label" element. |

### PersistentVolumeClaimVolumeSource



PersistentVolumeClaimVolumeSource is a composite for the Kubernetes
corev1.PersistentVolumeClaimVolumeSource and instance storage options.

_Appears in:_
- [VirtualMachineVolume](#virtualmachinevolume)

| Field | Description |
| --- | --- |
| `claimName` _string_ | claimName is the name of a PersistentVolumeClaim in the same namespace as the pod using this volume.
More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims |
| `readOnly` _boolean_ | readOnly Will force the ReadOnly setting in VolumeMounts.
Default false. |
| `instanceVolumeClaim` _[InstanceVolumeClaimVolumeSource](#instancevolumeclaimvolumesource)_ | InstanceVolumeClaim is set if the PVC is backed by instance storage. |

### Probe



Probe describes a health check to be performed against a VirtualMachine to determine whether it is
alive or ready to receive traffic. Only one probe action can be specified.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `tcpSocket` _[TCPSocketAction](#tcpsocketaction)_ | TCPSocket specifies an action involving a TCP port.


Deprecated: The TCPSocket action requires network connectivity that is not supported in all environments.
This field will be removed in a later API version. |
| `guestHeartbeat` _[GuestHeartbeatAction](#guestheartbeataction)_ | GuestHeartbeat specifies an action involving the guest heartbeat status. |
| `timeoutSeconds` _integer_ | TimeoutSeconds specifies a number of seconds after which the probe times out.
Defaults to 10 seconds. Minimum value is 1. |
| `periodSeconds` _integer_ | PeriodSeconds specifics how often (in seconds) to perform the probe.
Defaults to 10 seconds. Minimum value is 1. |

### ResourcePoolSpec



ResourcePoolSpec defines a Logical Grouping of workloads that share resource policies.

_Appears in:_
- [VirtualMachineSetResourcePolicySpec](#virtualmachinesetresourcepolicyspec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the name of the ResourcePool grouping. |
| `reservations` _[VirtualMachineResourceSpec](#virtualmachineresourcespec)_ | Reservations describes the guaranteed resources reserved for the ResourcePool. |
| `limits` _[VirtualMachineResourceSpec](#virtualmachineresourcespec)_ | Limits describes the limit to resources available to the ResourcePool. |

### TCPSocketAction



TCPSocketAction describes an action based on opening a socket.

_Appears in:_
- [Probe](#probe)

| Field | Description |
| --- | --- |
| `port` _IntOrString_ | Port specifies a number or name of the port to access on the VirtualMachine.
If the format of port is a number, it must be in the range 1 to 65535.
If the format of name is a string, it must be an IANA_SVC_NAME. |
| `host` _string_ | Host is an optional host name to connect to.  Host defaults to the VirtualMachine IP. |

### VGPUDevice



VGPUDevice contains the configuration corresponding to a vGPU device.

_Appears in:_
- [VirtualDevices](#virtualdevices)

| Field | Description |
| --- | --- |
| `profileName` _string_ |  |

### VirtualDevices



VirtualDevices contains information about the virtual devices associated with a VirtualMachineClass.

_Appears in:_
- [VirtualMachineClassHardware](#virtualmachineclasshardware)

| Field | Description |
| --- | --- |
| `vgpuDevices` _[VGPUDevice](#vgpudevice) array_ |  |
| `dynamicDirectPathIODevices` _[DynamicDirectPathIODevice](#dynamicdirectpathiodevice) array_ |  |

### VirtualMachineAdvancedOptions



VirtualMachineAdvancedOptions describes a set of optional, advanced options for configuring a VirtualMachine.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `defaultVolumeProvisioningOptions` _[VirtualMachineVolumeProvisioningOptions](#virtualmachinevolumeprovisioningoptions)_ | DefaultProvisioningOptions specifies the provisioning type to be used by default for VirtualMachine volumes exclusively
owned by this VirtualMachine. This does not apply to PersistentVolumeClaim volumes that are created and managed externally. |
| `changeBlockTracking` _boolean_ | ChangeBlockTracking specifies the enablement of incremental backup support for this VirtualMachine, which can be utilized
by external backup systems such as VMware Data Recovery. |

### VirtualMachineClassHardware



VirtualMachineClassHardware describes a virtual hardware resource specification.

_Appears in:_
- [VirtualMachineClassSpec](#virtualmachineclassspec)

| Field | Description |
| --- | --- |
| `cpus` _integer_ |  |
| `memory` _Quantity_ |  |
| `devices` _[VirtualDevices](#virtualdevices)_ |  |
| `instanceStorage` _[InstanceStorage](#instancestorage)_ |  |

### VirtualMachineClassPolicies



VirtualMachineClassPolicies describes the policy configuration to be used by a VirtualMachineClass.

_Appears in:_
- [VirtualMachineClassSpec](#virtualmachineclassspec)

| Field | Description |
| --- | --- |
| `resources` _[VirtualMachineClassResources](#virtualmachineclassresources)_ |  |

### VirtualMachineClassResources



VirtualMachineClassResources describes the virtual hardware resource reservations and limits configuration to be used
by a VirtualMachineClass.

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
| `hardware` _[VirtualMachineClassHardware](#virtualmachineclasshardware)_ | Hardware describes the configuration of the VirtualMachineClass attributes related to virtual hardware.  The
configuration specified in this field is used to customize the virtual hardware characteristics of any VirtualMachine
associated with this VirtualMachineClass. |
| `policies` _[VirtualMachineClassPolicies](#virtualmachineclasspolicies)_ | Policies describes the configuration of the VirtualMachineClass attributes related to virtual infrastructure
policy.  The configuration specified in this field is used to customize various policies related to
infrastructure resource consumption. |
| `description` _string_ | Description describes the configuration of the VirtualMachineClass which is not related to virtual hardware
or infrastructure policy. This field is used to address remaining specs about this VirtualMachineClass. |
| `configSpec` _[json.RawMessage](https://pkg.go.dev/encoding/json#RawMessage)_ | ConfigSpec describes additional configuration information for a
VirtualMachine.
The contents of this field are the VirtualMachineConfigSpec data object
(https://bit.ly/3HDtiRu) marshaled to JSON using the discriminator
field "_typeName" to preserve type information. |


### VirtualMachineImageOSInfo



VirtualMachineImageOSInfo describes optional information related to the image operating system that can be added
to an image template. This information can be used by the image author to communicate details of the operating
system associated with the image.

_Appears in:_
- [VirtualMachineImageSpec](#virtualmachineimagespec)

| Field | Description |
| --- | --- |
| `version` _string_ | Version typically describes the version of the guest operating system. |
| `type` _string_ | Type typically describes the type of the guest operating system. |

### VirtualMachineImageProductInfo



VirtualMachineImageProductInfo describes optional product-related information that can be added to an image
template.  This information can be used by the image author to communicate details of the product contained in the
image.

_Appears in:_
- [VirtualMachineImageSpec](#virtualmachineimagespec)

| Field | Description |
| --- | --- |
| `product` _string_ | Product typically describes the type of product contained in the image. |
| `vendor` _string_ | Vendor typically describes the name of the vendor that is producing the image. |
| `version` _string_ | Version typically describes a short-form version of the image. |
| `fullVersion` _string_ | FullVersion typically describes a long-form version of the image. |

### VirtualMachineImageSpec



VirtualMachineImageSpec defines the desired state of VirtualMachineImage.

_Appears in:_
- [ClusterVirtualMachineImage](#clustervirtualmachineimage)
- [VirtualMachineImage](#virtualmachineimage)

| Field | Description |
| --- | --- |
| `type` _string_ | Type describes the type of the VirtualMachineImage. Currently, the only supported image is "OVF" |
| `imageSourceType` _string_ | ImageSourceType describes the type of content source of the VirtualMachineImage.  The only Content Source
supported currently is the vSphere Content Library. |
| `imageID` _string_ | ImageID is a unique identifier exposed by the provider of this VirtualMachineImage. |
| `providerRef` _[ContentProviderReference](#contentproviderreference)_ | ProviderRef is a reference to a content provider object that describes a provider. |
| `productInfo` _[VirtualMachineImageProductInfo](#virtualmachineimageproductinfo)_ | ProductInfo describes the attributes of the VirtualMachineImage relating to the product contained in the
image. |
| `osInfo` _[VirtualMachineImageOSInfo](#virtualmachineimageosinfo)_ | OSInfo describes the attributes of the VirtualMachineImage relating to the Operating System contained in the
image. |
| `ovfEnv` _object (keys:string, values:[OvfProperty](#ovfproperty))_ | OVFEnv describes the user configurable customization parameters of the VirtualMachineImage. |
| `hwVersion` _integer_ | HardwareVersion describes the virtual hardware version of the image |

### VirtualMachineImageStatus



VirtualMachineImageStatus defines the observed state of VirtualMachineImage.

_Appears in:_
- [ClusterVirtualMachineImage](#clustervirtualmachineimage)
- [VirtualMachineImage](#virtualmachineimage)

| Field | Description |
| --- | --- |
| `uuid` _string_ | Deprecated |
| `internalId` _string_ | Deprecated |
| `powerState` _string_ | Deprecated |
| `imageName` _string_ | ImageName describes the display name of this image. |
| `imageSupported` _boolean_ | ImageSupported indicates whether the VirtualMachineImage is supported by VMService.
A VirtualMachineImage is supported by VMService if the following conditions are true:
- VirtualMachineImageV1Alpha1CompatibleCondition |
| `conditions` _[Condition](#condition) array_ | Conditions describes the current condition information of the VirtualMachineImage object. e.g. if the OS type
is supported or image is supported by VMService |
| `contentLibraryRef` _[TypedLocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#typedlocalobjectreference-v1-core)_ | ContentLibraryRef is a reference to the source ContentLibrary/ClusterContentLibrary resource.


Deprecated: This field is provider specific but the VirtualMachineImage types are intended to be provider generic.
This field does not exist in later API versions. Instead, the Spec.ProviderRef field should be used to look up the
provider. For images provided by a Content Library, the ProviderRef will point to either a ContentLibraryItem or
ClusterContentLibraryItem that contains a reference to the Content Library. |
| `contentVersion` _string_ | ContentVersion describes the observed content version of this VirtualMachineImage that was last successfully
synced with the vSphere content library item. |
| `firmware` _string_ | Firmware describe the firmware type used by this VirtualMachineImage.
eg: bios, efi. |

### VirtualMachineMetadata



VirtualMachineMetadata defines any metadata that should be passed to the VirtualMachine instance.  A typical use
case is for this metadata to be used for Guest Customization, however the intended use of the metadata is
agnostic to the VirtualMachine controller.  VirtualMachineMetadata is read from a configured ConfigMap or a Secret and then
propagated to the VirtualMachine instance using a desired "Transport" mechanism.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `configMapName` _string_ | ConfigMapName describes the name of the ConfigMap, in the same Namespace as the VirtualMachine, that should be
used for VirtualMachine metadata.  The contents of the Data field of the ConfigMap is used as the VM Metadata.
The format of the contents of the VM Metadata are not parsed or interpreted by the VirtualMachine controller.
Please note, this field and SecretName are mutually exclusive. |
| `secretName` _string_ | SecretName describes the name of the Secret, in the same Namespace as the VirtualMachine, that should be used
for VirtualMachine metadata. The contents of the Data field of the Secret is used as the VM Metadata.
The format of the contents of the VM Metadata are not parsed or interpreted by the VirtualMachine controller.
Please note, this field and ConfigMapName are mutually exclusive. |
| `transport` _VirtualMachineMetadataTransport_ | Transport describes the name of a supported VirtualMachineMetadata transport protocol.  Currently, the only supported
transport protocols are "ExtraConfig", "OvfEnv" and "CloudInit". |

### VirtualMachineNetworkInterface



VirtualMachineNetworkInterface defines the properties of a network interface to attach to a VirtualMachine
instance.  A VirtualMachineNetworkInterface describes network interface configuration that is used by the
VirtualMachine controller when integrating the VirtualMachine into a VirtualNetwork. Currently, only NSX-T
and vSphere Distributed Switch (VDS) type network integrations are supported using this VirtualMachineNetworkInterface
structure.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `networkType` _string_ | NetworkType describes the type of VirtualNetwork that is referenced by the NetworkName. Currently, the supported
NetworkTypes are "nsx-t", "nsx-t-subnet", "nsx-t-subnetset" and "vsphere-distributed". |
| `networkName` _string_ | NetworkName describes the name of an existing virtual network that this interface should be added to.
For "nsx-t" NetworkType, this is the name of a pre-existing NSX-T VirtualNetwork. If unspecified,
the default network for the namespace will be used. For "vsphere-distributed" NetworkType, the
NetworkName must be specified. |
| `providerRef` _[NetworkInterfaceProviderReference](#networkinterfaceproviderreference)_ | ProviderRef is reference to a network interface provider object that specifies the network interface configuration.
If unset, default configuration is assumed. |
| `ethernetCardType` _string_ | EthernetCardType describes an optional ethernet card that should be used by the VirtualNetworkInterface (vNIC)
associated with this network integration.  The default is "vmxnet3". |

### VirtualMachinePort



VirtualMachinePort is unused and can be considered deprecated.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `port` _integer_ |  |
| `ip` _string_ |  |
| `name` _string_ |  |
| `protocol` _[Protocol](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#protocol-v1-core)_ |  |

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
| `conditions` _[Condition](#condition) array_ | Conditions is a list of the latest, available observations of the
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
| `name` _string_ | Name is the display name of the published object.


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

### VirtualMachineResourceSpec



VirtualMachineResourceSpec describes a virtual hardware policy specification.

_Appears in:_
- [ResourcePoolSpec](#resourcepoolspec)
- [VirtualMachineClassResources](#virtualmachineclassresources)

| Field | Description |
| --- | --- |
| `cpu` _Quantity_ |  |
| `memory` _Quantity_ |  |

### VirtualMachineServicePort



VirtualMachineServicePort describes the specification of a service port to be exposed by a VirtualMachineService.
This VirtualMachineServicePort specification includes attributes that define the external and internal
representation of the service port.

_Appears in:_
- [VirtualMachineServiceSpec](#virtualmachineservicespec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the name to be used to identify this VirtualMachineServicePort |
| `protocol` _string_ | Protocol describes the Layer 4 transport protocol for this port. Supports "TCP", "UDP", and "SCTP". |
| `port` _integer_ | Port describes the external port that will be exposed by the service. |
| `targetPort` _integer_ | TargetPort describes the internal port open on a VirtualMachine that should be mapped to the external Port. |

### VirtualMachineServiceSpec



VirtualMachineServiceSpec defines the desired state of VirtualMachineService. Each VirtualMachineService exposes
a set of TargetPorts on a set of VirtualMachine instances as a network endpoint within or outside of the
Kubernetes cluster. The VirtualMachineService is loosely coupled to the VirtualMachines that are backing it through
the use of a Label Selector. In Kubernetes, a Label Selector enables matching of a resource using a set of
key-value pairs, aka Labels. By using a Label Selector, the VirtualMachineService can be generically defined to apply
to any VirtualMachine in the same namespace that has the appropriate set of labels.

_Appears in:_
- [VirtualMachineService](#virtualmachineservice)

| Field | Description |
| --- | --- |
| `type` _VirtualMachineServiceType_ | Type specifies a desired VirtualMachineServiceType for this VirtualMachineService. Supported types
are ClusterIP, LoadBalancer, ExternalName. |
| `ports` _[VirtualMachineServicePort](#virtualmachineserviceport) array_ | Ports specifies a list of VirtualMachineServicePort to expose with this VirtualMachineService. Each of these ports
will be an accessible network entry point to access this service by. |
| `selector` _object (keys:string, values:string)_ | Selector specifies a map of key-value pairs, also known as a Label Selector, that is used to match this
VirtualMachineService with the set of VirtualMachines that should back this VirtualMachineService. |
| `loadBalancerIP` _string_ | Only applies to VirtualMachineService Type: LoadBalancer
LoadBalancer will get created with the IP specified in this field.
This feature depends on whether the underlying load balancer provider supports specifying
the loadBalancerIP when a load balancer is created.
This field will be ignored if the provider does not support the feature. |
| `loadBalancerSourceRanges` _string array_ | LoadBalancerSourceRanges is an array of IP addresses in the format of
CIDRs, for example: 103.21.244.0/22 and 10.0.0.0/24.
If specified and supported by the load balancer provider, this will restrict
ingress traffic to the specified client IPs. This field will be ignored if the
provider does not support the feature. |
| `clusterIp` _string_ | clusterIP is the IP address of the service and is usually assigned
randomly by the master. If an address is specified manually and is not in
use by others, it will be allocated to the service; otherwise, creation
of the service will fail. This field can not be changed through updates.
Valid values are "None", empty string (""), or a valid IP address. "None"
can be specified for headless services when proxying is not required.
Only applies to types ClusterIP and LoadBalancer.
Ignored if type is ExternalName.
More info: https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies |
| `externalName` _string_ | externalName is the external reference that kubedns or equivalent will
return as a CNAME record for this service. No proxying will be involved.
Must be a valid RFC-1123 hostname (https://tools.ietf.org/html/rfc1123)
and requires Type to be ExternalName. |

### VirtualMachineServiceStatus



VirtualMachineServiceStatus defines the observed state of VirtualMachineService.

_Appears in:_
- [VirtualMachineService](#virtualmachineservice)

| Field | Description |
| --- | --- |
| `loadBalancer` _[LoadBalancerStatus](#loadbalancerstatus)_ | LoadBalancer contains the current status of the load balancer,
if one is present. |

### VirtualMachineSetResourcePolicySpec



VirtualMachineSetResourcePolicySpec defines the desired state of VirtualMachineSetResourcePolicy.

_Appears in:_
- [VirtualMachineSetResourcePolicy](#virtualmachinesetresourcepolicy)

| Field | Description |
| --- | --- |
| `resourcepool` _[ResourcePoolSpec](#resourcepoolspec)_ |  |
| `folder` _[FolderSpec](#folderspec)_ |  |
| `clustermodules` _[ClusterModuleSpec](#clustermodulespec) array_ |  |

### VirtualMachineSetResourcePolicyStatus



VirtualMachineSetResourcePolicyStatus defines the observed state of VirtualMachineSetResourcePolicy.

_Appears in:_
- [VirtualMachineSetResourcePolicy](#virtualmachinesetresourcepolicy)

| Field | Description |
| --- | --- |
| `clustermodules` _[ClusterModuleStatus](#clustermodulestatus) array_ |  |

### VirtualMachineSpec



VirtualMachineSpec defines the desired state of a VirtualMachine.

_Appears in:_
- [VirtualMachine](#virtualmachine)

| Field | Description |
| --- | --- |
| `imageName` _string_ | ImageName describes the name of a VirtualMachineImage that is to be used as the base Operating System image of
the desired VirtualMachine instances.  The VirtualMachineImage resources can be introspected to discover identifying
attributes that may help users to identify the desired image to use. |
| `className` _string_ | ClassName describes the name of a VirtualMachineClass that is to be used as the overlaid resource configuration
of VirtualMachine.  A VirtualMachineClass is used to further customize the attributes of the VirtualMachine
instance.  See VirtualMachineClass for more description. |
| `powerState` _VirtualMachinePowerState_ | PowerState describes the desired power state of a VirtualMachine.


Please note this field may be omitted when creating a new VM and will
default to "poweredOn." However, once the field is set to a non-empty
value, it may no longer be set to an empty value.


Additionally, setting this value to "suspended" is not supported when
creating a new VM. The valid values when creating a new VM are
"poweredOn" and "poweredOff." An empty value is also allowed on create
since this value defaults to "poweredOn" for new VMs. |
| `powerOffMode` _VirtualMachinePowerOpMode_ | PowerOffMode describes the desired behavior when powering off a VM.


There are three, supported power off modes: hard, soft, and
trySoft. The first mode, hard, is the equivalent of a physical
system's power cord being ripped from the wall. The soft mode
requires the VM's guest to have VM Tools installed and attempts to
gracefully shutdown the VM. Its variant, trySoft, first attempts
a graceful shutdown, and if that fails or the VM is not in a powered off
state after five minutes, the VM is halted.


If omitted, the mode defaults to hard. |
| `suspendMode` _VirtualMachinePowerOpMode_ | SuspendMode describes the desired behavior when suspending a VM.


There are three, supported suspend modes: hard, soft, and
trySoft. The first mode, hard, is where vSphere suspends the VM to
disk without any interaction inside of the guest. The soft mode
requires the VM's guest to have VM Tools installed and attempts to
gracefully suspend the VM. Its variant, trySoft, first attempts
a graceful suspend, and if that fails or the VM is not in a put into
standby by the guest after five minutes, the VM is suspended.


If omitted, the mode defaults to hard. |
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
| `restartMode` _VirtualMachinePowerOpMode_ | RestartMode describes the desired behavior for restarting a VM when
spec.nextRestartTime is set to "now" (case-insensitive).


There are three, supported suspend modes: hard, soft, and
trySoft. The first mode, hard, is where vSphere resets the VM without any
interaction inside of the guest. The soft mode requires the VM's guest to
have VM Tools installed and asks the guest to restart the VM. Its
variant, trySoft, first attempts a soft restart, and if that fails or
does not complete within five minutes, the VM is hard reset.


If omitted, the mode defaults to hard. |
| `ports` _[VirtualMachinePort](#virtualmachineport) array_ | Ports is currently unused and can be considered deprecated. |
| `vmMetadata` _[VirtualMachineMetadata](#virtualmachinemetadata)_ | VmMetadata describes any optional metadata that should be passed to the Guest OS. |
| `storageClass` _string_ | StorageClass describes the name of a StorageClass that should be used to configure storage-related attributes of the VirtualMachine
instance. |
| `networkInterfaces` _[VirtualMachineNetworkInterface](#virtualmachinenetworkinterface) array_ | NetworkInterfaces describes a list of VirtualMachineNetworkInterfaces to be configured on the VirtualMachine instance.
Each of these VirtualMachineNetworkInterfaces describes external network integration configurations that are to be
used by the VirtualMachine controller when integrating the VirtualMachine into one or more external networks.


The maximum number of network interface allowed is 10 because of the limit built into vSphere. |
| `resourcePolicyName` _string_ | ResourcePolicyName describes the name of a VirtualMachineSetResourcePolicy to be used when creating the
VirtualMachine instance. |
| `volumes` _[VirtualMachineVolume](#virtualmachinevolume) array_ | Volumes describes the list of VirtualMachineVolumes that are desired to be attached to the VirtualMachine.  Each of
these volumes specifies a volume identity that the VirtualMachine controller will attempt to satisfy, potentially
with an external Volume Management service. |
| `readinessProbe` _[Probe](#probe)_ | ReadinessProbe describes a network probe that can be used to determine if the VirtualMachine is available and
responding to the probe. |
| `advancedOptions` _[VirtualMachineAdvancedOptions](#virtualmachineadvancedoptions)_ | AdvancedOptions describes a set of optional, advanced options for configuring a VirtualMachine |
| `minHardwareVersion` _integer_ | MinHardwareVersion specifies the desired minimum hardware version
for this VM.


Usually the VM's hardware version is derived from:
1. the VirtualMachineClass used to deploy the VM provided by the ClassName field
2. the datacenter/cluster/host default hardware version
Setting this field will ensure that the hardware version of the VM
is at least set to the specified value. To enforce this, it will override
the value from the VirtualMachineClass.


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

### VirtualMachineStatus



VirtualMachineStatus defines the observed state of a VirtualMachine instance.

_Appears in:_
- [VirtualMachine](#virtualmachine)

| Field | Description |
| --- | --- |
| `host` _string_ | Host describes the hostname or IP address of the infrastructure host that the VirtualMachine is executing on. |
| `powerState` _VirtualMachinePowerState_ | PowerState describes the current power state of the VirtualMachine. |
| `phase` _VMStatusPhase_ | Phase describes the current phase information of the VirtualMachine. |
| `conditions` _[Condition](#condition) array_ | Conditions describes the current condition information of the VirtualMachine. |
| `vmIp` _string_ | VmIp describes the Primary IP address assigned to the guest operating system, if known.
Multiple IPs can be available for the VirtualMachine. Refer to networkInterfaces in the VirtualMachine
status for additional IPs |
| `uniqueID` _string_ | UniqueID describes a unique identifier that is provided by the underlying infrastructure provider, such as
vSphere. |
| `biosUUID` _string_ | BiosUUID describes a unique identifier provided by the underlying infrastructure provider that is exposed to the
Guest OS BIOS as a unique hardware identifier. |
| `instanceUUID` _string_ | InstanceUUID describes the unique instance UUID provided by the underlying infrastructure provider, such as vSphere. |
| `volumes` _[VirtualMachineVolumeStatus](#virtualmachinevolumestatus) array_ | Volumes describes a list of current status information for each Volume that is desired to be attached to the
VirtualMachine. |
| `changeBlockTracking` _boolean_ | ChangeBlockTracking describes the CBT enablement status on the VirtualMachine. |
| `networkInterfaces` _[NetworkInterfaceStatus](#networkinterfacestatus) array_ | NetworkInterfaces describes a list of current status information for each network interface that is desired to
be attached to the VirtualMachine. |
| `zone` _string_ | Zone describes the availability zone where the VirtualMachine has been scheduled.
Please note this field may be empty when the cluster is not zone-aware. |
| `lastRestartTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | LastRestartTime describes the last time the VM was restarted. |
| `hardwareVersion` _integer_ | HardwareVersion describes the VirtualMachine resource's observed
hardware version.


Please refer to VirtualMachineSpec.MinHardwareVersion for more
information on the topic of a VM's hardware version. |


### VirtualMachineVolume



VirtualMachineVolume describes a Volume that should be attached to a specific VirtualMachine.
Only one of PersistentVolumeClaim, VsphereVolume should be specified.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name specifies the name of the VirtualMachineVolume.  Each volume within the scope of a VirtualMachine must
have a unique name. |
| `persistentVolumeClaim` _[PersistentVolumeClaimVolumeSource](#persistentvolumeclaimvolumesource)_ | PersistentVolumeClaim represents a reference to a PersistentVolumeClaim
in the same namespace. The PersistentVolumeClaim must match one of the
following:


  * A volume provisioned (either statically or dynamically) by the
    cluster's CSI provider.


  * An instance volume with a lifecycle coupled to the VM. |
| `vSphereVolume` _[VsphereVolumeSource](#vspherevolumesource)_ | VsphereVolume represents a reference to a VsphereVolumeSource in the same namespace. Only one of PersistentVolumeClaim or
VsphereVolume can be specified. This is enforced via a webhook |

### VirtualMachineVolumeProvisioningOptions



VirtualMachineVolumeProvisioningOptions specifies the provisioning options for a VirtualMachineVolume.

_Appears in:_
- [VirtualMachineAdvancedOptions](#virtualmachineadvancedoptions)

| Field | Description |
| --- | --- |
| `thinProvisioned` _boolean_ | ThinProvisioned specifies whether to use thin provisioning for the VirtualMachineVolume.
This means a sparse (allocate on demand) format with additional space optimizations. |
| `eagerZeroed` _boolean_ | EagerZeroed specifies whether to use eager zero provisioning for the VirtualMachineVolume.
An eager zeroed thick disk has all space allocated and wiped clean of any previous contents
on the physical media at creation time. Such disks may take longer time during creation
compared to other disk formats.
EagerZeroed is only applicable if ThinProvisioned is false. This is validated by the webhook. |

### VirtualMachineVolumeStatus



VirtualMachineVolumeStatus defines the observed state of a VirtualMachineVolume instance.

_Appears in:_
- [VirtualMachineStatus](#virtualmachinestatus)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of the volume in a VirtualMachine. |
| `attached` _boolean_ | Attached represents whether a volume has been successfully attached to the VirtualMachine or not. |
| `diskUUID` _string_ | DiskUuid represents the underlying virtual disk UUID and is present when attachment succeeds. |
| `error` _string_ | Error represents the last error seen when attaching or detaching a volume.  Error will be empty if attachment succeeds. |

### VsphereVolumeSource



VsphereVolumeSource describes a volume source that represent static disks that belong to a VirtualMachine.

_Appears in:_
- [VirtualMachineVolume](#virtualmachinevolume)

| Field | Description |
| --- | --- |
| `capacity` _object (keys:[ResourceName](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcename-v1-core), values:Quantity)_ | A description of the virtual volume's resources and capacity |
| `deviceKey` _integer_ | Device key of vSphere disk. |

### WebConsoleRequestSpec



WebConsoleRequestSpec describes the specification for used to request a web console request.

_Appears in:_
- [WebConsoleRequest](#webconsolerequest)

| Field | Description |
| --- | --- |
| `virtualMachineName` _string_ | VirtualMachineName is the VM in the same namespace, for which the web console is requested. |
| `publicKey` _string_ | PublicKey is used to encrypt the status.response. This is expected to be a RSA OAEP public key in X.509 PEM format. |

### WebConsoleRequestStatus



WebConsoleRequestStatus defines the observed state, which includes the web console request itself.

_Appears in:_
- [WebConsoleRequest](#webconsolerequest)

| Field | Description |
| --- | --- |
| `response` _string_ | Response will be the authenticated ticket corresponding to this web console request. |
| `expiryTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | ExpiryTime is when the ticket referenced in Response will expire. |
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
