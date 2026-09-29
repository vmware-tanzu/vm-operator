// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

// GetContentSourceBindingYaml returns a v1alpha1 ContentSourceBinding YAML
// manifest.
func GetContentSourceBindingYaml(namespace, contentSourceName string) []byte {
	return ToYAML(ContentSourceBindingA1(namespace, contentSourceName))
}

// ContentSourceBindingA1 returns a v1alpha1 ContentSourceBinding for the
// ContentSource contentSourceName.
func ContentSourceBindingA1(namespace, contentSourceName string) *vmopv1a1.ContentSourceBinding {
	return &vmopv1a1.ContentSourceBinding{
		TypeMeta: typeMeta(vmopv1a1.GroupVersion.String(), "ContentSourceBinding"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      contentSourceName,
			Namespace: namespace,
		},
		ContentSourceRef: vmopv1a1.ContentSourceReference{
			APIVersion: vmopv1a1.GroupVersion.String(),
			Kind:       "ContentSource",
			Name:       contentSourceName,
		},
	}
}
