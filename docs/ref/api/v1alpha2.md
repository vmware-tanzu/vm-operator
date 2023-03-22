# v1alpha2



---

## Kinds


### ClusterVirtualMachineImage



ClusterVirtualMachineImage is the schema for the clustervirtualmachineimages API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha2`
| `kind` _string_ | `ClusterVirtualMachineImage`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineImageSpec](#virtualmachineimagespec)_ |  |
| `status` _[VirtualMachineImageStatus](#virtualmachineimagestatus)_ |  |

### VirtualMachine



VirtualMachine is the schema for the virtualmachines API and represents the desired state and observed status of a virtualmachines resource.

_Appears in:_
- [VirtualMachineTemplate](#virtualmachinetemplate)

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha2`
| `kind` _string_ | `VirtualMachine`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineSpec](#virtualmachinespec)_ |  |
| `status` _[VirtualMachineStatus](#virtualmachinestatus)_ |  |

### VirtualMachineClass



VirtualMachineClass is the schema for the virtualmachineclasses API and represents the desired state and observed status of a virtualmachineclasses resource.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha2`
| `kind` _string_ | `VirtualMachineClass`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineClassSpec](#virtualmachineclassspec)_ |  |
| `status` _[VirtualMachineClassStatus](#virtualmachineclassstatus)_ |  |

### VirtualMachineImage



VirtualMachineImage is the schema for the virtualmachineimages API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha2`
| `kind` _string_ | `VirtualMachineImage`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineImageSpec](#virtualmachineimagespec)_ |  |
| `status` _[VirtualMachineImageStatus](#virtualmachineimagestatus)_ |  |

### VirtualMachinePublishRequest



VirtualMachinePublishRequest defines the information necessary to publish a VirtualMachine as a VirtualMachineImage to an image registry.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha2`
| `kind` _string_ | `VirtualMachinePublishRequest`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachinePublishRequestSpec](#virtualmachinepublishrequestspec)_ |  |
| `status` _[VirtualMachinePublishRequestStatus](#virtualmachinepublishrequeststatus)_ |  |

### VirtualMachineService



VirtualMachineService is the Schema for the virtualmachineservices API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha2`
| `kind` _string_ | `VirtualMachineService`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineServiceSpec](#virtualmachineservicespec)_ |  |
| `status` _[VirtualMachineServiceStatus](#virtualmachineservicestatus)_ |  |

### VirtualMachineSetResourcePolicy



VirtualMachineSetResourcePolicy is the Schema for the virtualmachinesetresourcepolicies API.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha2`
| `kind` _string_ | `VirtualMachineSetResourcePolicy`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineSetResourcePolicySpec](#virtualmachinesetresourcepolicyspec)_ |  |
| `status` _[VirtualMachineSetResourcePolicyStatus](#virtualmachinesetresourcepolicystatus)_ |  |

### VirtualMachineWebConsoleRequest



VirtualMachineWebConsoleRequest allows the creation of a one-time, web console connection to a VM.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vmoperator.vmware.com/v1alpha2`
| `kind` _string_ | `VirtualMachineWebConsoleRequest`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VirtualMachineWebConsoleRequestSpec](#virtualmachinewebconsolerequestspec)_ |  |
| `status` _[VirtualMachineWebConsoleRequestStatus](#virtualmachinewebconsolerequeststatus)_ |  |


## Types
### DynamicDirectPathIODevice



DynamicDirectPathIODevice contains the configuration corresponding to a Dynamic DirectPath I/O device.

_Appears in:_
- [VirtualDevices](#virtualdevices)

| Field | Description |
| --- | --- |
| `vendorID` _integer_ |  |
| `deviceID` _integer_ |  |
| `customLabel` _string_ |  |

### GuestHeartbeatAction



GuestHeartbeatAction describes an action based on the guest heartbeat.

_Appears in:_
- [VirtualMachineReadinessProbeSpec](#virtualmachinereadinessprobespec)

| Field | Description |
| --- | --- |
| `thresholdStatus` _[GuestHeartbeatStatus](#guestheartbeatstatus)_ | ThresholdStatus is the value that the guest heartbeat status must be at or above to be considered successful. |

### GuestHeartbeatStatus

_Underlying type:_ `string`

GuestHeartbeatStatus is the guest heartbeat status.

_Appears in:_
- [GuestHeartbeatAction](#guestheartbeataction)


### GuestInfoAction



GuestInfoAction describes a key from GuestInfo that must match the associated value expression.

_Appears in:_
- [VirtualMachineReadinessProbeSpec](#virtualmachinereadinessprobespec)

| Field | Description |
| --- | --- |
| `key` _string_ | Key is the name of the GuestInfo key. 
 Values are automatically prefixed with "guestinfo." before being evaluated. Thus if the key "guestinfo.mykey" is provided, it will be evaluated as "guestinfo.guestinfo.mykey". |
| `value` _string_ | Value is a regular expression that is matched against the value of the specified key. 
 An empty value is the equivalent of "match any" or ".*". 
 All values must adhere to the RE2 regular expression syntax as documented at https://golang.org/s/re2syntax. Invalid values may be rejected or ignored depending on the implementation of this API. Either way, invalid values will not be considered when evaluating the ready state of a VM. |

### InstanceStorage



InstanceStorage provides information used to configure instance storage volumes for a VirtualMachine.

_Appears in:_
- [VirtualMachineClassHardware](#virtualmachineclasshardware)

| Field | Description |
| --- | --- |
| `storageClass` _string_ | StorageClass refers to the name of a StorageClass resource used to provide the storage for the configured instance storage volumes. The value of this field has no relationship to or bearing on the field virtualMachine.spec.storageClass. Please note the referred StorageClass must be available in the same namespace as the VirtualMachineClass that uses it for configuring instance storage. |
| `volumes` _[InstanceStorageVolume](#instancestoragevolume) array_ | Volumes describes instance storage volumes created for a VirtualMachine instance that use this VirtualMachineClass. |

### InstanceStorageVolume



InstanceStorageVolume contains information required to create an instance storage volume on a VirtualMachine.

_Appears in:_
- [InstanceStorage](#instancestorage)

| Field | Description |
| --- | --- |
| `size` _Quantity_ |  |

### LoadBalancerIngress



LoadBalancerIngress represents the status of a load balancer ingress point: traffic intended for the service should be sent to an ingress point. IP or Hostname may both be set in this structure. It is up to the consumer to determine which field should be used when accessing this LoadBalancer.

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
| `ingress` _[LoadBalancerIngress](#loadbalanceringress) array_ | Ingress is a list containing ingress addresses for the load balancer. Traffic intended for the service should be sent to any of these ingress points. |

### NetworkDeviceStatus



NetworkDeviceStatus defines the network interface IP configuration including gateway, subnet mask and IP address as seen by OVF properties.

_Appears in:_
- [NetworkStatus](#networkstatus)

| Field | Description |
| --- | --- |
| `Gateway4` _string_ | Gateway4 is the gateway for the IPv4 address family for this device. |
| `IPAddresses` _string array_ | IpAddresses represents one or more IP addresses assigned to the network device in CIDR notation, ex. "192.0.2.1/16". |

### NetworkStatus



NetworkStatus describes the observed state of the VM's network configuration.

_Appears in:_
- [VirtualMachineTemplate](#virtualmachinetemplate)

| Field | Description |
| --- | --- |
| `Devices` _[NetworkDeviceStatus](#networkdevicestatus) array_ | Devices describe a list of current status information for each network interface that is desired to be attached to the VirtualMachineTemplate. |
| `Nameservers` _string array_ | Nameservers describe a list of the DNS servers accessible by one of the VM's configured network devices. |

### OVFProperty



OVFProperty describes an OVF property associated with an image. OVF properties may be used in conjunction with the vAppConfig bootstrap provider to customize a VM during its creation.

_Appears in:_
- [VirtualMachineImageStatus](#virtualmachineimagestatus)

| Field | Description |
| --- | --- |
| `key` _string_ | Key describes the OVF property's key. |
| `type` _string_ | Type describes the OVF property's type. |
| `default` _string_ | Default describes the OVF property's default value. |

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
- [VirtualMachineReadinessProbeSpec](#virtualmachinereadinessprobespec)

| Field | Description |
| --- | --- |
| `port` _IntOrString_ | Port specifies a number or name of the port to access on the VM. If the format of port is a number, it must be in the range 1 to 65535. If the format of name is a string, it must be an IANA_SVC_NAME. |
| `host` _string_ | Host is an optional host name to connect to. Host defaults to the VM IP. |

### VGPUDevice



VGPUDevice contains the configuration corresponding to a vGPU device.

_Appears in:_
- [VirtualDevices](#virtualdevices)

| Field | Description |
| --- | --- |
| `profileName` _string_ |  |

### VSphereClusterModuleStatus



VSphereClusterModuleStatus describes the observed state of a vSphere cluster module.

_Appears in:_
- [VirtualMachineSetResourcePolicyStatus](#virtualmachinesetresourcepolicystatus)

| Field | Description |
| --- | --- |
| `groupName` _string_ |  |
| `moduleUUID` _string_ |  |
| `clusterMoID` _string_ |  |

### VirtualDevices



VirtualDevices contains information about the virtual devices associated with a VirtualMachineClass.

_Appears in:_
- [VirtualMachineClassHardware](#virtualmachineclasshardware)

| Field | Description |
| --- | --- |
| `vgpuDevices` _[VGPUDevice](#vgpudevice) array_ |  |
| `dynamicDirectPathIODevices` _[DynamicDirectPathIODevice](#dynamicdirectpathiodevice) array_ |  |

### VirtualMachineAdvancedSpec



VirtualMachineAdvancedSpec describes a set of optional, advanced VM configuration options.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `bootDiskCapacity` _Quantity_ | BootDiskCapacity is the capacity of the VM's boot disk -- the first disk from the VirtualMachineImage from which the VM was deployed. 
 Please note it is not advised to change this value while the VM is running. Also, resizing the VM's boot disk may require actions inside of the guest to take advantage of the additional capacity. Finally, changing the size of the VM's boot disk, even increasing it, could adversely affect the VM. |
| `defaultVolumeProvisioningMode` _string_ | DefaultVolumeProvisioningMode specifies the default provisioning mode for persistent volumes managed by this VM. |
| `changeBlockTracking` _boolean_ | ChangeBlockTracking is a flag that enables incremental backup support for this VM, a feature utilized by external backup systems such as VMware Data Recovery. |

### VirtualMachineBootstrapCloudInitSpec



VirtualMachineBootstrapCloudInitSpec describes the CloudInit configuration used to bootstrap the VM.

_Appears in:_
- [VirtualMachineBootstrapSpec](#virtualmachinebootstrapspec)

| Field | Description |
| --- | --- |
| `cloudConfig` _[CloudConfig](#cloudconfig)_ | CloudConfig describes a subset of a Cloud-Init CloudConfig, used to bootstrap the VM. 
 Please note this field and RawCloudConfig are mutually exclusive. |
| `rawCloudConfig` _[SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core)_ | RawCloudConfig describes a key in a Secret resource that contains the CloudConfig data used to bootstrap the VM. 
 The CloudConfig data specified by the key may be plain-text, base64-encoded, or gzipped and base64-encoded. 
 Please note this field and CloudConfig are mutually exclusive. |
| `sshAuthorizedKeys` _string array_ | SSHAuthorizedKeys is a list of public keys that CloudInit will apply to the guest's default user. |

### VirtualMachineBootstrapLinuxPrepSpec



VirtualMachineBootstrapLinuxPrepSpec

_Appears in:_
- [VirtualMachineBootstrapSpec](#virtualmachinebootstrapspec)

| Field | Description |
| --- | --- |
| `hardwareClockIsUTC` _boolean_ | HardwareClockIsUTC specifies whether the hardware clock is in UTC or local time. |
| `timeZone` _string_ | TimeZone is a case-sensitive timezone, such as Europe/Sofia. 
 Valid values are based on the tz (timezone) database used by Linux and other Unix systems. The values are strings in the form of "Area/Location," in which Area is a continent or ocean name, and Location is the city, island, or other regional designation. 
 Please see https://kb.vmware.com/s/article/2145518 for a list of valid time zones for Linux systems. |

### VirtualMachineBootstrapSpec



VirtualMachineBootstrapSpec defines the desired state of a VM's bootstrap configuration.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `cloudInit` _[VirtualMachineBootstrapCloudInitSpec](#virtualmachinebootstrapcloudinitspec)_ | CloudInit may be used to bootstrap Linux guests with Cloud-Init or Windows guests that support Cloudbase-Init. 
 The guest's networking stack is configured by Cloud-Init on Linux guests and Cloudbase-Init on Windows guests. 
 Please note this bootstrap provider may not be used in conjunction with the other bootstrap providers. |
| `linuxPrep` _[VirtualMachineBootstrapLinuxPrepSpec](#virtualmachinebootstraplinuxprepspec)_ | LinuxPrep may be used to bootstrap Linux guests. 
 The guest's networking stack is configured by Guest OS Customization (GOSC). 
 Please note this bootstrap provider may be used in conjunction with the VAppConfig bootstrap provider when wanting to configure the guest's network with GOSC but also send vApp/OVF properties into the guest. 
 This bootstrap provider may not be used in conjunction with the CloudInit or Sysprep bootstrap providers. |
| `sysprep` _[VirtualMachineBootstrapSysprepSpec](#virtualmachinebootstrapsysprepspec)_ | Sysprep may be used to bootstrap Windows guests. 
 The guest's networking stack is configured by Guest OS Customization (GOSC). 
 Please note this bootstrap provider may be used in conjunction with the VAppConfig bootstrap provider when wanting to configure the guest's network with GOSC but also send vApp/OVF properties into the guest. 
 This bootstrap provider may not be used in conjunction with the CloudInit or LinuxPrep bootstrap providers. |
| `vAppConfig` _[VirtualMachineBootstrapVAppConfigSpec](#virtualmachinebootstrapvappconfigspec)_ | VAppConfig may be used to bootstrap guests that rely on vApp properties (how VMware surfaces OVF properties on guests) to transport data into the guest. 
 The guest's networking stack may be configured using either vApp properties or GOSC. 
 Many OVFs define one or more properties that are used by the guest to bootstrap its networking stack. If the VirtualMachineImage defines one or more properties like this, then they can be configured to use the network data provided for this VM at runtime by setting these properties to Go template strings. 
 It is also possible to use GOSC to bootstrap this VM's network stack by configuring either the LinuxPrep or Sysprep bootstrap providers. 
 Please note the VAppConfig bootstrap provider in conjunction with the LinuxPrep bootstrap provider is the equivalent of setting the v1alpha1 VM metadata transport to "OvfEnv". 
 This bootstrap provider may not be used in conjunction with the CloudInit bootstrap provider. |

### VirtualMachineBootstrapSysprepSpec



VirtualMachineBootstrapSysprepSpec describes the Sysprep configuration used to bootstrap the VM.

_Appears in:_
- [VirtualMachineBootstrapSpec](#virtualmachinebootstrapspec)

| Field | Description |
| --- | --- |
| `sysprep` _[Sysprep](#sysprep)_ | Sysprep is an object representation of a Windows sysprep.xml answer file. 
 This field encloses all the individual keys listed in a sysprep.xml file. 
 For more detailed information please see https://technet.microsoft.com/en-us/library/cc771830(v=ws.10).aspx. 
 Please note this field and RawSysprep are mutually exclusive. |
| `rawSysprep` _[SecretKeySelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core)_ | RawSysprep describes a key in a Secret resource that contains an XML string of the Sysprep text used to bootstrap the VM. 
 The data specified by the Secret key may be plain-text, base64-encoded, or gzipped and base64-encoded. 
 Please note this field and Sysprep are mutually exclusive. |

### VirtualMachineBootstrapVAppConfigSpec



VirtualMachineBootstrapVAppConfigSpec

_Appears in:_
- [VirtualMachineBootstrapSpec](#virtualmachinebootstrapspec)

| Field | Description |
| --- | --- |
| `properties` _[KeyValueOrSecretKeySelectorPair](#keyvalueorsecretkeyselectorpair) array_ | Properties is a list of vApp/OVF property key/value pairs. 
 Please note this field and RawProperties are mutually exclusive. |
| `rawProperties` _string_ | RawProperties is the name of a Secret resource in the same Namespace as this VM where each key/value pair from the Secret is used as a vApp key/value pair. 
 Please note this field and Properties are mutually exclusive. |

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



VirtualMachineClassResources describes the virtual hardware resource reservations and limits configuration to be used by a VirtualMachineClass.

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
| `hardware` _[VirtualMachineClassHardware](#virtualmachineclasshardware)_ | Hardware describes the configuration of the VirtualMachineClass attributes related to virtual hardware. The configuration specified in this field is used to customize the virtual hardware characteristics of any VirtualMachine associated with this VirtualMachineClass. |
| `policies` _[VirtualMachineClassPolicies](#virtualmachineclasspolicies)_ | Policies describes the configuration of the VirtualMachineClass attributes related to virtual infrastructure policy. The configuration specified in this field is used to customize various policies related to infrastructure resource consumption. |
| `description` _string_ | Description describes the configuration of the VirtualMachineClass which is not related to virtual hardware or infrastructure policy. This field is used to address remaining specs about this VirtualMachineClass. |
| `configSpec` _[json.RawMessage](https://pkg.go.dev/encoding/json#RawMessage)_ | ConfigSpec describes additional configuration information for a VirtualMachine. The contents of this field are the VirtualMachineConfigSpec data object (https://bit.ly/3HDtiRu) marshaled to JSON using the discriminator field "_typeName" to preserve type information. |

### VirtualMachineClassStatus



VirtualMachineClassStatus defines the observed state of VirtualMachineClass.

_Appears in:_
- [VirtualMachineClass](#virtualmachineclass)

| Field | Description |
| --- | --- |
| `capabilities` _string array_ | Capabilities describes the class's observed capabilities. 
 The capabilities are discerned when VM Operator reconciles a class and inspects its specification. Well-known capabilities include: 
 * instance-storage * nvidia-gpu * sriov-net 
 In addition to "nvidia-gpu", a capability is added for every nVidia profile name associated with the class. 
 Every capability is also added to the resource's labels as VirtualMachineClassCapabilityLabel + Value. For example, if the capability is "nvidia-gpu" then the following label will be added to the resource: capability.class.vmoperator.vmware.com/nvidia-gpu. |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions describes the observed conditions of the VirtualMachineClass. |
| `ready` _boolean_ | Ready describes whether the class's hardware can be realized in the cluster. 
 This field is only set to true if all of the class resource's conditions have Status=True. |


### VirtualMachineImageOSInfo



VirtualMachineImageOSInfo describes the image's guest operating system.

_Appears in:_
- [VirtualMachineImageStatus](#virtualmachineimagestatus)

| Field | Description |
| --- | --- |
| `id` _string_ | ID describes the operating system ID. 
 This value is also added to the image resource's labels as VirtualMachineImageOSIDLabel. |
| `type` _string_ | Type describes the operating system type. 
 This value is also added to the image resource's labels as VirtualMachineImageOSTypeLabel. |
| `version` _string_ | Version describes the operating system version. 
 This value is also added to the image resource's labels as VirtualMachineImageOSVersionLabel. |

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

### VirtualMachineImageSpec



VirtualMachineImageSpec defines the desired state of VirtualMachineImage.

_Appears in:_
- [ClusterVirtualMachineImage](#clustervirtualmachineimage)
- [VirtualMachineImage](#virtualmachineimage)

| Field | Description |
| --- | --- |
| `providerRef` _[LocalObjectRef](#localobjectref)_ | ProviderRef is a reference to the resource that contains the source of this image's information. |

### VirtualMachineImageStatus



VirtualMachineImageStatus defines the observed state of VirtualMachineImage.

_Appears in:_
- [ClusterVirtualMachineImage](#clustervirtualmachineimage)
- [VirtualMachineImage](#virtualmachineimage)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the observed, "friendly" name for this image. |
| `capabilities` _string array_ | Capabilities describes the image's observed capabilities. 
 The capabilities are discerned when VM Operator reconciles an image. If the source of an image is an OVF in Content Library, then the capabilities are parsed from the OVF property capabilities.image.vmoperator.vmware.com as a comma-separated list of values. Well-known capabilities include: 
 * cloud-init * nvidia-gpu * sriov-net 
 Every capability is also added to the resource's labels as VirtualMachineImageCapabilityLabel + Value. For example, if the capability is "cloud-init" then the following label will be added to the resource: capability.image.vmoperator.vmware.com/cloud-init. |
| `firmware` _string_ | Firmware describe the firmware type used by this image, ex. BIOS, EFI. |
| `hardwareVersion` _integer_ | HardwareVersion describes the observed hardware version of this image. |
| `osInfo` _[VirtualMachineImageOSInfo](#virtualmachineimageosinfo)_ | OSInfo describes the observed operating system information for this image. 
 The OS information is also added to the image resource's labels. Please refer to VirtualMachineImageOSInfo for more information. |
| `ovfProperties` _[OVFProperty](#ovfproperty) array_ | OVFProperties describes the observed OVF properties defined for this image. |
| `productInfo` _[VirtualMachineImageProductInfo](#virtualmachineimageproductinfo)_ | ProductInfo describes the observed product information for this image. |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions describes the observed conditions for this image. |

### VirtualMachineNetworkDHCPOptionsStatus



VirtualMachineNetworkDHCPOptionsStatus describes the observed state of DHCP options.

_Appears in:_
- [VirtualMachineNetworkDHCPStatus](#virtualmachinenetworkdhcpstatus)

| Field | Description |
| --- | --- |
| `config` _[KeyValuePair](#keyvaluepair) array_ | Config describes platform-dependent settings for the DHCP client. 
 The key part is a unique number while the value part is the platform specific configuration command. For example on Linux and BSD systems using the file dhclient.conf output would be reported at system scope: key='1', value='timeout 60;' key='2', value='reboot 10;'. The output reported per interface would be: key='1', value='prepend domain-name-servers 192.0.2.1;' key='2', value='require subnet-mask, domain-name-servers;'. |
| `enabled` _boolean_ | Enabled reports the status of the DHCP client services. |

### VirtualMachineNetworkDHCPStatus



VirtualMachineNetworkDHCPStatus describes the observed state of the client-side, system-wide DHCP settings for IP4 and IP6.

_Appears in:_
- [VirtualMachineNetworkIPStackStatus](#virtualmachinenetworkipstackstatus)
- [VirtualMachineNetworkInterfaceIPStatus](#virtualmachinenetworkinterfaceipstatus)

| Field | Description |
| --- | --- |
| `ip4` _[VirtualMachineNetworkDHCPOptionsStatus](#virtualmachinenetworkdhcpoptionsstatus)_ | IP4 describes the observed state of the IP4 DHCP client settings. |
| `ip6` _[VirtualMachineNetworkDHCPOptionsStatus](#virtualmachinenetworkdhcpoptionsstatus)_ | IP6 describes the observed state of the IP6 DHCP client settings. |

### VirtualMachineNetworkDNSStatus



VirtualMachineNetworkDNSStatus describes the observed state of the guest's RFC 1034 client-side DNS settings.

_Appears in:_
- [VirtualMachineNetworkIPStackStatus](#virtualmachinenetworkipstackstatus)
- [VirtualMachineNetworkInterfaceStatus](#virtualmachinenetworkinterfacestatus)

| Field | Description |
| --- | --- |
| `dhcp` _boolean_ | DHCP indicates whether or not dynamic host control protocol (DHCP) was used to configure DNS configuration. |
| `domainName` _string_ | DomainName is the domain name portion of the DNS name. For example, the "domain.local" part of "my-vm.domain.local". |
| `hostName` _string_ | HostName is the host name portion of the DNS name. For example, the "my-vm" part of "my-vm.domain.local". |
| `nameservers` _string array_ | Nameservers is a list of the IP addresses for the DNS servers to use. 
 IP4 addresses are specified using dotted decimal notation. For example, "192.0.2.1". 
 IP6 addresses are 128-bit addresses represented as eight fields of up to four hexadecimal digits. A colon separates each field (:). For example, 2001:DB8:101::230:6eff:fe04:d9ff. The address can also consist of the symbol '::' to represent multiple 16-bit groups of contiguous 0's only once in an address as described in RFC 2373. |
| `searchDomains` _string array_ | SearchDomains is a list of domains in which to search for hosts, in the order of preference. |

### VirtualMachineNetworkIPRouteGatewayStatus



VirtualMachineNetworkIPRouteGatewayStatus describes the observed state of a guest network's IP route's next hop gateway.

_Appears in:_
- [VirtualMachineNetworkIPRouteStatus](#virtualmachinenetworkiproutestatus)

| Field | Description |
| --- | --- |
| `device` _string_ | Device is the name of the device in the guest for which this gateway applies. |
| `address` _string_ | Address is the IP4 or IP6 address of the gateway. |

### VirtualMachineNetworkIPRouteStatus



VirtualMachineNetworkIPRouteStatus describes the observed state of a guest network's IP routes.

_Appears in:_
- [VirtualMachineNetworkIPStackStatus](#virtualmachinenetworkipstackstatus)
- [VirtualMachineNetworkRouteStatus](#virtualmachinenetworkroutestatus)

| Field | Description |
| --- | --- |
| `gateway` _[VirtualMachineNetworkIPRouteGatewayStatus](#virtualmachinenetworkiproutegatewaystatus)_ | Gateway describes where to send the packets to next. |
| `networkAddress` _string_ | NetworkAddress is the IP4 or IP6 address of the destination network. 
 Addresses include the network's prefix length, ex. 192.168.0.0/24 or 2001:DB8:101::230:6eff:fe04:d9ff::/64. 
 IP6 addresses are 128-bit addresses represented as eight fields of up to four hexadecimal digits. A colon separates each field (:). For example, 2001:DB8:101::230:6eff:fe04:d9ff. The address can also consist of symbol '::' to represent multiple 16-bit groups of contiguous 0's only once in an address as described in RFC 2373. |

### VirtualMachineNetworkIPStackStatus



VirtualMachineNetworkIPStackStatus describes the observed state of a a VM's IP stack.

_Appears in:_
- [VirtualMachineNetworkStatus](#virtualmachinenetworkstatus)

| Field | Description |
| --- | --- |
| `dhcp` _[VirtualMachineNetworkDHCPStatus](#virtualmachinenetworkdhcpstatus)_ | DHCP describes the VM's observed, client-side, system-wide DHCP options. |
| `dns` _[VirtualMachineNetworkDNSStatus](#virtualmachinenetworkdnsstatus)_ | DNS describes the VM's observed, client-side DNS configuration. |
| `ipRoutes` _[VirtualMachineNetworkIPRouteStatus](#virtualmachinenetworkiproutestatus) array_ | IPRoutes contain the VM's routing tables for all address families. |
| `kernelConfig` _[KeyValuePair](#keyvaluepair) array_ | KernelConfig describes the observed state of the VM's kernel IP configuration settings. 
 The key part contains a unique number while the value part contains the 'key=value' as provided by the underlying provider. For example, on Linux and/or BSD, the systcl -a output would be reported as: key='5', value='net.ipv4.tcp_keepalive_time = 7200'. |

### VirtualMachineNetworkInterfaceIPAddrStatus



VirtualMachineNetworkInterfaceIPAddrStatus describes information about a specific IP address.

_Appears in:_
- [VirtualMachineNetworkInterfaceIPStatus](#virtualmachinenetworkinterfaceipstatus)

| Field | Description |
| --- | --- |
| `address` _string_ | Address is an IP4 or IP6 address and their network prefix length. 
 An IP4 address is specified using dotted decimal notation. For example, "192.0.2.1". 
 IP6 addresses are 128-bit addresses represented as eight fields of up to four hexadecimal digits. A colon separates each field (:). For example, 2001:DB8:101::230:6eff:fe04:d9ff. The address can also consist of the symbol '::' to represent multiple 16-bit groups of contiguous 0's only once in an address as described in RFC 2373. |
| `lifetime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | Lifetime describes when this address will expire. |
| `origin` _string_ | Origin describes how this address was configured. |
| `state` _string_ | State describes the state of this IP address. |

### VirtualMachineNetworkInterfaceIPStatus



VirtualMachineNetworkInterfaceIPStatus describes the observed state of a VM's network interface's IP configuration.

_Appears in:_
- [VirtualMachineNetworkInterfaceStatus](#virtualmachinenetworkinterfacestatus)

| Field | Description |
| --- | --- |
| `autoConfigurationEnabled` _boolean_ | AutoConfigurationEnabled describes whether or not ICMPv6 router solicitation requests are enabled or disabled from a given interface. 
 These requests acquire an IP6 address and default gateway route from zero-to-many routers on the connected network. 
 If not set then ICMPv6 is not available on this VM. |
| `dhcp` _[VirtualMachineNetworkDHCPStatus](#virtualmachinenetworkdhcpstatus)_ | DHCP describes the VM's observed, client-side, interface-specific DHCP options. |
| `addresses` _[VirtualMachineNetworkInterfaceIPAddrStatus](#virtualmachinenetworkinterfaceipaddrstatus) array_ | Addresses describes observed IP addresses for this interface. |
| `macAddr` _string_ | MACAddr describes the observed MAC address for this interface. |

### VirtualMachineNetworkInterfaceSpec



VirtualMachineNetworkInterfaceSpec describes the desired state of a VM's network interface.

_Appears in:_
- [VirtualMachineNetworkSpec](#virtualmachinenetworkspec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the unique name of this network interface, used to distinguish it from other network interfaces attached to this VM. 
 This value is also used to rename the device inside the guest when the bootstrap provider is CloudInit. Please note it is up to the user to ensure the provided device name does not conflict with any other devices inside the guest, ex. dvd, cdrom, sda, etc. |
| `network` _[PartialObjectRef](#partialobjectref)_ | Network is the name of the network resource to which this interface is connected. 
 If no network is provided, then this interface will be connected to the Namespace's default network. |
| `addresses` _string array_ | Addresses is an optional list of IP4 or IP6 addresses to assign to this interface. 
 Please note this field is only supported if the connected network supports manual IP allocation. 
 Please note IP4 and IP6 addresses must include the network prefix length, ex. 192.168.0.10/24 or 2001:db8:101::a/64. 
 Please note this field may not contain IP4 addresses if DHCP4 is set to true or IP6 addresses if DHCP6 is set to true. 
 Please note if the Interfaces field is non-empty then this field is ignored and should be specified on the elements in the Interfaces list. |
| `dhcp4` _boolean_ | DHCP4 indicates whether or not this interface uses DHCP for IP4 networking. 
 Please note this field is only supported if the network connection supports DHCP. 
 Please note this field is mutually exclusive with IP4 addresses in the Addresses field and the Gateway4 field. |
| `dhcp6` _boolean_ | DHCP6 indicates whether or not this interface uses DHCP for IP6 networking. 
 Please note this field is only supported if the network connection supports DHCP. 
 Please note this field is mutually exclusive with IP4 addresses in the Addresses field and the Gateway6 field. |
| `gateway4` _string_ | Gateway4 is the default, IP4 gateway for this interface. 
 Please note this field is only supported if the network connection supports manual IP allocation. 
 If the network connection supports manual IP allocation and the Addresses field includes at least one IP4 address, then this field is required. 
 Please note the IP address must include the network prefix length, ex. 192.168.0.1/24. 
 Please note this field is mutually exclusive with DHCP4. |
| `gateway6` _string_ | Gateway6 is the primary IP6 gateway for this interface. 
 Please note this field is only supported if the network connection supports manual IP allocation. 
 If the network connection supports manual IP allocation and the Addresses field includes at least one IP4 address, then this field is required. 
 Please note the IP address must include the network prefix length, ex. 2001:db8:101::1/64. 
 Please note this field is mutually exclusive with DHCP6. |
| `mtu` _integer_ | MTU is the Maximum Transmission Unit size in bytes. 
 Please note this feature is available only with the following bootstrap providers: CloudInit. |
| `nameservers` _string array_ | Nameservers is a list of IP4 and/or IP6 addresses used as DNS nameservers. 
 Please note this feature is available only with the following bootstrap providers: CloudInit, LinuxPrep, and Sysprep. 
 Please note that Linux allows only three nameservers (https://linux.die.net/man/5/resolv.conf). |
| `routes` _[VirtualMachineNetworkRouteSpec](#virtualmachinenetworkroutespec) array_ | Routes is a list of optional, static routes. 
 Please note this feature is available only with the following bootstrap providers: CloudInit. |
| `searchDomains` _string array_ | SearchDomains is a list of search domains used when resolving IP addresses with DNS. 
 Please note this feature is available only with the following bootstrap providers: CloudInit, LinuxPrep, and Sysprep. |

### VirtualMachineNetworkInterfaceStatus



VirtualMachineNetworkInterfaceStatus describes the observed state of a VM's network interface.

_Appears in:_
- [VirtualMachineNetworkStatus](#virtualmachinenetworkstatus)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the unique name of this network interface, used to distinguish it from other network interfaces attached to this VM. 
 Please note this name is not related to the name of the device as it is surfaced inside of the guest. |
| `ip` _[VirtualMachineNetworkInterfaceIPStatus](#virtualmachinenetworkinterfaceipstatus)_ | IP describes the observed state of the interface's IP configuration. |
| `dns` _[VirtualMachineNetworkDNSStatus](#virtualmachinenetworkdnsstatus)_ | DNS describes the observed state of the interface's DNS configuration. |

### VirtualMachineNetworkRouteSpec



VirtualMachineNetworkRouteSpec defines a static route for a guest.

_Appears in:_
- [VirtualMachineNetworkInterfaceSpec](#virtualmachinenetworkinterfacespec)
- [VirtualMachineNetworkSpec](#virtualmachinenetworkspec)

| Field | Description |
| --- | --- |
| `to` _string_ | To is an IP4 address. |
| `via` _string_ | Via is an IP4 address. |
| `metric` _integer_ | Metric is the weight/priority of the route. |


### VirtualMachineNetworkSpec



VirtualMachineNetworkSpec defines a VM's desired network configuration.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `network` _[PartialObjectRef](#partialobjectref)_ | Network is the optional name of the network resource to which this VM is connected. 
 Please note if the Interfaces field is non-empty then this field is ignored. 
 If networking is not disabled, no interfaces are defined, and this value is omitted, then the VM will be provided a single virtual network interface and connected to the Namespace's default network. |
| `disabled` _boolean_ | Disabled is a flag that indicates whether or not to disable networking for this VM. 
 When set to true, the VM is not configured with a default interface nor any specified from the Interfaces field. |
| `hostName` _string_ | HostName is the value the guest uses as its host name. If omitted then the name of the VM will be used. 
 Please note this feature is available only with the following bootstrap providers: CloudInit, LinuxPrep, and Sysprep. |
| `interfaces` _[VirtualMachineNetworkInterfaceSpec](#virtualmachinenetworkinterfacespec) array_ | Interfaces is the list of network interfaces used by this VM. 
 Please note this field is mutually exclusive with the following fields: DeviceName, Network, Addresses, DHCP4, DHCP6, Gateway4, Gateway6, MTU, Nameservers, Routes, and SearchDomains. |
| `deviceName` _string_ | DeviceName describes the unique name of this network interface, used to distinguish it from other network interfaces attached to this VM. 
 This value is also used to rename the device inside the guest when the bootstrap provider is CloudInit. Please note it is up to the user to ensure the provided device name does not conflict with any other devices inside the guest, ex. dvd, cdrom, sda, etc. 
 Please note if the Interfaces field is non-empty then this field is ignored and should be specified on the elements in the Interfaces list. 
 If the Interfaces field is empty and this field is not specified, then the default interface's name will be eth0. |
| `addresses` _string array_ | Addresses is an optional list of IP4 or IP6 addresses to assign to the VM. 
 Please note this field is only supported if the connected network supports manual IP allocation. 
 Please note IP4 and IP6 addresses must include the network prefix length, ex. 192.168.0.10/24 or 2001:db8:101::a/64. 
 Please note this field may not contain IP4 addresses if DHCP4 is set to true or IP6 addresses if DHCP6 is set to true. 
 Please note if the Interfaces field is non-empty then this field is ignored and should be specified on the elements in the Interfaces list. |
| `dhcp4` _boolean_ | DHCP4 indicates whether or not to use DHCP for IP4 networking. 
 Please note this field is only supported if the network connection supports DHCP. 
 Please note this field is mutually exclusive with IP4 addresses in the Addresses field and the Gateway4 field. 
 Please note if the Interfaces field is non-empty then this field is ignored and should be specified on the elements in the Interfaces list. |
| `dhcp6` _boolean_ | DHCP6 indicates whether or not to use DHCP for IP6 networking. 
 Please note this field is only supported if the network connection supports DHCP. 
 Please note this field is mutually exclusive with IP4 addresses in the Addresses field and the Gateway6 field. 
 Please note if the Interfaces field is non-empty then this field is ignored and should be specified on the elements in the Interfaces list. |
| `gateway4` _string_ | Gateway4 is the default, IP4 gateway for this VM. 
 Please note this field is only supported if the network connection supports manual IP allocation. 
 If the network connection supports manual IP allocation and the Addresses field includes at least one IP4 address, then this field is required. 
 Please note the IP address must include the network prefix length, ex. 192.168.0.1/24. 
 Please note this field is mutually exclusive with DHCP4. 
 Please note if the Interfaces field is non-empty then this field is ignored and should be specified on the elements in the Interfaces list. |
| `gateway6` _string_ | Gateway6 is the primary IP6 gateway for this VM. 
 Please note this field is only supported if the network connection supports manual IP allocation. 
 If the network connection supports manual IP allocation and the Addresses field includes at least one IP4 address, then this field is required. 
 Please note the IP address must include the network prefix length, ex. 2001:db8:101::1/64. 
 Please note this field is mutually exclusive with DHCP6. 
 Please note if the Interfaces field is non-empty then this field is ignored and should be specified on the elements in the Interfaces list. |
| `mtu` _integer_ | MTU is the Maximum Transmission Unit size in bytes. 
 Please note this feature is available only with the following bootstrap providers: CloudInit. 
 Please note if the Interfaces field is non-empty then this field is ignored and should be specified on the elements in the Interfaces list. |
| `nameservers` _string array_ | Nameservers is a list of IP4 and/or IP6 addresses used as DNS nameservers. 
 Please note this feature is available only with the following bootstrap providers: CloudInit, LinuxPrep, and Sysprep. 
 Please note that Linux allows only three nameservers (https://linux.die.net/man/5/resolv.conf). 
 Please note if the Interfaces field is non-empty then this field is ignored and should be specified on the elements in the Interfaces list. |
| `routes` _[VirtualMachineNetworkRouteSpec](#virtualmachinenetworkroutespec) array_ | Routes is a list of optional, static routes. 
 Please note this feature is available only with the following bootstrap providers: CloudInit. 
 Please note if the Interfaces field is non-empty then this field is ignored and should be specified on the elements in the Interfaces list. |
| `searchDomains` _string array_ | SearchDomains is a list of search domains used when resolving IP addresses with DNS. 
 Please note this feature is available only with the following bootstrap providers: CloudInit, LinuxPrep, and Sysprep. 
 Please note if the Interfaces field is non-empty then this field is ignored and should be specified on the elements in the Interfaces list. |

### VirtualMachineNetworkStatus



VirtualMachineNetworkStatus defines the observed state of a VM's network configuration.

_Appears in:_
- [VirtualMachineStatus](#virtualmachinestatus)

| Field | Description |
| --- | --- |
| `interfaces` _[VirtualMachineNetworkInterfaceStatus](#virtualmachinenetworkinterfacestatus) array_ | Interfaces describes the status of the VM's network interfaces. |
| `primaryIP4` _string_ | PrimaryIP4 describes the VM's primary IP4 address. 
 If the bootstrap provider is CloudInit then this value is set to the value of the VM's "guestinfo.local-ipv4" property. Please see https://bit.ly/3A66vZg for more information on how this value is calculated. 
 If the bootstrap provider is anything else then this field is set to the the value of the infrastructure VM's "guest.ipAddress" field. Please see https://bit.ly/3Au0jM4 for more information. |
| `primaryIP6` _string_ | PrimaryIP6 describes the VM's primary IP6 address. 
 If the bootstrap provider is CloudInit then this value is set to the value of the VM's "guestinfo.local-ipv6" property. Please see https://bit.ly/3A66vZg for more information on how this value is calculated. 
 If the bootstrap provider is anything else then this field is set to the the value of the infrastructure VM's "guest.ipAddress" field. Please see https://bit.ly/3Au0jM4 for more information. |
| `dhcp` _[VirtualMachineNetworkDHCPStatus](#virtualmachinenetworkdhcpstatus)_ | DHCP describes the VM's observed, client-side, system-wide DHCP options. |
| `dns` _[VirtualMachineNetworkDNSStatus](#virtualmachinenetworkdnsstatus)_ | DNS describes the VM's observed, client-side DNS configuration. |
| `ipRoutes` _[VirtualMachineNetworkIPRouteStatus](#virtualmachinenetworkiproutestatus) array_ | IPRoutes contain the VM's routing tables for all address families. |
| `kernelConfig` _[KeyValuePair](#keyvaluepair) array_ | KernelConfig describes the observed state of the VM's kernel IP configuration settings. 
 The key part contains a unique number while the value part contains the 'key=value' as provided by the underlying provider. For example, on Linux and/or BSD, the systcl -a output would be reported as: key='5', value='net.ipv4.tcp_keepalive_time = 7200'. |

### VirtualMachinePublishRequestSource



VirtualMachinePublishRequestSource is the source of a publication request, typically a VirtualMachine resource.

_Appears in:_
- [VirtualMachinePublishRequestSpec](#virtualmachinepublishrequestspec)
- [VirtualMachinePublishRequestStatus](#virtualmachinepublishrequeststatus)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of the referenced object. 
 If omitted this value defaults to the name of the VirtualMachinePublishRequest resource. |
| `apiVersion` _string_ | APIVersion is the API version of the referenced object. |
| `kind` _string_ | Kind is the kind of referenced object. |

### VirtualMachinePublishRequestSpec



VirtualMachinePublishRequestSpec defines the desired state of a VirtualMachinePublishRequest. 
 All the fields in this spec are optional. This is especially useful when a DevOps persona wants to publish a VM without doing anything more than applying a VirtualMachinePublishRequest resource that has the same name as said VM in the same namespace as said VM.

_Appears in:_
- [VirtualMachinePublishRequest](#virtualmachinepublishrequest)

| Field | Description |
| --- | --- |
| `source` _[VirtualMachinePublishRequestSource](#virtualmachinepublishrequestsource)_ | Source is the source of the publication request, ex. a VirtualMachine resource. 
 If this value is omitted then the publication controller checks to see if there is a resource with the same name as this VirtualMachinePublishRequest resource, an API version equal to spec.source.apiVersion, and a kind equal to spec.source.kind. If such a resource exists, then it is the source of the publication. |
| `target` _[VirtualMachinePublishRequestTarget](#virtualmachinepublishrequesttarget)_ | Target is the target of the publication request, ex. item information and a ContentLibrary resource. 
 If this value is omitted, the controller uses spec.source.name + "-image" as the name of the published item. Additionally, when omitted the controller attempts to identify the target location by matching a resource with an API version equal to spec.target.location.apiVersion, a kind equal to spec.target.location.kind, w/ the label "imageregistry.vmware.com/default". 
 Please note that while optional, if a VirtualMachinePublishRequest sans target information is applied to a namespace without a default publication target, then the VirtualMachinePublishRequest resource will be marked in error. |
| `ttlSecondsAfterFinished` _integer_ | TTLSecondsAfterFinished is the time-to-live duration for how long this resource will be allowed to exist once the publication operation completes. After the TTL expires, the resource will be automatically deleted without the user having to take any direct action. 
 If this field is unset then the request resource will not be automatically deleted. If this field is set to zero then the request resource is eligible for deletion immediately after it finishes. |

### VirtualMachinePublishRequestStatus



VirtualMachinePublishRequestStatus defines the observed state of a VirtualMachinePublishRequest.

_Appears in:_
- [VirtualMachinePublishRequest](#virtualmachinepublishrequest)

| Field | Description |
| --- | --- |
| `sourceRef` _[VirtualMachinePublishRequestSource](#virtualmachinepublishrequestsource)_ | SourceRef is the reference to the source of the publication request, ex. a VirtualMachine resource. |
| `targetRef` _[VirtualMachinePublishRequestTarget](#virtualmachinepublishrequesttarget)_ | TargetRef is the reference to the target of the publication request, ex. item information and a ContentLibrary resource. |
| `completionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | CompletionTime represents time when the request was completed. It is not guaranteed to be set in happens-before order across separate operations. It is represented in RFC3339 form and is in UTC. 
 The value of this field should be equal to the value of the LastTransitionTime for the status condition Type=Complete. |
| `startTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | StartTime represents time when the request was acknowledged by the controller. It is not guaranteed to be set in happens-before order across separate operations. It is represented in RFC3339 form and is in UTC. |
| `attempts` _integer_ | Attempts represents the number of times the request to publish the VM has been attempted. |
| `lastAttemptTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | LastAttemptTime represents the time when the latest request was sent. |
| `imageName` _string_ | ImageName is the name of the VirtualMachineImage resource that is eventually realized in the same namespace as the VM and publication request after the publication operation completes. 
 This field will not be set until the VirtualMachineImage resource is realized. |
| `ready` _boolean_ | Ready is set to true only when the VM has been published successfully and the new VirtualMachineImage resource is ready. 
 Readiness is determined by waiting until there is status condition Type=Complete and ensuring it and all other status conditions present have a Status=True. The conditions present will be: 
 * SourceValid * TargetValid * Uploaded * ImageAvailable * Complete |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions is a list of the latest, available observations of the request's current state. |

### VirtualMachinePublishRequestTarget



VirtualMachinePublishRequestTarget is the target of a publication request, typically a ContentLibrary resource.

_Appears in:_
- [VirtualMachinePublishRequestSpec](#virtualmachinepublishrequestspec)
- [VirtualMachinePublishRequestStatus](#virtualmachinepublishrequeststatus)

| Field | Description |
| --- | --- |
| `item` _[VirtualMachinePublishRequestTargetItem](#virtualmachinepublishrequesttargetitem)_ | Item contains information about the name of the object to which the VM is published. 
 Please note this value is optional and if omitted, the controller will use spec.source.name + "-image" as the name of the published item. |
| `location` _[VirtualMachinePublishRequestTargetLocation](#virtualmachinepublishrequesttargetlocation)_ | Location contains information about the location to which to publish the VM. |

### VirtualMachinePublishRequestTargetItem



VirtualMachinePublishRequestTargetItem is the item part of a publication request's target.

_Appears in:_
- [VirtualMachinePublishRequestTarget](#virtualmachinepublishrequesttarget)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of the published object. 
 If the spec.target.location.apiVersion equals imageregistry.vmware.com/v1alpha1 and the spec.target.location.kind equals ContentLibrary, then this should be the name that will show up in vCenter Content Library, not the custom resource name in the namespace. 
 If omitted then the controller will use spec.source.name + "-image". |
| `description` _string_ | Description is the description to assign to the published object. |

### VirtualMachinePublishRequestTargetLocation



VirtualMachinePublishRequestTargetLocation is the location part of a publication request's target.

_Appears in:_
- [VirtualMachinePublishRequestTarget](#virtualmachinepublishrequesttarget)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of the referenced object. 
 Please note an error will be returned if this field is not set in a namespace that lacks a default publication target. 
 A default publication target is a resource with an API version equal to spec.target.location.apiVersion, a kind equal to spec.target.location.kind, and has the label "imageregistry.vmware.com/default". |
| `apiVersion` _string_ | APIVersion is the API version of the referenced object. |
| `kind` _string_ | Kind is the kind of referenced object. |

### VirtualMachineReadinessGate



VirtualMachineReadinessGate contains the reference to a VM condition.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `conditionType` _string_ | ConditionType refers to a condition in the VM's condition list with matching type. |

### VirtualMachineReadinessProbeSpec



VirtualMachineReadinessProbeSpec describes a probe used to determine if a VM is in a ready state. All probe actions are mutually exclusive.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `tcpSocket` _[TCPSocketAction](#tcpsocketaction)_ | TCPSocket specifies an action involving a TCP port. |
| `guestHeartbeat` _[GuestHeartbeatAction](#guestheartbeataction)_ | GuestHeartbeat specifies an action involving the guest heartbeat status. |
| `guestInfo` _[GuestInfoAction](#guestinfoaction) array_ | GuestInfo specifies an action involving key/value pairs from GuestInfo. 
 The elements are evaluated with the logical AND operator, meaning all expressions must evaluate as true for the probe to succeed. 
 For example, a VM resource's probe definition could be specified as the following: 
 guestInfo: - key:   ready value: true 
 With the above configuration in place, the VM would not be considered ready until the GuestInfo key "ready" was set to the value "true". 
 From within the guest operating system it is possible to set GuestInfo key/value pairs using the program "vmware-rpctool," which is included with VM Tools. For example, the following command will set the key "guestinfo.ready" to the value "true": 
 vmware-rpctool "info-set guestinfo.ready true" 
 Once executed, the VM's readiness probe will be signaled and the VM resource will be marked as ready. |
| `timeoutSeconds` _integer_ | TimeoutSeconds specifies a number of seconds after which the probe times out. Defaults to 10 seconds. Minimum value is 1. |
| `periodSeconds` _integer_ | PeriodSeconds specifics how often (in seconds) to perform the probe. Defaults to 10 seconds. Minimum value is 1. |

### VirtualMachineReservedSpec



VirtualMachineReservedSpec describes a set of VM configuration options reserved for system use. Modification attempts by DevOps users will result in a validation error.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `resourcePolicyName` _string_ | ResourcePolicyName describes the name of a VirtualMachineSetResourcePolicy resource used to configure the VM's resource policy. |

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



VirtualMachineServicePort describes the specification of a service port to be exposed by a VirtualMachineService. This VirtualMachineServicePort specification includes attributes that define the external and internal representation of the service port.

_Appears in:_
- [VirtualMachineServiceSpec](#virtualmachineservicespec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name describes the name to be used to identify this VirtualMachineServicePort. |
| `protocol` _string_ | Protocol describes the Layer 4 transport protocol for this port. Supports "TCP", "UDP", and "SCTP". |
| `port` _integer_ | Port describes the external port that will be exposed by the service. |
| `targetPort` _integer_ | TargetPort describes the internal port open on a VirtualMachine that should be mapped to the external Port. |

### VirtualMachineServiceSpec



VirtualMachineServiceSpec defines the desired state of VirtualMachineService.

_Appears in:_
- [VirtualMachineService](#virtualmachineservice)

| Field | Description |
| --- | --- |
| `type` _VirtualMachineServiceType_ | Type specifies a desired VirtualMachineServiceType for this VirtualMachineService. Supported types are ClusterIP, LoadBalancer, ExternalName. |
| `ports` _[VirtualMachineServicePort](#virtualmachineserviceport) array_ | Ports specifies a list of VirtualMachineServicePort to expose with this VirtualMachineService. Each of these ports will be an accessible network entry point to access this service by. |
| `selector` _object (keys:string, values:string)_ | Selector specifies a map of key-value pairs, also known as a Label Selector, that is used to match this VirtualMachineService with the set of VirtualMachines that should back this VirtualMachineService. |
| `loadBalancerIP` _string_ | Only applies to VirtualMachineService Type: LoadBalancer LoadBalancer will get created with the IP specified in this field. This feature depends on whether the underlying load balancer provider supports specifying the loadBalancerIP when a load balancer is created. This field will be ignored if the provider does not support the feature. |
| `loadBalancerSourceRanges` _string array_ | LoadBalancerSourceRanges is an array of IP addresses in the format of CIDRs, for example: 103.21.244.0/22 and 10.0.0.0/24. If specified and supported by the load balancer provider, this will restrict ingress traffic to the specified client IPs. This field will be ignored if the provider does not support the feature. |
| `clusterIp` _string_ | clusterIP is the IP address of the service and is usually assigned randomly by the master. If an address is specified manually and is not in use by others, it will be allocated to the service; otherwise, creation of the service will fail. This field can not be changed through updates. Valid values are "None", empty string (""), or a valid IP address. "None" can be specified for headless services when proxying is not required. Only applies to types ClusterIP and LoadBalancer. Ignored if type is ExternalName. More info: https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies |
| `externalName` _string_ | externalName is the external reference that kubedns or equivalent will return as a CNAME record for this service. No proxying will be involved. Must be a valid RFC-1123 hostname (https://tools.ietf.org/html/rfc1123) and requires Type to be ExternalName. |

### VirtualMachineServiceStatus



VirtualMachineServiceStatus defines the observed state of VirtualMachineService.

_Appears in:_
- [VirtualMachineService](#virtualmachineservice)

| Field | Description |
| --- | --- |
| `loadBalancer` _[LoadBalancerStatus](#loadbalancerstatus)_ | LoadBalancer contains the current status of the load balancer, if one is present. |

### VirtualMachineSetResourcePolicySpec



VirtualMachineSetResourcePolicySpec defines the desired state of VirtualMachineSetResourcePolicy.

_Appears in:_
- [VirtualMachineSetResourcePolicy](#virtualmachinesetresourcepolicy)

| Field | Description |
| --- | --- |
| `resourcePool` _[ResourcePoolSpec](#resourcepoolspec)_ |  |
| `folder` _string_ |  |
| `clusterModuleGroups` _string array_ |  |

### VirtualMachineSetResourcePolicyStatus



VirtualMachineSetResourcePolicyStatus defines the observed state of VirtualMachineSetResourcePolicy.

_Appears in:_
- [VirtualMachineSetResourcePolicy](#virtualmachinesetresourcepolicy)

| Field | Description |
| --- | --- |
| `clustermodules` _[VSphereClusterModuleStatus](#vsphereclustermodulestatus) array_ |  |

### VirtualMachineSpec



VirtualMachineSpec defines the desired state of a VirtualMachine.

_Appears in:_
- [VirtualMachine](#virtualmachine)

| Field | Description |
| --- | --- |
| `imageName` _string_ | ImageName describes the name of the image resource used to deploy this VM. 
 This field may be used to specify the name of a VirtualMachineImage or ClusterVirtualMachineImage resource. The resolver first checks to see if there is a ClusterVirtualMachineImage with the specified name. If no such resource exists, the resolver then checks to see if there is a VirtualMachineImage resource with the specified name in the same Namespace as the VM being deployed. 
 This field is optional in the cases where there exists a sensible default value, such as when there is a single VirtualMachineImage resource available in the same Namespace as the VM being deployed. |
| `className` _string_ | Class describes the name of the VirtualMachineClass resource used to deploy this VM. 
 This field is optional in the cases where there exists a sensible default value, such as when there is a single VirtualMachineClass resource available in the same Namespace as the VM being deployed. |
| `storageClass` _string_ | StorageClass describes the name of a Kubernetes StorageClass resource used to configure this VM's storage-related attributes. 
 Please see https://kubernetes.io/docs/concepts/storage/storage-classes/ for more information on Kubernetes storage classes. 
 This field is optional in the cases where there exists a sensible default value, such as when there is a single StorageClass resource available in the same Namespace as the VM being deployed. |
| `bootstrap` _[VirtualMachineBootstrapSpec](#virtualmachinebootstrapspec)_ | Bootstrap describes the desired state of the guest's bootstrap configuration. 
 If omitted, then the bootstrap method is determined based on the guest identifier from the VirtualMachineImage. If the image's guest OS type is Windows, then the Sysprep bootstrap method is used; if Linux, the LinuxPrep method is used. 
 Please note that defaulting to Sysprep for Windows images only works if the image uses a volume license key, otherwise the image's product ID is required. |
| `network` _[VirtualMachineNetworkSpec](#virtualmachinenetworkspec)_ | Network describes the desired network configuration for the VM. 
 Please note this value may be omitted entirely and the VM will be assigned a single, virtual network interface that is connected to the Namespace's default network. |
| `powerState` _VirtualMachinePowerState_ | PowerState describes the desired power state of a VirtualMachine. |
| `volumes` _[VirtualMachineVolume](#virtualmachinevolume) array_ | Volumes describes a list of volumes that can be mounted to the VM. |
| `readinessProbe` _[VirtualMachineReadinessProbeSpec](#virtualmachinereadinessprobespec)_ | ReadinessProbe describes a probe used to determine the VM's ready state. |
| `readinessGates` _[VirtualMachineReadinessGate](#virtualmachinereadinessgate) array_ | ReadinessGates, if specified, will be evaluated to determine the VM's readiness. 
 A VM is ready when its readiness probe, if specified, is true AND all of the conditions specified by the readiness gates have a status equal to "True". |
| `advanced` _[VirtualMachineAdvancedSpec](#virtualmachineadvancedspec)_ | Advanced describes a set of optional, advanced VM configuration options. |
| `reserved` _[VirtualMachineReservedSpec](#virtualmachinereservedspec)_ | Reserved describes a set of VM configuration options reserved for system use. 
 Please note attempts to modify the value of this field by a DevOps user will result in a validation error. |

### VirtualMachineStatus



VirtualMachineStatus defines the observed state of a VirtualMachine instance.

_Appears in:_
- [VirtualMachine](#virtualmachine)

| Field | Description |
| --- | --- |
| `image` _LocalObjectRef_ | Image is a reference to the VirtualMachineImage resource used to deploy this VM. |
| `class` _[LocalObjectRef](#localobjectref)_ | Class is a reference to the VirtualMachineClass resource used to deploy this VM. |
| `host` _string_ | Host describes the hostname or IP address of the infrastructure host where the VM is executed. |
| `powerState` _VirtualMachinePowerState_ | PowerState describes the observed power state of the VirtualMachine. |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta) array_ | Conditions describes the observed conditions of the VirtualMachine. |
| `network` _[VirtualMachineNetworkStatus](#virtualmachinenetworkstatus)_ | Network describes the observed state of the VM's network configuration. Please note much of the network status information is only available if the guest has VM Tools installed. |
| `uniqueID` _string_ | UniqueID describes a unique identifier that is provided by the underlying infrastructure provider, such as vSphere. |
| `biosUUID` _string_ | BiosUUID describes a unique identifier provided by the underlying infrastructure provider that is exposed to the Guest OS BIOS as a unique hardware identifier. |
| `instanceUUID` _string_ | InstanceUUID describes the unique instance UUID provided by the underlying infrastructure provider, such as vSphere. |
| `volumes` _[VirtualMachineVolumeStatus](#virtualmachinevolumestatus) array_ | Volumes describes a list of current status information for each Volume that is desired to be attached to the VM. |
| `changeBlockTracking` _boolean_ | ChangeBlockTracking describes the CBT enablement status on the VM. |
| `zone` _string_ | Zone describes the availability zone where the VirtualMachine has been scheduled. 
 Please note this field may be empty when the cluster is not zone-aware. |


### VirtualMachineVolume



VirtualMachineVolume represents a named volume in a VM.

_Appears in:_
- [VirtualMachineSpec](#virtualmachinespec)

| Field | Description |
| --- | --- |
| `name` _string_ | Name represents the volume's name. Must be a DNS_LABEL and unique within the VM. |
| `persistentVolumeClaim` _[PersistentVolumeClaimVolumeSource](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#persistentvolumeclaimvolumesource-v1-core)_ | PersistentVolumeClaim represents a reference to a PersistentVolumeClaim in the same namespace. 
 More information is available at https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims. |


### VirtualMachineVolumeSource



VirtualMachineVolumeSource represents the source location of a volume to mount. Only one of its members may be specified.

_Appears in:_
- [VirtualMachineVolume](#virtualmachinevolume)

| Field | Description |
| --- | --- |
| `persistentVolumeClaim` _[PersistentVolumeClaimVolumeSource](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#persistentvolumeclaimvolumesource-v1-core)_ | PersistentVolumeClaim represents a reference to a PersistentVolumeClaim in the same namespace. 
 More information is available at https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims. |

### VirtualMachineVolumeStatus



VirtualMachineVolumeStatus defines the observed state of a VirtualMachineVolume instance.

_Appears in:_
- [VirtualMachineStatus](#virtualmachinestatus)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of the attached volume. |
| `attached` _boolean_ | Attached represents whether a volume has been successfully attached to the VirtualMachine or not. |
| `diskUUID` _string_ | DiskUUID represents the underlying virtual disk UUID and is present when attachment succeeds. |
| `error` _string_ | Error represents the last error seen when attaching or detaching a volume.  Error will be empty if attachment succeeds. |

### VirtualMachineWebConsoleRequestSpec



VirtualMachineWebConsoleRequestSpec describes the desired state for a web console request to a VM.

_Appears in:_
- [VirtualMachineWebConsoleRequest](#virtualmachinewebconsolerequest)

| Field | Description |
| --- | --- |
| `name` _string_ | Name is the name of a VM in the same Namespace as this web console request. |

### VirtualMachineWebConsoleRequestStatus



VirtualMachineWebConsoleRequestStatus describes the observed state of the request.

_Appears in:_
- [VirtualMachineWebConsoleRequest](#virtualmachinewebconsolerequest)

| Field | Description |
| --- | --- |
| `expiryTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta)_ | ExpiryTime is the time at which access via this request will expire. |
| `proxyAddr` _string_ | ProxyAddr describes the host address and optional port used to access the VM's web console. 
 The value could be a DNS entry, IPv4, or IPv6 address, followed by an optional port. For example, valid values include: 
 DNS * host.com * host.com:6443 
 IPv4 * 1.2.3.4 * 1.2.3.4:6443 
 IPv6 * 1234:1234:1234:1234:1234:1234:1234:1234 * [1234:1234:1234:1234:1234:1234:1234:1234]:6443 * 1234:1234:1234:0000:0000:0000:1234:1234 * 1234:1234:1234::::1234:1234 * [1234:1234:1234::::1234:1234]:6443 
 In other words, the field may be set to any value that is parsable by Go's https://pkg.go.dev/net#ResolveIPAddr and https://pkg.go.dev/net#ParseIP functions. |
