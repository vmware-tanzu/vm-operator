// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resize

import (
	"context"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// CreateResizeConfigSpec takes the current VM state in the ConfigInfo and compares it to the
// desired state in the ConfigSpec, returning a ConfigSpec with any required changes to drive
// the desired state.
func CreateResizeConfigSpec(
	_ context.Context,
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec) (vimtypes.VirtualMachineConfigSpec, error) {

	outCS := vimtypes.VirtualMachineConfigSpec{}

	compareNumCpus(ci, cs, &outCS)
	compareMemoryMB(ci, cs, &outCS)

	return outCS, nil
}

func compareNumCpus(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {

	if ci.Hardware.NumCPU != cs.NumCPUs {
		outCS.NumCPUs = cs.NumCPUs
	}
}

func compareMemoryMB(
	ci vimtypes.VirtualMachineConfigInfo,
	cs vimtypes.VirtualMachineConfigSpec,
	outCS *vimtypes.VirtualMachineConfigSpec) {

	if int64(ci.Hardware.MemoryMB) != cs.MemoryMB {
		outCS.MemoryMB = cs.MemoryMB
	}
}
