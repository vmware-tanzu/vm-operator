// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StoragePolicySpec struct {
	// +required

	// ID is the storage policy ID.
	ID string `json:"id"`
}

// +kubebuilder:validation:Enum=VMFS;VSAN

// DatastoreType represents the type of datastore.
type DatastoreType string

const (
	DatastoreTypeVMFS DatastoreType = "VMFS"
	DatastoreTypeVSAN DatastoreType = "VSAN"
)

type Datastore struct {
	// +required

	// ID is the datastore's managed object ID.
	ID ManagedObjectID `json:"id"`

	// +optional

	// Type is the observed type of datastore.
	Type DatastoreType `json:"type,omitempty"`
}

//
// Please note, it is not possible to use the kubebuilder Enum validation for
// the valid values as they begin with digits.
//

// DiskFormat represents a disk format supported by a storage policy.
type DiskFormat string

const (
	DiskFormat512n DiskFormat = "512N"
	DiskFormat4k   DiskFormat = "4K"
)

// +kubebuilder:validation:Enum=Thin;Thick;ThickEagerZero

// DiskProvisioningMode represents the mode when provisioning a disk.
type DiskProvisioningMode string

const (
	DiskProvisioningModeThin           DiskProvisioningMode = "Thin"
	DiskProvisioningModeThick          DiskProvisioningMode = "Thick"
	DiskProvisioningModeThickEagerZero DiskProvisioningMode = "ThickEagerZero"
)

type StoragePolicyStatus struct {
	// +optional

	// Datastores are the observed datastores selected by the storage policy.
	Datastores []Datastore `json:"datastores,omitempty"`

	// +optional

	// DiskFormat is the observed disk format supported by the storage policy.
	DiskFormat DiskFormat `json:"diskFormat,omitempty"`

	// +optional

	// DiskProvisioningMode is the observed mode the storage policy uses when
	// provisioning a new disk.
	DiskProvisioningMode DiskProvisioningMode `json:"diskProvisioningMode,omitempty"`

	// +optional

	// StorageClasses are the observed names of StorageClass objects that
	// reference this storage policy.
	StorageClasses []string `json:"storageClasses,omitempty"`

	// +optional

	// Encrypted is the observed status of encryption support for the storage
	// policy.
	Encrypted bool `json:"encrypted,omitempty"`
}

// Sort ensures the list elements in the status are in a deterministic order.
func (s *StoragePolicyStatus) Sort() {

	// Sort the datastores.
	slices.SortFunc(s.Datastores, func(a, b Datastore) int {
		if a.ID.ServerID != b.ID.ServerID {
			switch {
			case a.ID.ServerID == "":
				return -1
			case b.ID.ServerID == "":
				return 1
			case a.ID.ServerID < b.ID.ServerID:
				return -1
			}
			return 1
		}
		return strings.Compare(a.ID.ObjectID, b.ID.ObjectID)
	})

	// Sort the storage classes.
	slices.Sort(s.StorageClasses)
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// StoragePolicy is the Schema for the storagepolicies API.
type StoragePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StoragePolicySpec   `json:"spec,omitempty"`
	Status StoragePolicyStatus `json:"status,omitempty"`
}

func (s StoragePolicy) NamespacedName() string {
	return s.Namespace + "/" + s.Name
}

// +kubebuilder:object:root=true

// StoragePolicyList contains a list of StoragePolicy.
type StoragePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StoragePolicy `json:"items"`
}

func init() {
	objectTypes = append(objectTypes,
		&StoragePolicy{},
		&StoragePolicyList{},
	)
}
