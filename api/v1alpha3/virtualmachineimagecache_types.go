// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// VirtualMachineImageCacheConditionProviderReady indicates the underlying
	// provider is fully synced, including its disks being available locally on
	// some datastore.
	VirtualMachineImageCacheConditionProviderReady = "VirtualMachineImageCacheProviderReady"

	// VirtualMachineImageCacheConditionDisksReady indicates the disks are
	// cached in a given location.
	VirtualMachineImageCacheConditionDisksReady = "VirtualMachineImageCacheDisksReady"

	// VirtualMachineImageCacheConditionOVFReady indicates the OVF is cached.
	VirtualMachineImageCacheConditionOVFReady = "VirtualMachineImageCacheOVFReady"
)

type VirtualMachineImageCacheLocationSpec struct {
	// DatacenterID describes the ID of the datacenter to which the image should
	// be cached.
	DatacenterID string `json:"datacenterID"`

	// DatastoreID describes the ID of the datastore to which the image should
	// be cached.
	DatastoreID string `json:"datastoreID"`
}

// VirtualMachineImageCacheSpec defines the desired state of
// VirtualMachineImageCache.
type VirtualMachineImageCacheSpec struct {
	// ProviderID describes the ID of the provider item to which the image
	// corresponds.
	// If the provider is Content Library, the ID refers to a Content Library
	// item.
	ProviderID string `json:"providerID"`

	// ProviderVersion describes the version of the provider item to which the
	// image corresponds.
	// The provider is Content Library, the version is the content version.
	ProviderVersion string `json:"providerVersion"`

	// +optional
	// +listType=map
	// +listMapKey=datacenterID
	// +listMapKey=datastoreID

	// Locations describes the locations where the image should be cached.
	Locations []VirtualMachineImageCacheLocationSpec `json:"locations,omitempty"`
}

// AddLocation adds the provided datacenterID and datastoreID to the the image
// cache object's spec.locations list if such a location does not already exist.
// Calling this function for a pair of datacenterID and datastoreID values that
// already exist in the object's spec.locations list has no effect.
func (i *VirtualMachineImageCache) AddLocation(
	datacenterID,
	datastoreID string) {

	for idx := range i.Spec.Locations {
		l := i.Spec.Locations[idx]
		if l.DatacenterID == datacenterID && l.DatastoreID == datastoreID {
			return
		}
	}
	i.Spec.Locations = append(
		i.Spec.Locations,
		VirtualMachineImageCacheLocationSpec{
			DatacenterID: datacenterID,
			DatastoreID:  datastoreID,
		})
}

type VirtualMachineImageCacheDiskStatus struct {

	// ID describes the value used to locate the disk.
	// The value of this field depends on the type of disk.
	// For Type=Classic, the ID value describes a datastore path, ex.
	// "[my-datastore-1] .contentlib-cache/1234/5678/my-disk-1.vmdk".
	// For Type=Managed, the ID value describes a First Class Disk (FCD).
	ID string `json:"id"`

	// Type describes the type of disk.
	Type VirtualMachineVolumeType `json:"type"`

	// TODO(akutz) In the future there may be additional information about the
	//             disk, such as its sector format (512 vs 4k), is encrypted,
	//             thin-provisioned, adapter type, etc.
}

type VirtualMachineImageCacheLocationStatus struct {

	// DatacenterID describes the ID of the datacenter to which the image should
	// be cached.
	DatacenterID string `json:"datacenterID"`

	// DatastoreID describes the ID of the datastore to which the image should
	// be cached.
	DatastoreID string `json:"datastoreID"`

	// +optional
	// +listType=map
	// +listMapKey=id
	// +listMapKey=type

	// Disks describes the image's disks cached on this datastore.
	Disks []VirtualMachineImageCacheDiskStatus `json:"disks,omitempty"`

	// +optional

	// Conditions describes any conditions associated with this cache location.
	//
	// Generally this should just include the ReadyType condition.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (i VirtualMachineImageCacheLocationStatus) GetConditions() []metav1.Condition {
	return i.Conditions
}

func (i *VirtualMachineImageCacheLocationStatus) SetConditions(conditions []metav1.Condition) {
	i.Conditions = conditions
}

type VirtualMachineImageCacheOVFStatus struct {

	// +optional

	// ConfigMapName describes the name of the ConfigMap resource that contains
	// the image's OVF envelope encoded as YAML. The data is located in the
	// ConfigMap key "value".
	ConfigMapName string `json:"configMapName,omitempty"`

	// +optional

	// ProviderVersion describes the observed provider version at which the OVF
	// is cached.
	// The provider is Content Library, the version is the content version.
	ProviderVersion string `json:"providerVersion,omitempty"`
}

// VirtualMachineImageCacheStatus defines the observed state of
// VirtualMachineImageCache.
type VirtualMachineImageCacheStatus struct {

	// +optional
	// +listType=map
	// +listMapKey=datacenterID
	// +listMapKey=datastoreID

	// Locations describe the observed locations where the image is cached.
	Locations []VirtualMachineImageCacheLocationStatus `json:"locations,omitempty"`

	// +optional

	// OVF describes the observed status of the cached OVF content.
	OVF *VirtualMachineImageCacheOVFStatus `json:"ovf,omitempty"`

	// +optional

	// Conditions describes any conditions associated with this cached image.
	//
	// Generally this should just include the ReadyType condition, which will
	// only be True if all of the cached locations also have True ReadyType
	// condition.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (i VirtualMachineImageCache) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *VirtualMachineImageCache) SetConditions(conditions []metav1.Condition) {
	i.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vmic;vmicache;vmimagecache
// +kubebuilder:subresource:status

// VirtualMachineImageCache is the schema for the
// virtualmachineimagecaches API.
type VirtualMachineImageCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineImageCacheSpec   `json:"spec,omitempty"`
	Status VirtualMachineImageCacheStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualMachineImageCacheList contains a list of VirtualMachineImageCache.
type VirtualMachineImageCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineImageCache `json:"items"`
}

func init() {
	objectTypes = append(objectTypes,
		&VirtualMachineImageCache{},
		&VirtualMachineImageCacheList{})
}
