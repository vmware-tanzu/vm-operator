// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

// GetImageFieldNameFromItem returns the Image field name in format of "vmi-<uuid>"
// by using the same identifier from the given Item name in "clitem-<uuid>".
func GetImageFieldNameFromItem(itemName string) (string, error) {
	if !strings.HasPrefix(itemName, ItemFieldNamePrefix) {
		return "", fmt.Errorf("item name doesn't start with %q", ItemFieldNamePrefix)
	}
	itemNameSplit := strings.Split(itemName, "-")
	if len(itemNameSplit) < 2 {
		return "", fmt.Errorf("item name doesn't have an identifier after %s-", ItemFieldNamePrefix)
	}

	uuid := strings.Join(itemNameSplit[1:], "-")
	return fmt.Sprintf("%s-%s", ImageFieldNamePrefix, uuid), nil
}

// IsItemReady returns if the given item conditions contain a Ready condition with status True.
func IsItemReady(itemConditions imgregv1a1.Conditions) bool {
	var isReady bool
	for _, condition := range itemConditions {
		if condition.Type == imgregv1a1.ReadyCondition {
			isReady = condition.Status == corev1.ConditionTrue
			break
		}
	}

	return isReady
}

// GetVMImageSpecStatus returns the VirtualMachineImage Spec and Status fields in an image-registry service enabled cluster.
// It first tries to get the namespace scope VM Image; if not found, it looks up the cluster scope VM Image.
func GetVMImageSpecStatus(ctx context.Context, ctrlClient client.Client, imageName, namespace string) (
	spec *vmopv1.VirtualMachineImageSpec, status *vmopv1.VirtualMachineImageStatus, err error) {

	vmi := vmopv1.VirtualMachineImage{}
	if err = ctrlClient.Get(ctx, client.ObjectKey{Name: imageName, Namespace: namespace}, &vmi); err == nil {
		spec = &vmi.Spec
		status = &vmi.Status
	} else if apierrors.IsNotFound(err) {
		// Namespace scope image is not found. Check if a cluster scope image with this name exists.
		cvmi := vmopv1.ClusterVirtualMachineImage{}
		if err = ctrlClient.Get(ctx, client.ObjectKey{Name: imageName}, &cvmi); err == nil {
			spec = &cvmi.Spec
			status = &cvmi.Status
		}
	}

	return spec, status, err
}
