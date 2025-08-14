// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmconfig

import (
	"context"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// Reconciler is a type that modifies a VirtualMachine's configuration before
// and/or after creating or updating the underlying, platform resource.
type Reconciler interface {

	// Name returns the unique name used to identify the reconciler.
	Name() string

	// Reconcile should be called prior to creating a VirtualMachine or updating
	// its configuration.
	Reconcile(
		ctx context.Context,
		k8sClient ctrlclient.Client,
		vimClient *vim25.Client,
		vm *vmopv1.VirtualMachine,
		moVM mo.VirtualMachine,
		configSpec *vimtypes.VirtualMachineConfigSpec) error

	// OnResult should be called after creating a VirtualMachine or updating its
	// configuration.
	OnResult(
		ctx context.Context,
		vm *vmopv1.VirtualMachine,
		moVM mo.VirtualMachine,
		resultErr error) error
}

// ReconcilerWithContext is a Reconciler that has state to share between its
// Reconcile and OnResult methods and does so via the context.
type ReconcilerWithContext interface {
	Reconciler

	// WithContext returns a context used to share state between the Reconcile
	// and OnResult methods.
	WithContext(context.Context) context.Context
}
