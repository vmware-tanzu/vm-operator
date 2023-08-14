// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
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
