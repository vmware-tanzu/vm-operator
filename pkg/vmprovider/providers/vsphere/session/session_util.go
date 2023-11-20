// Copyright (c) 2018-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	vimTypes "github.com/vmware/govmomi/vim25/types"
)

const (
	// OvfEnvironmentTransportGuestInfo is the OVF transport type that uses
	// GuestInfo. The other valid type is "iso".
	OvfEnvironmentTransportGuestInfo = "com.vmware.guestInfo"
)

// GetMergedvAppConfigSpec prepares a vApp VmConfigSpec which will set the vmMetadata supplied key/value fields. Only
// fields marked userConfigurable and pre-existing on the VM (ie. originated from the OVF Image)
// will be set, and all others will be ignored.
func GetMergedvAppConfigSpec(inProps map[string]string, vmProps []vimTypes.VAppPropertyInfo) *vimTypes.VmConfigSpec {
	outProps := make([]vimTypes.VAppPropertySpec, 0)

	// Update any vApp properties if they have been specified in VM metadata.
	for _, vmProp := range vmProps {
		inPropValue, found := inProps[vmProp.Id]
		if !found || vmProp.Value == inPropValue {
			continue
		}

		vmPropCopy := vmProp
		vmPropCopy.Value = inPropValue
		outProp := vimTypes.VAppPropertySpec{
			ArrayUpdateSpec: vimTypes.ArrayUpdateSpec{
				Operation: vimTypes.ArrayUpdateOperationEdit,
			},
			Info: &vmPropCopy,
		}
		outProps = append(outProps, outProp)
	}

	if len(outProps) == 0 {
		return nil
	}

	return &vimTypes.VmConfigSpec{
		Property: outProps,
		// Ensure the transport is guestInfo in case the VM does not have
		// a CD-ROM device required to use the ISO transport.
		OvfEnvironmentTransport: []string{OvfEnvironmentTransportGuestInfo},
	}
}
