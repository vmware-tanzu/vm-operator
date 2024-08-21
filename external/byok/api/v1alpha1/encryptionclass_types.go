// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EncryptionClassSpec defines the desired state of EncryptionClass.
type EncryptionClassSpec struct {
	// KeyProvider describes the key provider used to encrypt/recrypt/decrypt
	// resources.
	KeyProvider string `json:"keyProvider"`

	// KeyID describes the key used to encrypt/recrypt/decrypt resources.
	KeyID string `json:"keyID"`
}

// EncryptionClassStatus defines the observed state of EncryptionClass.
type EncryptionClassStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=encclass
// +kubebuilder:storageversion:true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="KeyProvider",type="string",JSONPath=".spec.keyProvider"
// +kubebuilder:printcolumn:name="KeyID",type="string",JSONPath=".spec.keyID"

// EncryptionClass is the Schema for the encryptionclasses API.
type EncryptionClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EncryptionClassSpec   `json:"spec,omitempty"`
	Status EncryptionClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EncryptionClassList contains a list of EncryptionClass.
type EncryptionClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EncryptionClass `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &EncryptionClass{}, &EncryptionClassList{})
}
