// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/storage"
)

// GetDefaultDiskProvisioningType gets the default disk provisioning type specified for the VM.
func GetDefaultDiskProvisioningType(
	vmCtx context.VirtualMachineContextA2,
	vcClient *vcclient.Client,
	storageProfileID string) (string, error) {

	var defaultProvMode vmopv1.VirtualMachineVolumeProvisioningMode
	if adv := vmCtx.VM.Spec.Advanced; adv != nil {
		defaultProvMode = adv.DefaultVolumeProvisioningMode
	}

	switch defaultProvMode {
	case vmopv1.VirtualMachineVolumeProvisioningModeThin:
		return string(types.OvfCreateImportSpecParamsDiskProvisioningTypeThin), nil
	case vmopv1.VirtualMachineVolumeProvisioningModeThick:
		return string(types.OvfCreateImportSpecParamsDiskProvisioningTypeThick), nil
	case vmopv1.VirtualMachineVolumeProvisioningModeThickEagerZero:
		return string(types.OvfCreateImportSpecParamsDiskProvisioningTypeEagerZeroedThick), nil
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

	return string(types.OvfCreateImportSpecParamsDiskProvisioningTypeThin), nil
}
