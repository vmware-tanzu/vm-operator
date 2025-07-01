// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	vmopv1cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha4/cloudinit"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha4/sysprep"
)

// VirtualMachineBootstrapSpec defines the desired state of a VM's bootstrap
// configuration.
type VirtualMachineBootstrapSpec struct {

	// +optional

	// CloudInit may be used to bootstrap Linux guests with Cloud-Init or
	// Windows guests that support Cloudbase-Init.
	//
	// The guest's networking stack is configured by Cloud-Init on Linux guests
	// and Cloudbase-Init on Windows guests.
	//
	// Please note this bootstrap provider may not be used in conjunction with
	// the other bootstrap providers.
	CloudInit *VirtualMachineBootstrapCloudInitSpec `json:"cloudInit,omitempty"`

	// +optional

	// LinuxPrep may be used to bootstrap Linux guests.
	//
	// The guest's networking stack is configured by Guest OS Customization
	// (GOSC).
	//
	// Please note this bootstrap provider may be used in conjunction with the
	// VAppConfig bootstrap provider when wanting to configure the guest's
	// network with GOSC but also send vApp/OVF properties into the guest.
	//
	// This bootstrap provider may not be used in conjunction with the CloudInit
	// or Sysprep bootstrap providers.
	LinuxPrep *VirtualMachineBootstrapLinuxPrepSpec `json:"linuxPrep,omitempty"`

	// +optional

	// Sysprep may be used to bootstrap Windows guests.
	//
	// The guest's networking stack is configured by Guest OS Customization
	// (GOSC).
	//
	// Please note this bootstrap provider may be used in conjunction with the
	// VAppConfig bootstrap provider when wanting to configure the guest's
	// network with GOSC but also send vApp/OVF properties into the guest.
	//
	// This bootstrap provider may not be used in conjunction with the CloudInit
	// or LinuxPrep bootstrap providers.
	Sysprep *VirtualMachineBootstrapSysprepSpec `json:"sysprep,omitempty"`

	// +optional

	// VAppConfig may be used to bootstrap guests that rely on vApp properties
	// (how VMware surfaces OVF properties on guests) to transport data into the
	// guest.
	//
	// The guest's networking stack may be configured using either vApp
	// properties or GOSC.
	//
	// Many OVFs define one or more properties that are used by the guest to
	// bootstrap its networking stack. If the VirtualMachineImage defines one or
	// more properties like this, then they can be configured to use the network
	// data provided for this VM at runtime by setting these properties to Go
	// template strings.
	//
	// It is also possible to use GOSC to bootstrap this VM's network stack by
	// configuring either the LinuxPrep or Sysprep bootstrap providers.
	//
	// Please note the VAppConfig bootstrap provider in conjunction with the
	// LinuxPrep bootstrap provider is the equivalent of setting the v1alpha1
	// VM metadata transport to "OvfEnv".
	//
	// This bootstrap provider may not be used in conjunction with the CloudInit
	// bootstrap provider.
	VAppConfig *VirtualMachineBootstrapVAppConfigSpec `json:"vAppConfig,omitempty"`
}

