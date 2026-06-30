// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ZoneResourceUtilizationReadyCondition indicates that last resource
// utilization data update was successful (and the time of that attempt).
const ZoneResourceUtilizationReadyCondition = "Ready"

// ZoneResourceUtilizationSpec contains identifying information about the
// vSphere resources used to represent a Kubernetes namespace on individual
// vSphere Zones.
type ZoneResourceUtilizationSpec struct {
	// PoolMoIDs are the managed object ID of the vSphere ResourcePools
	// in an individual vSphere Zone. A zone may be comprised of
	// multiple ResourcePools.
	PoolMoIDs []string `json:"poolMoIDs,omitempty"`
}

// ResourcePoolUsage specifies the resource usage for CPU or memory.
// In the typical case, where a resource pool is in a consistent state,
// unreservedForVM will be equal to unreservedForPool. Hence, we
// can simply talk about unreserved resources.
// If the reservation on the resource pool is not expandable, then
// the following is true:
// reservation = reservationUsed + unreserved
// If the reservation on the resource pool is expandable, then
// the following is true:
// reservation + parent.unreserved = reservationUsed + unreserved
type ResourcePoolUsage struct {
	// +optional
	// ReservationUsed is the total amount of resources that has been used to
	// satisfy the reservation requirements of all descendants of this
	// resource pool (includes both resource pools and virtual machines).
	ReservationUsed *resource.Quantity `json:"reservationUsed,omitempty"`

	// +optional
	// ReservationUsedForVM is the total amount of resources that has been
	// used to satisfy the reservation requirements of running virtual
	// machines in this resource pool or any of its child resource pools.
	ReservationUsedForVM *resource.Quantity `json:"reservationUsedForVM,omitempty"`

	// +optional
	// UnreservedForPool is the total amount of resources available to
	// satisfy a reservation for a child resource pool.
	//
	// In the undercommitted state, this is limited by the capacity at the
	// root node. In the overcommitted case, this could be higher since we do
	// not perform the dynamic capacity checks.
	UnreservedForPool *resource.Quantity `json:"unreservedForPool,omitempty"`

	// +optional
	// UnreservedForVM is the total amount of resource available to satisfy a
	// reservation for a child virtual machine.
	//
	// In general, this should be the same as `unreservedForPool`. However,
	// in the overcommitted case, this is limited by the remaining available
	// resources at the root node.
	UnreservedForVM *resource.Quantity `json:"unreservedForVM,omitempty"`

	// +optional
	// MaxUsage is the current upper-bound on resource usage.
	//
	// The upper-bound is based on the limit configured on this resource
	// pool, as well as limits configured on any parent resource pool.
	MaxUsage *resource.Quantity `json:"maxUsage,omitempty"`
}

