// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type VirtualMachinePowerState string

// See govmomi.vim25.types.VirtualMachinePowerState
const (
	VirtualMachinePoweredOff VirtualMachinePowerState = "poweredOff"
	VirtualMachinePoweredOn  VirtualMachinePowerState = "poweredOn"
)

type VMStatusPhase string

const (
	Creating VMStatusPhase = "Creating" // BMV: Not used
	Created  VMStatusPhase = "Created"
	Deleted  VMStatusPhase = "Deleted"
)

type VirtualMachinePort struct {
	Port     int             `json:"port"`
	Ip       string          `json:"ip"`
	Name     string          `json:"name"`
	Protocol corev1.Protocol `json:"protocol"`
}

type VirtualMachineNetworkInterface struct {
	NetworkName      string `json:"networkName"`
	EthernetCardType string `json:"ethernetCardType,omitempty"`
	NetworkType      string `json:"networkType,omitempty"`
}

// VirtualMachineMetadata defines the guest customization
type VirtualMachineMetadata struct {
	ConfigMapName string `json:"configMapName,omitempty"`
	Transport     string `json:"transport,omitempty"`
}

type VirtualMachineCondition struct {
	LastProbeTime      metav1.Time `json:"lastProbeTime"`
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	Message            string      `json:"message"`
	Reason             string      `json:"reason"`
	Status             string      `json:"status"`
	Type               string      `json:"type"`
}

type VirtualMachineVolumes struct {
	// Each volume in a VM must have a unique name.
	Name string `json:"name"`

	// persistentVolumeClaim represents a reference to a PersistentVolumeClaim (pvc) in the same namespace. The pvc
	// must match a persistent volume provisioned (either statically or dynamically) by the Cloud Native Storage CSI.
	PersistentVolumeClaim *corev1.PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty"`
}

// Probe describes a health check to be performed against a VM to determine whether it is
// alive or ready to receive traffic.
type Probe struct {
	// TCPSocket specifies an action involving a TCP port.
	// +optional
	TCPSocket *TCPSocketAction `json:"tcpSocket,omitempty"`
	// Number of seconds after which the probe times out.
	// Defaults to 10 second. Minimum value is 1.
	// +optional
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
}

// TCPSocketAction describes an action based on opening a socket
type TCPSocketAction struct {
	// Number or name of the port to access on the VirtualMachine.
	// Number must be in the range 1 to 65535.
	// Name must be an IANA_SVC_NAME.
	Port intstr.IntOrString `json:"port"`
	// Optional: Host name to connect to, defaults to the VirtualMachine IP.
	// +optional
	Host string `json:"host,omitempty"`
}

// VirtualMachineSpec defines the desired state of VirtualMachine
type VirtualMachineSpec struct {
	ImageName         string                           `json:"imageName"`
	ClassName         string                           `json:"className"`
	PowerState        VirtualMachinePowerState         `json:"powerState"`
	Ports             []VirtualMachinePort             `json:"ports,omitempty"`
	VmMetadata        *VirtualMachineMetadata          `json:"vmMetadata,omitempty"`
	StorageClass      string                           `json:"storageClass,omitempty"`
	NetworkInterfaces []VirtualMachineNetworkInterface `json:"networkInterfaces,omitempty"`
	// +optional
	ResourcePolicyName string `json:"resourcePolicyName"`
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	Volumes []VirtualMachineVolumes `json:"volumes,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
	// +optional
	ReadinessProbe *Probe `json:"readinessProbe,omitempty"`
}

type VirtualMachineVolumeStatus struct {
	// The name of the volume in a VM.
	Name string `json:"name"`

	// Attached represents the state of volume attachment
	Attached bool `json:"attached"`

	// DiskUuid represents the underlying virtual disk UUID and is present when attachment succeeds
	DiskUuid string `json:"diskUUID"`

	// Error represents the last error seen when attaching or detaching a volume and will be empty if attachment succeeds
	Error string `json:"error"`
}

// VirtualMachineStatus defines the observed state of VirtualMachine
type VirtualMachineStatus struct {
	Conditions []VirtualMachineCondition    `json:"conditions,omitempty"`
	Host       string                       `json:"host"`
	PowerState VirtualMachinePowerState     `json:"powerState"`
	Phase      VMStatusPhase                `json:"phase"`
	VmIp       string                       `json:"vmIp"`
	UniqueID   string                       `json:"uniqueID"`
	BiosUUID   string                       `json:"biosUUID"`
	Volumes    []VirtualMachineVolumeStatus `json:"volumes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vm
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// VirtualMachine is the Schema for the virtualmachines API
type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSpec   `json:"spec,omitempty"`
	Status VirtualMachineStatus `json:"status,omitempty"`
}

func (vm VirtualMachine) NamespacedName() string {
	return vm.Namespace + "/" + vm.Name
}

// VirtualMachineList contains a list of VirtualMachine
//
// +kubebuilder:object:root=true
type VirtualMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualMachine{}, &VirtualMachineList{})
}
