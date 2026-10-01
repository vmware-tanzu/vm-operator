// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetStorageQuotaYAML returns a ResourceQuota that limits the
// gc-storage-profile StorageClass to 1Gi of requested storage. It has no
// namespace: the caller applies it with "-n <namespace>".
func GetStorageQuotaYAML() ([]byte, error) {
	return ToYAML(&corev1.ResourceQuota{
		TypeMeta: typeMeta("v1", "ResourceQuota"),
		ObjectMeta: metav1.ObjectMeta{
			Name: "gc-storage-quota",
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				"gc-storage-profile.storageclass.storage.k8s.io/requests.storage": resource.MustParse("1Gi"),
			},
		},
	}), nil
}
