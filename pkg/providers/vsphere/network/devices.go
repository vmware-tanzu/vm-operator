// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// MapEthernetDevicesToSpecIdx maps the VM's ethernet devices to the corresponding
// entry in the VM's Spec.
func MapEthernetDevicesToSpecIdx(
	vmCtx pkgctx.VirtualMachineContext,
	devices object.VirtualDeviceList) map[int32]int {

	if vmCtx.VM.Spec.Network == nil {
		return nil
	}

	ethCards := devices.SelectByType((*vimtypes.VirtualEthernetCard)(nil))
	devKeyToSpecIdx := make(map[int32]int)

	// Just zip these together for now until we actually determine the matching entries.
	for i := range min(len(ethCards), len(vmCtx.VM.Spec.Network.Interfaces)) {
		devKeyToSpecIdx[ethCards[i].GetVirtualDevice().Key] = i
	}

	return devKeyToSpecIdx
}
