// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// IsOnlinePromoteDisksSupported returns true if the provided configSpec would
// create a VM that supports online disk promotion.
func IsOnlinePromoteDisksSupported(
	configSpec vimtypes.VirtualMachineConfigSpec) bool {

	// Check for specific device/backing types that would disallow online
	// promote.
	for _, bvdcs := range configSpec.DeviceChange {
		if bvdcs != nil {
			if vdcs := bvdcs.GetVirtualDeviceConfigSpec(); vdcs != nil {
				if bvd := vdcs.Device; bvd != nil {
					switch tvd := bvd.(type) { //nolint:gocritic
					case *vimtypes.VirtualPCIPassthrough:

						// Only Vmiop (nVidia vGPU) and DVX backings support
						// online disk promotion.
						switch tvd.Backing.(type) {
						case *vimtypes.VirtualPCIPassthroughDeviceBackingInfo,
							*vimtypes.VirtualPCIPassthroughDynamicBackingInfo:

							return false
						}
					}
				}
			}
		}
	}

	return true
}
