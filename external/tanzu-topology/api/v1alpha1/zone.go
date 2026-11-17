// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ZoneConditionPersistentVolumeClaimsExist indicates that PVCs exist in the
	// Zone.
	ZoneConditionPersistentVolumeClaimsExist = "ZonePersistentVolumeClaimsExist"

	// ZoneConditionPodsExist indicates that Pods exist in the Zone.
	ZoneConditionPodsExist = "ZonePodsExist"

	// ZoneConditionVirtualMachinesExist indicates that VirtualMachines exist
	// in the Zone.
	ZoneConditionVirtualMachinesExist = "ZoneVirtualMachinesExist"

	// ZoneConditionNamespaceResourcePoolReconciled indicates that the
	// Namespace ResourcePool has been reconciled.
	ZoneConditionNamespaceResourcePoolReconciled = "ZoneNamespaceResourcePoolReconciled"

	// ZoneConditionCPULimitRealized indicates that desired CPU limit was realized.
	ZoneConditionCPULimitRealized = "ZoneConditionCPULimitRealized"

	// ZoneConditionCPUReservationRealized indicates that desired CPU reservation was realized.
	ZoneConditionCPUReservationRealized = "ZoneConditionCPUReservationRealized"

	// ZoneConditionMemoryLimitRealized indicates that desired memory limit was realized.
	ZoneConditionMemoryLimitRealized = "ZoneConditionMemoryLimitRealized"

	// ZoneConditionCPUReservationRealized indicates that desired memory reservation was realized.
	ZoneConditionMemoryReservationRealized = "ZoneConditionMemoryReservationRealized"

	// ZoneConditionVMReservationsRealized indicates that desired VM classes have been reserved.
	ZoneConditionVMReservationsRealized = "ZoneConditionVMReservationsRealized"

	// ZoneConditionClusterDecommissionOpDone indicates that a cluster decommission operation was completed.
	ZoneConditionClusterDecommissionOpDone = "ZoneConditionClusterDecommissionOpDone"

	// ZoneConditionClusterDecommissionOpCanceled indicates that a cluster decommission operation was canceled.
	ZoneConditionClusterDecommissionOpCanceled = "ZoneConditionClusterDecommissionOpCanceled"
)

// +kubebuilder:validation:Enum=System;Namespace

// OverheadProviderType defines which component's quota should be used for
// the needed overhead memory.
type OverheadProviderType string

const (
	OverheadProviderTypeSystem    OverheadProviderType = "System"
	OverheadProviderTypeNamespace OverheadProviderType = "Namespace"
)

// AvailabilityZoneReference describes a reference to the cluster scoped
// AvailabilityZone object.
type AvailabilityZoneReference struct {
	// APIVersion defines the versioned schema of this reference to the cluster scoped
	// AvailabilityZone object.
	APIVersion string `json:"apiVersion"`

	// Name is the name of  the cluster scoped AvailabilityZone to refer to.
	Name string `json:"name"`
}

// VSphereEntityInfo contains the managed object IDs associated with
// a vSphere entity
type VSphereEntityInfo struct {
	// +optional
	// PoolMoIDs are the managed object ID of the vSphere ResourcePools
	// in an individual vSphere Zone. A zone may be comprised of
	// multiple ResourcePools.
	PoolMoIDs []string `json:"poolMoIDs,omitempty"`

	// +optional
	// FolderMoID is the managed object ID of the vSphere Folder for a
	// Namespace.
	FolderMoID string `json:"folderMoID,omitempty"`

	// +optional
	// ClusterMoIDs are the managed object IDs of the vSphere Clusters in an
	// individual vSphere Zone. A zone may be comprised of multiple Clusters.
	ClusterMoIDs []string `json:"clusterMoIDs,omitempty"`
}

// VirtualMachineClassAllocationInfo describes the definition of allocations
// for Virtual Machines of a given class.
type VirtualMachineClassAllocationInfo struct {
	// ReservedVMClass is the identifier of the Virtual Machine class used for allocation.
	ReservedVMClass string `json:"reservedVmClass"`

	// Count is the number of instances of given Virtual Machine class.
	Count int64 `json:"count"`
}

// ResourceAllocationInfo describes a CPU or memory limit and reservation.
type ResourceAllocationInfo struct {
	// +optional
	// Limit is the utilization of zone will not exceed this limit, even
	// if there are available resources.
	// If unset, then there is no fixed limit on resource usage.
	// Units are hertz for CPU and bytes for memory.
	Limit *resource.Quantity `json:"limit,omitempty"`

	// +optional
	// Reservation is the amount of resource that is guaranteed available to the zone.
	// Units are hertz for CPU and bytes for memory.
	Reservation *resource.Quantity `json:"reservation,omitempty"`
}