// ResourcePoolQuickStats is a set of statistics that are typically updated
// with near real-time regularity. These statistics are aggregates of the
// corresponding statistics of all virtual machines in the given resource
// pool, and unless otherwise noted, only make sense when at least one
// virtual machine in the given resource pool is powered on.
type ResourcePoolQuickStats struct {
	// +optional
	// OverallCPUUsage is the basic CPU performance statistics, in Hz.
	OverallCPUUsage *resource.Quantity `json:"overallCPUUsage,omitempty"`

	// +optional
	// OverallCPUDemand is the basic CPU performance statistics, in Hz.
	OverallCPUDemand *resource.Quantity `json:"overallCPUDemand,omitempty"`

	// +optional
	// GuestMemoryUsage is the guest memory utilization statistics.
	//
	// This is also known as active guest memory. The number can be between 0
	// and the configured memory size of all virtual machines in resource
	// pool.
	GuestMemoryUsage *resource.Quantity `json:"guestMemoryUsage,omitempty"`

	// +optional
	// HostMemoryUsage is the host memory utilization statistics.
	//
	// This is also known as consumed host memory. This is between 0 and the
	// configured resource limit. Valid while any virtual machine is running.
	// This includes the overhead memory of all virtual machines.
	HostMemoryUsage *resource.Quantity `json:"hostMemoryUsage,omitempty"`

	// +optional
	// DistributedCPUEntitlement is the amount of CPU resource, that this
	// resource pool is entitled to, as calculated by DRS.
	DistributedCPUEntitlement *resource.Quantity `json:"distributedCPUEntitlement,omitempty"`

	// +optional
	// DistributedMemoryEntitlement is the amount of memory, that this
	// resource pool is entitled to, as calculated by DRS.
	DistributedMemoryEntitlement *resource.Quantity `json:"distributedMemoryEntitlement,omitempty"`

	// +optional
	// StaticCPUEntitlement is the static CPU resource entitlement for a
	// resource pool, in Hz.
	//
	// This value is calculated based on resource reservations, shares and
	// limit, and doesn't take into account current usage. This is the worst
	// case CPU allocation, that is, the amount of CPU resource this resource
	// pool would receive if all virtual machines running in the cluster went
	// to maximum consumption.
	StaticCPUEntitlement *resource.Quantity `json:"staticCpuEntitlement,omitempty"`

	// +optional
	// StaticMemoryEntitlement is the static memory resource entitlement for a
	// virtual machine.
	//
	// This value is calculated based on resource reservations, shares and
	// limit, and doesn't take into account current usage. This is the worst
	// case memory allocation, that is, the amount of memory this resource
	// pool would receive if all virtual machines running in the cluster went
	// to maximum consumption.
	StaticMemoryEntitlement *resource.Quantity `json:"staticMemoryEntitlement,omitempty"`

	// +optional
	// PrivateMemory is the portion of memory, that is granted from
	// non-shared host memory.
	PrivateMemory *resource.Quantity `json:"privateMemory,omitempty"`

	// +optional
	// SharedMemory is the portion of memory, that is granted from host
	// memory that is shared between VMs.
	SharedMemory *resource.Quantity `json:"sharedMemory,omitempty"`

	// +optional
	// SwappedMemory is the portion of memory, that is granted to a from the
	// host's swap space.
	//
	// This is a sign that there is memory pressure on the host.
	SwappedMemory *resource.Quantity `json:"swappedMemory,omitempty"`

	// +optional
	// BalloonedMemory is the size of the balloon driver.
	//
	// The host will inflate the balloon driver to reclaim physical memory
	// from a virtual machine. This is a sign that there is memory pressure on
	// the host.
	BalloonedMemory *resource.Quantity `json:"balloonedMemory,omitempty"`

	// +optional
	// OverheadMemory is the amount of memory, that will be used by virtual
	// machines above their guest memory requirements.
	//
	// This value is set if and only if a virtual machine is registered on a
	// host that supports memory resource allocation features. For powered
	// off VMs, this is the minimum overhead required to power on the VM on
	// the registered host. For powered on VMs, this is the current overhead
	// reservation, a value which is almost always larger than the minimum
	// overhead, and which grows with time.
	OverheadMemory *resource.Quantity `json:"overheadMemory,omitempty"`

	// +optional
	// ConsumedOverheadMemory is the amount of overhead memory, currently
	// being consumed to run VMs.
	ConsumedOverheadMemory *resource.Quantity `json:"consumedOverheadMemory,omitempty"`

	// +optional
	// CompressedMemory is the amount of compressed memory currently consumed
	// by VMs.
	CompressedMemory *resource.Quantity `json:"compressedMemory,omitempty"`
}

// ResourcePoolInfo contains the utilization data for a single vSphere
// ResourcePool.
type ResourcePoolInfo struct {
	// +required
	// PoolMoID is the managed object ID of the vSphere ResourcePool.
	PoolMoID string `json:"poolMoID"`

	// +required
	// LastUpdate is the last time data from VC was received.
	LastUpdate metav1.Time `json:"lastUpdate"`

	// +optional
	// QuickStats is the quick statistics of the vSphere ResourcePool.
	QuickStats ResourcePoolQuickStats `json:"quickStats,omitempty"`

	// +optional
	// CPU is the CPU usage of the vSphere ResourcePool.
	CPU ResourcePoolUsage `json:"cpu,omitempty"`

	// +optional
	// Memory is the memory usage of the vSphere ResourcePool.
	Memory ResourcePoolUsage `json:"memory,omitempty"`
}

// ZoneResourceUtilizationStatus defines the observed state of
// ZoneResourceUtilization.
type ZoneResourceUtilizationStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	// Conditions describes the observed conditions of the Zone
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=poolMoID
	// Resources is the map storing utilization information for all the
	// resource pools listed in the spec.
	Resources []ResourcePoolInfo `json:"resources,omitempty"`
}

// ZoneResourceUtilization is the schema for the ZoneResourceUtilization
// resource for the vSphere topology API.
//
// A Zone is the zone the k8s namespace is confined to. That is workloads will
// be limited to the Zones in the namespace.  For more information about
// availability zones, refer to:
// https://kubernetes.io/docs/setup/best-practices/multiple-zones/
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=zoneresourceutilization,scope=Namespaced,shortName=zru
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Resource Pool ID",type=string,JSONPath=`.spec.poolMoIDs`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=='Ready')].status`
type ZoneResourceUtilization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ZoneResourceUtilizationSpec `json:"spec"`

	// +optional
	// Status is the observed state of the ZoneResourceUtilization.
	Status ZoneResourceUtilizationStatus `json:"status,omitempty"`
}

// ZoneResourceUtilizationList contains a list of ZoneResourceUtilization
// resources.
//
// +kubebuilder:object:root=true
type ZoneResourceUtilizationList struct {
	metav1.TypeMeta `json:"inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZoneResourceUtilization `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &ZoneResourceUtilization{}, &ZoneResourceUtilizationList{})
}
