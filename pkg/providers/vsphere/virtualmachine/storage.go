// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/storage"
)

// GetDefaultDiskProvisioningType gets the default disk provisioning type specified for the VM.
func GetDefaultDiskProvisioningType(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client,
	storageProfileID string) (string, error) {

	var defaultProvMode vmopv1.VirtualMachineVolumeProvisioningMode
	if adv := vmCtx.VM.Spec.Advanced; adv != nil {
		defaultProvMode = adv.DefaultVolumeProvisioningMode
	}

	switch defaultProvMode {
	case vmopv1.VirtualMachineVolumeProvisioningModeThin:
		return string(vimtypes.OvfCreateImportSpecParamsDiskProvisioningTypeThin), nil
	case vmopv1.VirtualMachineVolumeProvisioningModeThick:
		return string(vimtypes.OvfCreateImportSpecParamsDiskProvisioningTypeThick), nil
	case vmopv1.VirtualMachineVolumeProvisioningModeThickEagerZero:
		return string(vimtypes.OvfCreateImportSpecParamsDiskProvisioningTypeEagerZeroedThick), nil
	}

	if storageProfileID != "" {
		provisioning, err := storage.GetDiskProvisioningForProfile(vmCtx, vcClient, storageProfileID)
		if err != nil {
			return "", err
		}
		if provisioning != "" {
			return provisioning, nil
		}
	}

	return string(vimtypes.OvfCreateImportSpecParamsDiskProvisioningTypeThin), nil
}
