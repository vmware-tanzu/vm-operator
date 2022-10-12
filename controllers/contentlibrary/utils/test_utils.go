// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"
)

func DummyClusterContentLibraryItem(name string) *imgregv1a1.ClusterContentLibraryItem {
	cclItem := &imgregv1a1.ClusterContentLibraryItem{
		TypeMeta: metav1.TypeMeta{
			Kind:       ClusterContentLibraryItemKind,
			APIVersion: imgregv1a1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: imgregv1a1.ClusterContentLibraryItemSpec{
			UUID: "dummy-ccl-item-uuid",
		},
		Status: imgregv1a1.ClusterContentLibraryItemStatus{
			Type:                     imgregv1a1.ContentLibraryItemTypeOvf,
			Name:                     "dummy-image-name",
			ContentVersion:           "dummy-content-version",
			ClusterContentLibraryRef: "dummy-ccl-name",
			Conditions: []imgregv1a1.Condition{
				{
					Type:   imgregv1a1.ReadyCondition,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	return cclItem
}

func GetTestCVMINameFrom(cclItemName string) string {
	return strings.Replace(cclItemName, ItemFieldNamePrefix, ImageFieldNamePrefix, 1)
}

func GetExpectedCVMIFrom(cclItem imgregv1a1.ClusterContentLibraryItem,
	providerFunc func(context.Context, string, *vmopv1a1.ClusterVirtualMachineImage) error) *vmopv1a1.ClusterVirtualMachineImage {

	cvmi := &vmopv1a1.ClusterVirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: GetTestCVMINameFrom(cclItem.Name),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         cclItem.APIVersion,
					Kind:               cclItem.Kind,
					Name:               cclItem.Name,
					Controller:         &[]bool{true}[0],
					BlockOwnerDeletion: &[]bool{true}[0],
				},
			},
		},
		Spec: vmopv1a1.VirtualMachineImageSpec{
			Type:    string(cclItem.Status.Type),
			ImageID: cclItem.Spec.UUID,
			ProviderRef: vmopv1a1.ContentProviderReference{
				APIVersion: cclItem.APIVersion,
				Kind:       cclItem.Kind,
				Name:       cclItem.Name,
			},
		},
		Status: vmopv1a1.VirtualMachineImageStatus{
			ImageName:      cclItem.Status.Name,
			ContentVersion: cclItem.Status.ContentVersion,
			ContentLibraryRef: &corev1.TypedLocalObjectReference{
				APIGroup: &imgregv1a1.GroupVersion.Group,
				Kind:     ClusterContentLibraryKind,
				Name:     cclItem.Status.ClusterContentLibraryRef,
			},
			Conditions: []vmopv1a1.Condition{
				{
					Type:   vmopv1a1.VirtualMachineImageProviderReadyCondition,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   vmopv1a1.VirtualMachineImageSyncedCondition,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	if providerFunc != nil {
		_ = providerFunc(nil, "", cvmi)
	}

	return cvmi
}

func PopulateRuntimeFieldsTo(cvmi *vmopv1a1.ClusterVirtualMachineImage,
	appliedCVMI vmopv1a1.ClusterVirtualMachineImage) {

	// Populate condition LastTransitionTime.
	if appliedCVMI.Status.Conditions != nil {
		transactionTimeMap := map[vmopv1a1.ConditionType]metav1.Time{}
		for _, condition := range appliedCVMI.Status.Conditions {
			transactionTimeMap[condition.Type] = condition.LastTransitionTime
		}
		updatedConditions := []vmopv1a1.Condition{}
		for _, condition := range cvmi.Status.Conditions {
			if transactionTime, ok := transactionTimeMap[condition.Type]; ok {
				condition.LastTransitionTime = transactionTime
			}
			updatedConditions = append(updatedConditions, condition)
		}
		cvmi.Status.Conditions = updatedConditions
	}

	// Populate owner reference UID.
	if appliedCVMI.OwnerReferences != nil {
		uidMap := map[string]types.UID{}
		for _, appliedOwnerReference := range appliedCVMI.OwnerReferences {
			uidMap[appliedOwnerReference.Name] = appliedOwnerReference.UID
		}
		updatedOwnerReferences := []metav1.OwnerReference{}
		for _, ownerReference := range cvmi.OwnerReferences {
			if uid, ok := uidMap[ownerReference.Name]; ok {
				ownerReference.UID = uid
			}
			updatedOwnerReferences = append(updatedOwnerReferences, ownerReference)
		}
		cvmi.OwnerReferences = updatedOwnerReferences
	}
}
