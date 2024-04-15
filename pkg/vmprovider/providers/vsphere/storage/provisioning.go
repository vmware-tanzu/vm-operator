// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/pbm"
	pbmTypes "github.com/vmware/govmomi/pbm/types"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
)

// getProfileProportionalCapacity returns the storage profile "proportionalCapacity" value if the
// policy is for vSAN. The proportionalCapacity is percentage of the logical size of the storage
// object that will be reserved upon provisioning. Returns -1 if not specified.
// The UI presents options for "thin" (0%), 25%, 50%, 75% and "thick" (100%).
func getProfileProportionalCapacity(profile pbmTypes.BasePbmProfile) int32 {
	capProfile, ok := profile.(*pbmTypes.PbmCapabilityProfile)
	if !ok {
		return -1
	}

	if capProfile.ResourceType.ResourceType != string(pbmTypes.PbmProfileResourceTypeEnumSTORAGE) {
		return -1
	}

	if capProfile.ProfileCategory != string(pbmTypes.PbmProfileCategoryEnumREQUIREMENT) {
		return -1
	}

	sub, ok := capProfile.Constraints.(*pbmTypes.PbmCapabilitySubProfileConstraints)
	if !ok {
		return -1
	}

	for _, p := range sub.SubProfiles {
		for _, capability := range p.Capability {
			if capability.Id.Namespace != "VSAN" || capability.Id.Id != "proportionalCapacity" {
				continue
			}

			for _, c := range capability.Constraint {
				for _, prop := range c.PropertyInstance {
					if prop.Id != capability.Id.Id {
						continue
					}
					if val, ok := prop.Value.(int32); ok {
						return val
					}
				}
			}
		}
	}

	return -1
}

// GetDiskProvisioningForProfile returns the provisioning type for the storage profile if it has
// one specified.
func GetDiskProvisioningForProfile(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	storageProfileID string) (string, error) {

	c, err := pbm.NewClient(vmCtx, vcClient.VimClient())
	if err != nil {
		return "", err
	}

	profiles, err := c.RetrieveContent(vmCtx, []pbmTypes.PbmProfileId{{UniqueId: storageProfileID}})
	if err != nil {
		return "", errors.Wrapf(err, "Failed to get storage profiles for ID: %s", storageProfileID)
	}

	for _, p := range profiles {
		switch getProfileProportionalCapacity(p) {
		case 0:
			return string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeThin), nil
		case 100:
			return string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeThick), nil
		}
	}

	return "", nil
}
