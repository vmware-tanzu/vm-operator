// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StoragePolicyLevelQuotaStatusList []StoragePolicyLevelQuotaStatus
type StoragePolicyLevelQuotaStatus struct {
	// ID of the storage policy
	StoragePolicyId string `json:"storagePolicyId"`

	// Storage quota usage details for given storage policy
	// +optional
	StoragePolicyLevelQuotaUsage QuotaUsageDetails `json:"policyQuotaUsage,omitempty"`
}

// StorageQuotaSpec defines the desired state of StorageQuota
type StorageQuotaSpec struct {
	// Total limit of storage across all types of vSphere storage resources
	// within given namespace
	// NOTE: This is analogous to "requests.storage" in Kubernetes ResourceQuota object.
	Limit *resource.Quantity `json:"limit,omitempty"`
	// Quota limits per storage policy associated with given namespace
	// +optional
	StoragePolicyLevelLimits map[string]*resource.Quantity `json:"storagePolicyLevelLimits,omitempty"`
}

// StorageQuotaStatus defines the observed state of StorageQuota
type StorageQuotaStatus struct {
	// Storage quota usage details per storage policy within given namespace
	// +optional
	StoragePolicyLevelQuotaStatuses StoragePolicyLevelQuotaStatusList `json:"total,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// StorageQuota is the Schema for the storagequotas API
type StorageQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StorageQuotaSpec   `json:"spec,omitempty"`
	Status StorageQuotaStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StorageQuotaList contains a list of StorageQuota
type StorageQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StorageQuota `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &StorageQuota{}, &StorageQuotaList{})
}
