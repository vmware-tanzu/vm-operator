// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"fmt"
	"hash/fnv"
	"strings"

	"k8s.io/apimachinery/pkg/types"
)

// LifecycleStateName returns the canonical name of the LifecycleState for a
// target object, given the target's kind and UID. Both the Lifecycle
// Operator and a target operator (e.g. VM Operator) derive this name
// independently, so a Get by this name is how a target operator finds its
// own object's LifecycleState without a List.
func LifecycleStateName(kind string, uid types.UID) string {
	return fmt.Sprintf("%s-%s", strings.ToLower(kind), uid)
}

// SubscribedStagesName returns the canonical name of the
// LifecycleSubscribedStages object for one (target group, target kind) pair
// in a namespace. It is a short, deterministic hash of the pair so the name
// fits Kubernetes object-name limits regardless of group/kind length.
func SubscribedStagesName(group, kind string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(group + "/" + kind))

	return fmt.Sprintf("lss-%016x", h.Sum64())
}
