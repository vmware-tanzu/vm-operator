// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmclass

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

// ResourceDefaultsProvider resolves class resources into VM resource units.
type ResourceDefaultsProvider interface {
	GetVirtualMachineClassResources(context.Context, vmopv1.VirtualMachineClassSpec) (*vmopv1.VirtualMachineResourcesSpec, error)
}

// ApplyResourceDefaults materializes class resources on creation or a class
// change. Ordinary updates preserve the VM's selected configuration, even if
// its class has since been edited. Removing the class also retains its values.
func ApplyResourceDefaults(
	ctx context.Context,
	c ctrlclient.Client,
	provider ResourceDefaultsProvider,
	vm, oldVM *vmopv1.VirtualMachine,
) (bool, error) {
	features := pkgcfg.FromContext(ctx).Features
	if !features.TelcoVMServiceAPI || vm.Spec.ClassName == "" {
		return false, nil
	}
	if oldVM != nil {
		if !features.VMResizeCPUMemory ||
			(vm.Spec.ClassName == oldVM.Spec.ClassName &&
				apiequality.Semantic.DeepEqual(vm.Spec.Class, oldVM.Spec.Class)) {
			return false, nil
		}
	}
	var spec vmopv1.VirtualMachineClassSpec
	useInstance := features.ImmutableClasses && vm.Spec.Class != nil && vm.Spec.Class.Name != ""
	// A className change must not read a snapshot of the previous class.
	if oldVM != nil && vm.Spec.ClassName != oldVM.Spec.ClassName &&
		apiequality.Semantic.DeepEqual(vm.Spec.Class, oldVM.Spec.Class) {
		useInstance = false
	}
	if useInstance {
		var instance vmopv1.VirtualMachineClassInstance
		key := ctrlclient.ObjectKey{Namespace: vm.Namespace, Name: vm.Spec.Class.Name}
		if err := c.Get(ctx, key, &instance); err != nil {
			return false, fmt.Errorf("resolve class instance %s: %w", key, err)
		}
		spec = instance.Spec.VirtualMachineClassSpec
	} else {
		var class vmopv1.VirtualMachineClass
		key := ctrlclient.ObjectKey{Namespace: vm.Namespace, Name: vm.Spec.ClassName}
		if err := c.Get(ctx, key, &class); err != nil {
			// Existing provisioning flows admit the VM before its class is copied
			// into the namespace. Reconciliation resolves it when it arrives.
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("resolve class %s: %w", key, err)
		}
		spec = class.Spec
	}
	if provider == nil {
		return false, fmt.Errorf("class resource defaults provider is unavailable")
	}
	resources, err := provider.GetVirtualMachineClassResources(ctx, spec)
	if err != nil {
		return false, fmt.Errorf("resolve resources for class %s: %w", vm.Spec.ClassName, err)
	}
	if apiequality.Semantic.DeepEqual(vm.Spec.Resources, resources) {
		return false, nil
	}
	vm.Spec.Resources = resources.DeepCopy()
	return true, nil
}
