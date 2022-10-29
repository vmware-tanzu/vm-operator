// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	crtlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

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
			ContentLibraryRef: imgregv1a1.ContentLibraryReference{
				Name:      "cl-dummy",
				Namespace: namespace,
			},
			Conditions: []imgregv1a1.Condition{
				{
					Type:   imgregv1a1.ReadyCondition,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	return clItem
}

func GetTestVMINameFrom(clItemName string) string {
	return strings.Replace(clItemName, ItemFieldNamePrefix, ImageFieldNamePrefix, 1)
}

func GetExpectedCVMIFrom(cclItem imgregv1a1.ClusterContentLibraryItem,
	providerFunc func(context.Context, string, crtlclient.Object) error) *vmopv1a1.ClusterVirtualMachineImage {

	cvmi := &vmopv1a1.ClusterVirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: GetTestVMINameFrom(cclItem.Name),
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

func GetExpectedVMIFrom(clItem imgregv1a1.ContentLibraryItem,
	providerFunc func(context.Context, string, crtlclient.Object) error) *vmopv1a1.VirtualMachineImage {

	vmi := &vmopv1a1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetTestVMINameFrom(clItem.Name),
			Namespace: clItem.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         clItem.APIVersion,
					Kind:               clItem.Kind,
					Name:               clItem.Name,
					Controller:         &[]bool{true}[0],
					BlockOwnerDeletion: &[]bool{true}[0],
				},
			},
		},
		Spec: vmopv1a1.VirtualMachineImageSpec{
			Type:    string(clItem.Status.Type),
			ImageID: clItem.Spec.UUID,
			ProviderRef: vmopv1a1.ContentProviderReference{
				APIVersion: clItem.APIVersion,
				Kind:       clItem.Kind,
				Name:       clItem.Name,
			},
		},
		Status: vmopv1a1.VirtualMachineImageStatus{
			ImageName:      clItem.Status.Name,
			ContentVersion: clItem.Status.ContentVersion,
			ContentLibraryRef: &corev1.TypedLocalObjectReference{
				APIGroup: &imgregv1a1.GroupVersion.Group,
				Kind:     ContentLibraryKind,
				Name:     clItem.Status.ContentLibraryRef.Name,
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
		_ = providerFunc(nil, "", vmi)
	}

	return vmi
}

func PopulateRuntimeFieldsTo(vmi, appliedVMI crtlclient.Object) {
	status := &vmopv1a1.VirtualMachineImageStatus{}
	appliedStatus := &vmopv1a1.VirtualMachineImageStatus{}

	switch vmi := vmi.(type) {
	case *vmopv1a1.ClusterVirtualMachineImage:
		status = &vmi.Status
		appliedStatus = &appliedVMI.(*vmopv1a1.ClusterVirtualMachineImage).Status
	case *vmopv1a1.VirtualMachineImage:
		status = &vmi.Status
		appliedStatus = &appliedVMI.(*vmopv1a1.VirtualMachineImage).Status
	}

	// Populate condition LastTransitionTime.
	if appliedStatus.Conditions != nil {
		transactionTimeMap := map[vmopv1a1.ConditionType]metav1.Time{}
		for _, condition := range appliedStatus.Conditions {
			transactionTimeMap[condition.Type] = condition.LastTransitionTime
		}
		updatedConditions := []vmopv1a1.Condition{}
		for _, condition := range status.Conditions {
			if transactionTime, ok := transactionTimeMap[condition.Type]; ok {
				condition.LastTransitionTime = transactionTime
			}
			updatedConditions = append(updatedConditions, condition)
		}
		status.Conditions = updatedConditions
	}

	// Populate owner reference UID.
	appliedOwnerReferences := appliedVMI.GetOwnerReferences()
	ownerReferences := vmi.GetOwnerReferences()
	if appliedOwnerReferences != nil {
		uidMap := map[string]types.UID{}
		for _, appliedOwnerReference := range appliedOwnerReferences {
			uidMap[appliedOwnerReference.Name] = appliedOwnerReference.UID
		}
		updatedOwnerReferences := []metav1.OwnerReference{}
		for _, ownerReference := range ownerReferences {
			if uid, ok := uidMap[ownerReference.Name]; ok {
				ownerReference.UID = uid
			}
			updatedOwnerReferences = append(updatedOwnerReferences, ownerReference)
		}
		ownerReferences = updatedOwnerReferences
		vmi.SetOwnerReferences(ownerReferences)
	}
}
