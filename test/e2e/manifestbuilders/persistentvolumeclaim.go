// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PersistentVolumeClaim returns the PersistentVolumeClaim described by pvc.
// The access modes default to ReadWriteOnce when none are specified.
func PersistentVolumeClaim(pvc PVC) (*corev1.PersistentVolumeClaim, error) {
	size, err := resource.ParseQuantity(pvc.RequestSize)
	if err != nil {
		return nil, fmt.Errorf("invalid request size %q for PVC %s: %w",
			pvc.RequestSize, pvc.ClaimName, err)
	}

	accessModes := pvc.AccessModes
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	obj := &corev1.PersistentVolumeClaim{
		TypeMeta: typeMeta("v1", "PersistentVolumeClaim"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvc.ClaimName,
			Namespace: pvc.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
			VolumeMode: pvc.VolumeMode,
		},
	}

	if pvc.StorageClassName != "" {
		obj.Spec.StorageClassName = &pvc.StorageClassName
	}

	return obj, nil
}

// PersistentVolumeClaims returns a PersistentVolumeClaim for each of the
// given pvcs.
func PersistentVolumeClaims(pvcs []PVC) ([]*corev1.PersistentVolumeClaim, error) {
	out := make([]*corev1.PersistentVolumeClaim, 0, len(pvcs))
	for _, pvc := range pvcs {
		obj, err := PersistentVolumeClaim(pvc)
		if err != nil {
			return nil, err
		}
		out = append(out, obj)
	}
	return out, nil
}

func persistentVolumeClaimObjects(pvcs []PVC) ([]ctrlclient.Object, error) {
	objs, err := PersistentVolumeClaims(pvcs)
	if err != nil {
		return nil, err
	}
	out := make([]ctrlclient.Object, len(objs))
	for i := range objs {
		out[i] = objs[i]
	}
	return out, nil
}