// ZoneSpec contains identifying information about the
// vSphere resources used to represent a Kubernetes namespace on individual
// vSphere Zones.
type ZoneSpec struct {
	// Namespace contains ResourcePool and folder moIDs to represent the namespace
	Namespace VSphereEntityInfo `json:"namespace,omitempty"`

	// VSpherePods contains ResourcePool and folder moIDs to represent vSpherePods
	// entity within the namespace
	VSpherePods VSphereEntityInfo `json:"vSpherePods,omitempty"`

	// ManagedVMs contains ResourcePool and folder moIDs to represent managedVMs
	// entity within the namespace
	ManagedVMs VSphereEntityInfo `json:"managedVMs,omitempty"`

	// Zone is a reference to the cluster scoped AvailabilityZone this
	// Zone is derived from.
	Zone AvailabilityZoneReference `json:"availabilityZoneReference"`

	// +optional
	// VirtualMachineReservations is the desired number of reserved Virtual Machine
	// class instances that are available for the namespace in this zone.
	VirtualMachineReservations []VirtualMachineClassAllocationInfo `json:"virtualMachineReservations,omitempty"`

	// +optional
	// CPU is the desired CPU limit and reservation (in hertz) for the namespace in
	// this zone, in addition to the limits specified as part of reserved virtual
	// machine classes.
	CPU *ResourceAllocationInfo `json:"cpu,omitempty"`

	// +optional
	// Memory is the desired Memory limit and reservation (in bytes) for the
	// namespace in this zone, in addition to the limits specified as part of
	// reserved virtual machine classes.
	Memory *ResourceAllocationInfo `json:"memory,omitempty"`

	// +optional
	// OverheadMemoryProvidedBy determines the provider of overhead memory for the
	// namespace in this zone. The default behaviour depends on whether or not
	// memory reservations are set on the namespace on this zone. If no memory is
	// reserved, then the overhead memory for the workload will be consumed from
	// the system. If memory is reserved on the namespace on the zone, then the
	// overhead memory for the workload will be consumed from that namespace. If
	// OverheadMemoryProvidedBy is set to System then overhead memory will not be
	// counted against namespace reservation. When it's set to Namespace then
	// overhead memory will be consuming memory reservations in this namespace.
	OverheadMemoryProvidedBy *OverheadProviderType `json:"overheadMemoryProvidedBy,omitempty"`

	// +optional
	// DisallowUnreservedDirectPathUsage determines whether workloads that don't
	// use a reserved Virtual Machine class instance can use a DirectPath device.
	DisallowUnreservedDirectPathUsage *bool `json:"disallowUnreservedDirectPathUsage,omitempty"`

	// +optional
	// AllowedClusterComputeResourceMoIDs are the managed object IDs of the vSphere
	// ClusterComputeResources in this vSphere Zone on which workloads in this
	// Supervisor Namespace can be placed on. If empty, all the vSphere Clusters in
	// the vSphere Zone are candidates to place the workloads in this vSphere
	// Namespace.
	AllowedClusterComputeResourceMoIDs []string `json:"allowedClusterComputeResourceMoIDs,omitempty"`
}

// ZoneStatus defines the observed state of Zone.
type ZoneStatus struct {
	// +optional
	// Conditions describes the observed conditions of the Zone
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	// MarkedForRemoval describes if the Zone is marked for removal from the
	// Namespace.
	MarkedForRemoval bool `json:"markedForRemoval,omitempty"`

	// +optional
	// VirtualMachineReservations is the guaranteed number of reserved Virtual
	// Machine class instances that are available for the namespace in this zone.
	VirtualMachineReservations []VirtualMachineClassAllocationInfo `json:"virtualMachineReservations,omitempty"`

	// +optional
	// CPU is the realized CPU limit and reservation (in hertz) for the namespace
	// in this zone, in addition to the limits specified as part of reserved
	// virtual machine classes.
	CPU *ResourceAllocationInfo `json:"cpu,omitempty"`

	// +optional
	// Memory is the realized memory limit and reservation (in bytes) for the
	// namespace in this zone, in addition to the limits specified as part of
	// reserved virtual machine classes.
	Memory *ResourceAllocationInfo `json:"memory,omitempty"`

	// +optional
	// DisallowUnreservedDirectPathUsage is the configuration set indicating if
	// workloads that don't use a reserved Virtual Machine class instance can use
	// a DirectPath device.
	DisallowUnreservedDirectPathUsage *bool `json:"disallowUnreservedDirectPathUsage,omitempty"`
}

// Zone is the schema for the Zone resource for the vSphere topology API.
//
// A Zone is the zone the k8s namespace is confined to. That is workloads will
// be limited to the Zones in the namespace.  For more information about
// availability zones, refer to:
// https://kubernetes.io/docs/setup/best-practices/multiple-zones/
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=zones,scope=Namespaced
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
type Zone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZoneSpec   `json:"spec,omitempty"`
	Status ZoneStatus `json:"status,omitempty"`
}

// ZoneList contains a list of Zone resources.
//
// +kubebuilder:object:root=true
type ZoneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Zone `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &Zone{}, &ZoneList{})
}
