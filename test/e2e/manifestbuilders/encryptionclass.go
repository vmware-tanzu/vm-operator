// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
)

type EncryptionClass struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	KeyProvider string `json:"keyProvider"`
	KeyID       string `json:"keyID,omitempty"`
}

// GetEncryptionClassYaml returns the EncryptionClass described by class.
func GetEncryptionClassYaml(class EncryptionClass) []byte {
	return ToYAML(&byokv1.EncryptionClass{
		TypeMeta: typeMeta(byokv1.GroupVersion.String(), "EncryptionClass"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      class.Name,
			Namespace: class.Namespace,
		},
		Spec: byokv1.EncryptionClassSpec{
			KeyProvider: class.KeyProvider,
			KeyID:       class.KeyID,
		},
	})
}
