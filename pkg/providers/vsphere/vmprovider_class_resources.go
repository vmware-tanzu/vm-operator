// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// GetVirtualMachineClassResources resolves class resource defaults using the
// same CPU frequency and configuration precedence as VM creation.
func (vs *vSphereVMProvider) GetVirtualMachineClassResources(
	ctx context.Context,
	spec vmopv1.VirtualMachineClassSpec,
) (*vmopv1.VirtualMachineResourcesSpec, error) {
	configSpec, err := virtualmachine.ClassConfigSpecFromClassSpec(ctx, &spec)
	if err != nil {
		return nil, fmt.Errorf("resolve class configuration: %w", err)
	}
	var frequency uint64
	for _, cpu := range []*vmopv1.ResourceQuantityRange{
		spec.Resources.Requests.CPU, spec.Resources.Limits.CPU,
	} {
		if cpu != nil && cpu.Default != nil && !cpu.Default.IsZero() {
			frequency, err = vs.getOrComputeCPUMinFrequency(ctx)
			if err != nil {
				return nil, fmt.Errorf("resolve class CPU frequency: %w", err)
			}
			if frequency == 0 {
				return nil, fmt.Errorf("class CPU frequency is unavailable")
			}
			break
		}
	}
	vm := &vmopv1.VirtualMachine{}
	configSpec = virtualmachine.CreateConfigSpec(
		pkgctx.VirtualMachineContext{Context: ctx, VM: vm},
		configSpec, spec, vmopv1.VirtualMachineImageStatus{}, frequency)
	vmopv1util.SyncClassSizeAndAllocationToSpec(vm, configSpec)
	return vm.Spec.Resources, nil
}
