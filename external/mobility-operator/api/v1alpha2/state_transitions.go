// Copyright (c) 2025 Broadcom. All Rights Reserved.

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=ClearPermissions;Move

// TransitionType defines the type for state transitions.
type TransitionType string

const (
	// TransitionClearPermissions indicates a transition where the virtual machine's non-inherited permissions are cleared.
	TransitionClearPermissions TransitionType = "ClearPermissions"

	// TransitionMove indicates a transition where the virtual machine is moved to a different location.
	TransitionMove TransitionType = "Move"
)

// Permission contains details for the ClearPermissions transition.
type Permission struct {
	// Group indicates whether the principal refers to a group (`true`) or a user (`false`).
	Group bool `json:"group"`

	// Principal specifies the name of the user or group whose permissions are being cleared.
	Principal string `json:"principal"`

	// Propagate indicates whether this permission propagates down the hierarchy to sub-entities.
	Propagate bool `json:"propagate"`

	// RoleID is the unique identifier of the role providing access in vSphere.
	RoleID int32 `json:"roleID"`
}

// DiskStorageInfo represents storage information for a specific virtual disk.
type DiskStorageInfo struct {
	// Name is the device key for the virtual disk.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	StorageInfo `json:",inline"`
}

// StorageInfo contains information about the storage configuration for a virtual machine.
type StorageInfo struct {
	// DatastoreID is the unique identifier of the datastore in vSphere.
	DatastoreID string `json:"datastoreID"`

	// +optional

	// ProfileIDs is a list of unique storage profile identifiers in vSphere applied to the storage.
	ProfileIDs []string `json:"profileIDs,omitempty"`
}

// DeviceBackingInfo provides information about an infrastructure device or resource that backs a device in a virtual machine.
type DeviceBackingInfo struct {
	// DeviceName is the name of the device.
	DeviceName string `json:"deviceName"`
}

// NetworkBackingInfo contains the network backing information for a virtual Ethernet card of a virtual machine.
type NetworkBackingInfo struct {
	DeviceBackingInfo `json:",inline"`

	// +optional

	// NetworkID is the unique identifier of the network in vSphere.
	NetworkID string `json:"networkID,omitempty"`
}

// DistributedVirtualSwitchPortConnection specifies the connection details for a port on a distributed virtual switch.
type DistributedVirtualSwitchPortConnection struct {
	// +optional

	// PortgroupKey is the key of the port group.
	PortgroupKey string `json:"portgroupKey,omitempty"`

	// SwitchUUID is the UUID of the distributed virtual switch.
	SwitchUUID string `json:"switchUUID"`
}

// DistributedVirtualPortBackingInfo contains the network backing information for a virtual Ethernet card that connects to a distributed virtual switch port or port group.
type DistributedVirtualPortBackingInfo struct {
	// PortConnection specifies the connection to a distributed virtual switch port.
	PortConnection DistributedVirtualSwitchPortConnection `json:"portConnection"`
}

// OpaqueNetworkBackingInfo contains the network backing information for a virtual Ethernet card that connects to an opaque network.
type OpaqueNetworkBackingInfo struct {
	// OpaqueNetworkID is the unique identifier of the opaque network in vSphere.
	OpaqueNetworkID string `json:"opaqueNetworkID"`

	// OpaqueNetworkType is the type of the opaque network. For example, "nsx.LogicalSwitch".
	OpaqueNetworkType string `json:"opaqueNetworkType"`
}

// NetworkInfo is a union type that specifies the network backing information.
// Only one of the fields should be set to represent the network backing type.
type NetworkInfo struct {
	// Name is the device key for Virtual Ethernet Card.
	Name string `json:"name"`

	// +optional

	// NetworkBacking specifies a standard network backing.
	NetworkBacking *NetworkBackingInfo `json:"networkBacking,omitempty"`

	// +optional

	// DistributedVirtualPortBacking specifies a distributed virtual port backing.
	DistributedVirtualPortBacking *DistributedVirtualPortBackingInfo `json:"distributedVirtualPortBacking,omitempty"`

	// +optional

	// OpaqueNetworkBacking specifies an opaque network backing.
	OpaqueNetworkBacking *OpaqueNetworkBackingInfo `json:"opaqueNetworkBacking,omitempty"`
}

// Location specifies the details of a virtual machine's location within the infrastructure.
type Location struct {
	// +optional

	// DatacenterID specifies the unique identifier of the datacenter in vSphere where the virtual machine is located.
	DatacenterID string `json:"datacenterID,omitempty"`

	// +optional

	// HostID specifies the unique identifier of the host in vSphere where the virtual machine is located.
	HostID string `json:"hostID,omitempty"`

	// +optional

	// FolderID specifies the unique identifier of the folder in vSphere where the virtual machine is located.
	FolderID string `json:"folderID,omitempty"`

	// +optional

	// ResourcePoolID specifies the unique identifier of the resource pool in vSphere where the virtual machine is assigned.
	ResourcePoolID string `json:"resourcePoolID,omitempty"`

	// +optional

	// HomeStorage contains storage information for the virtual machine's home files.
	HomeStorage *StorageInfo `json:"homeStorage,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name

	// DiskStorage contains storage information for the virtual machine's disks.
	DiskStorage []DiskStorageInfo `json:"diskStorage,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name

	// Network specifies the network configuration for the virtual machine's virtual Ethernet cards.
	Network []NetworkInfo `json:"network,omitempty"`

	// +optional

	// PowerState contains the power state of the virtual machine.

	PowerState string `json:"powerState,omitempty"`
}

// MoveDetails contains details about the virtual machine move transition, including the original and new locations.
type MoveDetails struct {
	// +optional

	// Source specifies the original location of the virtual machine before the move.
	Source *Location `json:"source,omitempty"`

	// +optional

	// Destination specifies the new location of the virtual machine after the move.
	Destination *Location `json:"destination,omitempty"`
}

// TransitionDetails contains detailed information specific to the type of transition.
// Only one of the fields should be set, corresponding to the `TransitionType`.
type TransitionDetails struct {
	// +optional

	// ClearedPermissions contains the virtual machine's non-inherited permissions that were cleared during the import.
	ClearedPermissions []Permission `json:"clearedPermissions,omitempty"`

	// +optional

	// Move contains details if the transition is of type `Move`.
	Move *MoveDetails `json:"move,omitempty"`
}

// StateTransition records a specific state change or action performed during the virtual machine import process.
type StateTransition struct {
	// Type specifies the type of state transition.
	Type TransitionType `json:"type"`

	// Timestamp is the time when the transition was recorded.
	// This field is always set.
	Timestamp metav1.Time `json:"timestamp"`

	// Details holds additional information about the transition.
	Details *TransitionDetails `json:"details,omitempty"`
}