// VirtualMachineBootstrapCloudInitSpec describes the CloudInit configuration
// used to bootstrap the VM.
type VirtualMachineBootstrapCloudInitSpec struct {
	// +optional

	// InstanceID is the cloud-init metadata instance ID.
	// If omitted, this field defaults to the VM's BiosUUID.
	InstanceID string `json:"instanceID,omitempty"`

	// +optional

	// CloudConfig describes a subset of a Cloud-Init CloudConfig, used to
	// bootstrap the VM.
	//
	// Please note this field and RawCloudConfig are mutually exclusive.
	CloudConfig *vmopv1cloudinit.CloudConfig `json:"cloudConfig,omitempty"`

	// +optional

	// RawCloudConfig describes a key in a Secret resource that contains the
	// CloudConfig data used to bootstrap the VM.
	//
	// The CloudConfig data specified by the key may be plain-text,
	// base64-encoded, or gzipped and base64-encoded.
	//
	// Please note this field and CloudConfig are mutually exclusive.
	RawCloudConfig *vmopv1common.SecretKeySelector `json:"rawCloudConfig,omitempty"`

	// +optional

	// SSHAuthorizedKeys is a list of public keys that CloudInit will apply to
	// the guest's default user.
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty"`

	// +optional
	// +kubebuilder:default:true

	// UseGlobalNameserversAsDefault will use the global nameservers specified in
	// the NetworkSpec as the per-interface nameservers when the per-interface
	// nameservers is not provided.
	//
	// Defaults to true if omitted.
	UseGlobalNameserversAsDefault *bool `json:"useGlobalNameserversAsDefault,omitempty"`

	// +optional
	// +kubebuilder:default:true

	// UseGlobalSearchDomainsAsDefault will use the global search domains specified
	// in the NetworkSpec as the per-interface search domains when the per-interface
	// search domains is not provided.
	//
	// Defaults to true if omitted.
	UseGlobalSearchDomainsAsDefault *bool `json:"useGlobalSearchDomainsAsDefault,omitempty"`

	// +optional

	// WaitOnNetwork4 indicates whether the cloud-init datasource should wait
	// for an IPv4 address to be available before writing the instance-data.
	//
	// When set to true, the cloud-init datasource will sleep for a second,
	// check network status, and repeat until an IPv4 address is available.
	WaitOnNetwork4 *bool `json:"waitOnNetwork4,omitempty"`

	// +optional

	// WaitOnNetwork6 indicates whether the cloud-init datasource should wait
	// for an IPv6 address to be available before writing the instance-data.
	//
	// When set to true, the cloud-init datasource will sleep for a second,
	// check network status, and repeat until an IPv6 address is available.
	WaitOnNetwork6 *bool `json:"waitOnNetwork6,omitempty"`
}

// VirtualMachineBootstrapLinuxPrepSpec describes the LinuxPrep configuration
// used to bootstrap the VM.
type VirtualMachineBootstrapLinuxPrepSpec struct {
	// +optional

	// HardwareClockIsUTC specifies whether the hardware clock is in UTC or
	// local time.
	HardwareClockIsUTC *bool `json:"hardwareClockIsUTC,omitempty"`

	// +optional

	// TimeZone is a case-sensitive timezone, such as Europe/Sofia.
	//
	// Valid values are based on the tz (timezone) database used by Linux and
	// other Unix systems. The values are strings in the form of
	// "Area/Location," in which Area is a continent or ocean name, and
	// Location is the city, island, or other regional designation.
	//
	// Please see https://kb.vmware.com/s/article/2145518 for a list of valid
	// time zones for Linux systems.
	TimeZone string `json:"timeZone,omitempty"`
}

// VirtualMachineBootstrapSysprepSpec describes the Sysprep configuration used
// to bootstrap the VM.
type VirtualMachineBootstrapSysprepSpec struct {
	// +optional

	// Sysprep is an object representation of a Windows sysprep.xml answer file.
	//
	// This field encloses all the individual keys listed in a sysprep.xml file.
	//
	// For more detailed information please see
	// https://technet.microsoft.com/en-us/library/cc771830(v=ws.10).aspx.
	//
	// Please note this field and RawSysprep are mutually exclusive.
	Sysprep *vmopv1sysprep.Sysprep `json:"sysprep,omitempty"`

	// +optional

	// RawSysprep describes a key in a Secret resource that contains an XML
	// string of the Sysprep text used to bootstrap the VM.
	//
	// The data specified by the Secret key may be plain-text, base64-encoded,
	// or gzipped and base64-encoded.
	//
	// Please note this field and Sysprep are mutually exclusive.
	RawSysprep *vmopv1common.SecretKeySelector `json:"rawSysprep,omitempty"`
}

// VirtualMachineBootstrapVAppConfigSpec describes the vApp configuration
// used to bootstrap the VM.
type VirtualMachineBootstrapVAppConfigSpec struct {
	// +optional
	// +listType=map
	// +listMapKey=key

	// Properties is a list of vApp/OVF property key/value pairs.
	//
	// Please note this field and RawProperties are mutually exclusive.
	Properties []vmopv1common.KeyValueOrSecretKeySelectorPair `json:"properties,omitempty"`

	// +optional

	// RawProperties is the name of a Secret resource in the same Namespace as
	// this VM where each key/value pair from the Secret is used as a vApp
	// key/value pair.
	//
	// Please note this field and Properties are mutually exclusive.
	RawProperties string `json:"rawProperties,omitempty"`
}
