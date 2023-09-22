// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
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

// GetImageFieldNameFromItem returns the image field name in the format of
// "vmi-<uuid>" by using the identifier from the given item name "clitem-<uuid>".
func GetImageFieldNameFromItem(itemName string) (string, error) {
	if !strings.HasPrefix(itemName, ItemFieldNamePrefix) {
		return "", fmt.Errorf("item name %q doesn't start with %q",
			itemName, ItemFieldNamePrefix)
	}

	itemNameSplit := strings.Split(itemName, "-")
	if len(itemNameSplit) < 2 {
		return "", fmt.Errorf("item name %q doesn't contain a uuid", itemName)
	}

	uuid := strings.Join(itemNameSplit[1:], "-")
	return fmt.Sprintf("%s-%s", ImageFieldNamePrefix, uuid), nil
}

// IsItemReady checks if an item is ready by iterating through its conditions.
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

// GetVMImageSpecStatus retrieves the VirtualMachineImageSpec and
// VirtualMachineImageStatus for a given image name and namespace.
// It first checks if a namespace scope image exists by the resource name,
// and if not, checks if a cluster scope image exists by the resource name.
func GetVMImageSpecStatus(
	ctx context.Context,
	ctrlClient client.Client,
	imageName, namespace string) (
	*vmopv1.VirtualMachineImageSpec,
	*vmopv1.VirtualMachineImageStatus,
	error) {
	// Check if a namespace scope image exists by the resource name.
	vmi := vmopv1.VirtualMachineImage{}
	key := client.ObjectKey{Name: imageName, Namespace: namespace}
	err := ctrlClient.Get(ctx, key, &vmi)
	if err == nil {
		return &vmi.Spec, &vmi.Status, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, nil, err
	}

	// Check if a cluster scope image exists by the resource name.
	cvmi := vmopv1.ClusterVirtualMachineImage{}
	key = client.ObjectKey{Name: imageName}
	err = ctrlClient.Get(ctx, key, &cvmi)
	if err == nil {
		return &cvmi.Spec, &cvmi.Status, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, nil, err
	}

	err = fmt.Errorf(
		"cannot find a namespace or cluster scope VM image for resource name %q",
		imageName,
	)
	return nil, nil, err
}
