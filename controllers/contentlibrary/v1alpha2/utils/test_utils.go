// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
)

// ContentLibraryServiceTypeLabelKey is used to differentiate a TKG resource from a VM service resource.
const ContentLibraryServiceTypeLabelKey = "type.services.vmware.com/tkg"

func DummyClusterContentLibraryItem(name string) *imgregv1a1.ClusterContentLibraryItem {
	cclItem := &imgregv1a1.ClusterContentLibraryItem{
		TypeMeta: metav1.TypeMeta{
			Kind:       ClusterContentLibraryItemKind,
			APIVersion: imgregv1a1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				ContentLibraryServiceTypeLabelKey: "",
				"dummy-not-service-label":         "",
			},
		},
		Spec: imgregv1a1.ContentLibraryItemSpec{
			UUID: "dummy-ccl-item-uuid",
		},
		Status: imgregv1a1.ContentLibraryItemStatus{
			Type:           imgregv1a1.ContentLibraryItemTypeOvf,
			Name:           "dummy-image-name",
			ContentVersion: "dummy-content-version",
			ContentLibraryRef: &imgregv1a1.NameAndKindRef{
				Kind: ClusterContentLibraryKind,
				Name: "dummy-ccl-name",
			},
			Conditions: []imgregv1a1.Condition{
				{
					Type:   imgregv1a1.ReadyCondition,
					Status: corev1.ConditionTrue,
				},
			},
			SecurityCompliance: &[]bool{true}[0],
		},
	}

	return cclItem
}

func DummyContentLibraryItem(name, namespace string) *imgregv1a1.ContentLibraryItem {
	clItem := &imgregv1a1.ContentLibraryItem{
		TypeMeta: metav1.TypeMeta{
			Kind:       ContentLibraryItemKind,
			APIVersion: imgregv1a1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: imgregv1a1.ContentLibraryItemSpec{
			UUID: "dummy-cl-item-uuid",
		},
		Status: imgregv1a1.ContentLibraryItemStatus{
			Type:           imgregv1a1.ContentLibraryItemTypeOvf,
			Name:           "dummy-image-name",
			ContentVersion: "dummy-content-version",
			ContentLibraryRef: &imgregv1a1.NameAndKindRef{
				Kind: ContentLibraryKind,
				Name: "cl-dummy",
			},
			Conditions: []imgregv1a1.Condition{
				{
					Type:   imgregv1a1.ReadyCondition,
					Status: corev1.ConditionTrue,
				},
			},
			SecurityCompliance: &[]bool{true}[0],
		},
	}

	return clItem
}

func FilterServicesTypeLabels(labels map[string]string) map[string]string {
	filtered := make(map[string]string)

	// Only watch for service type labels
	for label := range labels {
		if strings.HasPrefix(label, "type.services.vmware.com/") {
			filtered[label] = ""
		}
	}
	return filtered
}
