// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package controllers adds every controller this manager runs.
package controllers

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	kubevmcore "github.com/vmware-tanzu/vm-operator/external/kubevm/controller/controllers/virtualmachine"

	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/controllers/awsmachine"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/ec2"
)

// The KubeVM core runs inside this manager, so this manager's role carries
// the core's permissions as well as its own. These are the core's own
// kube-vm.io markers, repeated. The core also asks for VM Operator's
// VirtualMachine group, which this manager never serves, so that one is not
// repeated. The finalizers grant is one the core's markers omit: it sets
// blockOwnerDeletion on the owner reference it writes, which a cluster
// enforcing owner-reference permissions refuses without it.
//
// Narrower than the core's own markers in two places, because a grant this
// manager never uses is one a leaked token could. The core writes through
// merge patches, so update is not needed; and it deletes the PROVIDER object,
// never a VirtualMachine, so delete on the user-facing object is not either.
// Both are raised upstream rather than silently mirrored.
//
// +kubebuilder:rbac:groups=kube-vm.io,resources=virtualmachines,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=kube-vm.io,resources=virtualmachines/status,verbs=get;patch
// +kubebuilder:rbac:groups=kube-vm.io,resources=virtualmachines/finalizers,verbs=update

// AddToManager adds the AWSMachine controller and the KubeVM core controller
// to mgr.
//
// The region is not a parameter: it is settled when the EC2 client is built,
// and a second copy here could disagree with the client actually in use.
func AddToManager(mgr ctrl.Manager, client ec2.Client) error {
	reconciler := &awsmachine.Reconciler{
		Client: mgr.GetClient(),
		EC2:    client,
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("adding the AWSMachine controller: %w", err)
	}

	// Hosted in-process, as VM Operator hosts it, rather than run as the
	// core's own binary. The core is then reconciled on this manager's cache,
	// so the manager's sync period also bounds how long the portable
	// VirtualMachine can lag behind this provider's status: the core watches
	// only VirtualMachine, and a status change here does not wake it.
	if err := kubevmcore.AddToManager(mgr); err != nil {
		return fmt.Errorf("adding the KubeVM core controller: %w", err)
	}
	return nil
}
