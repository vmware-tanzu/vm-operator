// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package spq

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
)

type contextKeyType uint8

const contextKeyValue contextKeyType = 0

// FromContext returns the channel on which an event for a namespaced
// StorageClass resource may be sent to signal the sync of the usage for VMs
// in that namespace using that StorageClass.
func FromContext(ctx context.Context) chan event.GenericEvent {
	return cource.FromContext(ctx, contextKeyValue)
}
