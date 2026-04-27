// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// TODO(BV): Move this out of the provider package.

// FillEmptyNetworkInterfaceTypesFromMoVM sets spec.network.interfaces[].type
// when unset, using ethernet devices from the VM's current vSphere config in
// moVM, defaulting to VirtualMachineNetworkInterfaceTypeVMXNet3.
func FillEmptyNetworkInterfaceTypesFromMoVM(
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) (bool, error) {

	if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 {
		return false, nil
	}

	ethTypes := networkInterfaceTypesFromMoVM(moVM)
	mutated := false
	// TODO(BV): Do actual matching between the spec and devices, but for
	// now just zip together.
	for i := range vm.Spec.Network.Interfaces {
		if vm.Spec.Network.Interfaces[i].Type != "" {
			continue
		}
		t := vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3
		if i < len(ethTypes) {
			t = ethTypes[i]
		}
		vm.Spec.Network.Interfaces[i].Type = t
		mutated = true
	}
	return mutated, nil
}

func networkInterfaceTypesFromMoVM(
	moVM mo.VirtualMachine) []vmopv1.VirtualMachineNetworkInterfaceType {

	if moVM.Config == nil {
		return nil
	}

	out := make([]vmopv1.VirtualMachineNetworkInterfaceType, 0, len(moVM.Config.Hardware.Device))
	for idx := range moVM.Config.Hardware.Device {
		dev := moVM.Config.Hardware.Device[idx]
		if !pkgutil.IsEthernetCard(dev) {
			continue
		}
		t := mapVimEthernetToNetworkInterfaceType(dev)
		if t == "" {
			t = vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3
		}
		out = append(out, t)
	}
	return out
}

func mapVimEthernetToNetworkInterfaceType(
	dev vimtypes.BaseVirtualDevice) vmopv1.VirtualMachineNetworkInterfaceType {

	switch dev.(type) {
	case *vimtypes.VirtualSriovEthernetCard:
		return vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV
	case *vimtypes.VirtualVmxnet3:
		return vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3
	case *vimtypes.VirtualE1000:
		return vmopv1.VirtualMachineNetworkInterfaceTypeE1000
	case *vimtypes.VirtualE1000e:
		return vmopv1.VirtualMachineNetworkInterfaceTypeE1000e
	case *vimtypes.VirtualVmxnet2:
		return vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet2
	case *vimtypes.VirtualPCNet32:
		return vmopv1.VirtualMachineNetworkInterfaceTypePCNet32
	default:
		return ""
	}
}
