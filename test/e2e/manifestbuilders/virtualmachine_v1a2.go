// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a2sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha2/sysprep"
)

// VirtualMachineA2 returns the v1alpha2 VirtualMachine described by vmYaml. An
// error is returned if an inline cloud config or sysprep cannot be decoded.
func VirtualMachineA2(vmYaml VirtualMachineYaml) (*vmopv1a2.VirtualMachine, error) {
	vm := &vmopv1a2.VirtualMachine{
		TypeMeta:   typeMeta(vmopv1a2.GroupVersion.String(), "VirtualMachine"),
		ObjectMeta: objectMeta(vmYaml),
		Spec: vmopv1a2.VirtualMachineSpec{
			ClassName:    vmYaml.VMClassName,
			StorageClass: vmYaml.StorageClassName,
			ImageName:    vmYaml.ImageName,
			PowerState:   vmopv1a2.VirtualMachinePowerState(vmYaml.PowerState),
			PowerOffMode: vmopv1a2.VirtualMachinePowerOpMode(vmYaml.PowerOffMode),
		},
	}

	if vmYaml.Bootstrap.hasBootstrap() {
		bootstrap, err := bootstrapA2(vmYaml.Bootstrap)
		if err != nil {
			return nil, err
		}
		vm.Spec.Bootstrap = bootstrap
	}

	return vm, nil
}

func bootstrapA2(in Bootstrap) (*vmopv1a2.VirtualMachineBootstrapSpec, error) {
	out := &vmopv1a2.VirtualMachineBootstrapSpec{}

	if sp := in.Sysprep; sp != nil {
		sysprep, err := unmarshalInline[vmopv1a2sysprep.Sysprep](sp.Sysprep)
		if err != nil {
			return nil, err
		}
		if sysprep != nil {
			out.Sysprep = &vmopv1a2.VirtualMachineBootstrapSysprepSpec{
				Sysprep: sysprep,
			}
		}
	}

	return out, nil
}
