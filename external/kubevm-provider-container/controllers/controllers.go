// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package controllers adds every controller this manager runs.
package controllers

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	kubevmcore "github.com/vmware-tanzu/vm-operator/external/kubevm/controller/controllers/virtualmachine"

	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/controllers/containermachine"
)

// +kubebuilder:rbac:groups=kube-vm.io,resources=virtualmachines,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=kube-vm.io,resources=virtualmachines/status,verbs=get;patch
// +kubebuilder:rbac:groups=kube-vm.io,resources=virtualmachines/finalizers,verbs=update

// AddToManager adds the ContainerMachine controller and the KubeVM core
// controller to mgr.
//
// Both run in the same process, on the same manager, exactly as VM Operator
// hosts the core on vSphere and external/kubevm-provider-aws hosts it for
// EC2: one Deployment, one binary, one cache. See
// implementing-a-provider.md, "Run the core inside your own manager", for
// why that is the pattern to follow rather than running the core as a
// separate binary.
func AddToManager(mgr ctrl.Manager) error {
	reconciler := &containermachine.Reconciler{
		Client: mgr.GetClient(),
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("adding the ContainerMachine controller: %w", err)
	}

	if err := kubevmcore.AddToManager(mgr); err != nil {
		return fmt.Errorf("adding the KubeVM core controller: %w", err)
	}
	return nil
}
