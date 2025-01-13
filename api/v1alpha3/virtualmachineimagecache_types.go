// // © Broadcom. All Rights Reserved.
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

type VirtualMachineImageCacheObjectRef struct {
	// Kind is a string value representing the REST resource this object
	// represents.
	// Servers may infer this from the endpoint the client submits requests to.
	// Cannot be updated.
	// In CamelCase.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Kind string `json:"kind"`

	// +optional

	// Namespace refers to a namespace.
	// This field will be empty if Kind refers to a cluster-scoped resource.
	Namespace string `json:"namespace,omitempty"`

	// Name refers to a unique resource.
	// More info: http://kubernetes.io/docs/user-guide/identifiers#names
	Name string `json:"name"`
}

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

func (s *VirtualMachineImageCacheSpec) SetLocation(dcID, dsID string) {
	for i := range s.Locations {
		l := s.Locations[i]
		if l.DatacenterID == dcID && l.DatastoreID == dsID {
			return
		}
	}
	s.Locations = append(s.Locations, VirtualMachineImageCacheLocationSpec{
		DatacenterID: dcID,
		DatastoreID:  dsID,
	})
}

type VirtualMachineImageCacheLocationStatus struct {

	// DatacenterID describes the ID of the datacenter to which the image should
	// be cached.
	DatacenterID string `json:"datacenterID"`

	// DatastoreID describes the ID of the datastore to which the image should
	// be cached.
	DatastoreID string `json:"datastoreID"`

	// +optional

	// Files describes the paths to the image's cached files on this datastore.
	Files []string `json:"files,omitempty"`

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
// +kubebuilder:storageversion
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
