// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// FillEmptyNetworkInterfaceTypesFromClass sets spec.network.interfaces[].type when
// unset, using class ConfigSpec ethernet devices when mappable, otherwise
// VirtualMachineNetworkInterfaceTypeVMXNet3.
func FillEmptyNetworkInterfaceTypesFromClass(
	ctx context.Context,
	c ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 {
		return false, nil
	}

	cs, err := ClassConfigSpecForVirtualMachine(ctx, c, vm)
	if err != nil {
		return false, err
	}

	classTypes := NetworkInterfaceTypesFromClassConfigSpec(cs)
	mutated := false
	for i := range vm.Spec.Network.Interfaces {
		if vm.Spec.Network.Interfaces[i].Type != "" {
			continue
		}
		t := vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3
		if i < len(classTypes) && classTypes[i] != "" {
			t = classTypes[i]
		}
		vm.Spec.Network.Interfaces[i].Type = t
		mutated = true
	}
	return mutated, nil
}

// NetworkInterfaceTypesFromClassConfigSpec returns NIC types for ethernet
// devices in the order they appear in the ConfigSpec DeviceChange list.
func NetworkInterfaceTypesFromClassConfigSpec(
	cs vimtypes.VirtualMachineConfigSpec) []vmopv1.VirtualMachineNetworkInterfaceType {

	out := make([]vmopv1.VirtualMachineNetworkInterfaceType, 0, len(cs.DeviceChange))
	for idx := range cs.DeviceChange {
		spec := cs.DeviceChange[idx].GetVirtualDeviceConfigSpec()
		if spec == nil || !pkgutil.IsEthernetCard(spec.Device) {
			continue
		}
		out = append(out, mapVimEthernetToNetworkInterfaceType(spec.Device))
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
	default:
		return ""
	}
}
