// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/storage"
)

// GetDefaultDiskProvisioningType gets the default disk provisioning type specified for the VM.
func GetDefaultDiskProvisioningType(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	storageProfileID string) (string, error) {

	if advOpts := vmCtx.VM.Spec.AdvancedOptions; advOpts != nil && advOpts.DefaultVolumeProvisioningOptions != nil {
		// Webhook validated the combination of provisioning options so we can set to EagerZeroedThick if set.
		if eagerZeroed := advOpts.DefaultVolumeProvisioningOptions.EagerZeroed; eagerZeroed != nil && *eagerZeroed {
			return string(types.OvfCreateImportSpecParamsDiskProvisioningTypeEagerZeroedThick), nil
		}

		if thinProv := advOpts.DefaultVolumeProvisioningOptions.ThinProvisioned; thinProv != nil {
			if *thinProv {
				return string(types.OvfCreateImportSpecParamsDiskProvisioningTypeThin), nil
			}
			// Explicitly setting ThinProvisioning to false means use thick provisioning.
			return string(types.OvfCreateImportSpecParamsDiskProvisioningTypeThick), nil
		}
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
