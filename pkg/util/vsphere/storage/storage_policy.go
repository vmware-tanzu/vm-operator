// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/vmware/govmomi/pbm"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	storagev1 "k8s.io/api/storage/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// GetStoragePolicyStatus returns the storage policy status for a given profile
// ID.
func GetStoragePolicyStatus(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	pbmClient *pbm.Client,
	profileID string) (vmopv1.StoragePolicyStatus, error) {

	if ctx == nil {
		panic("ctx is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if vimClient == nil {
		panic("vimClient is nil")
	}
	if pbmClient == nil {
		panic("pbmClient is nil")
	}
	if profileID == "" {
		panic("profileID is empty")
	}

	var (
		status vmopv1.StoragePolicyStatus
		pid    = []pbmtypes.PbmProfileId{
			{
				UniqueId: profileID,
			},
		}
	)

	profiles, err := pbmClient.RetrieveContent(ctx, pid)
	if err != nil {
		return status, fmt.Errorf(
			"failed to retrieve content for profile %q: %w", profileID, err)
	}

	if len(profiles) == 0 {
		return status,
			fmt.Errorf("failed to find profile %q", profileID)
	}
	if len(profiles) > 1 {
		return status,
			fmt.Errorf("failed to find unique profile %q, count=%d",
				profileID,
				len(profiles))
	}

	p, ok := profiles[0].(*pbmtypes.PbmCapabilityProfile)
	if !ok {
		return status,
			fmt.Errorf("failed to assert PbmCapabilityProfile %q, type=%T",
				profileID,
				profiles[0])
	}

	c, ok := p.Constraints.(*pbmtypes.PbmCapabilitySubProfileConstraints)
	if !ok {
		return status,
			fmt.Errorf("failed to assert PbmCapabilitySubProfileConstraints %q, type=%T",
				profileID,
				p.Constraints)
	}

	// Determine the disk format and provisioning mode.
	for _, sp := range c.SubProfiles {
		for _, cap := range sp.Capability {
			switch cap.Id.Namespace {
			case "com.vmware.storage.storageformat":
				switch cap.Id.Id {
				case "SectorSize":
					for _, ci := range cap.Constraint {
						for _, pi := range ci.PropertyInstance {
							switch pi.Id {
							case "SectorSize":
								switch pi.Value {
								case "512n", "512N":
									status.DiskFormat = vmopv1.DiskFormat512n
								case "4kn", "4Kn", "4KN":
									status.DiskFormat = vmopv1.DiskFormat4k
								default:
									return status, fmt.Errorf(
										"unexpected value for SectorSize %v",
										pi.Value)
								}
							}
						}
					}
				}
			case "com.vmware.storage.volumeallocation":
				switch cap.Id.Id {
				case "VolumeAllocationType":
					for _, ci := range cap.Constraint {
						for _, pi := range ci.PropertyInstance {
							switch pi.Id {
							case "VolumeAllocationType":
								switch pi.Value {
								case "Conserve space when possible":
									status.DiskProvisioningMode = vmopv1.DiskProvisioningModeThin
								case "Reserve space":
									status.DiskProvisioningMode = vmopv1.DiskProvisioningModeThick
								case "Fully initialized":
									status.DiskProvisioningMode = vmopv1.DiskProvisioningModeThickEagerZero
								default:
									return status, fmt.Errorf(
										"unexpected value for VolumeAllocationType %v",
										pi.Value)
								}
							}
						}
					}
				}
			case "VSAN":
				switch cap.Id.Id {
				case "proportionalCapacity":
					for _, ci := range cap.Constraint {
						for _, pi := range ci.PropertyInstance {
							switch pi.Id {
							case "proportionalCapacity":
								v, ok := pi.Value.(int32)
								if !ok {
									return status, fmt.Errorf(
										"unexpected value for proportionalCapacity %[1]v, type=%[1]T",
										v)
								}
								switch v {
								case 0:
									status.DiskProvisioningMode = vmopv1.DiskProvisioningModeThin
								case 100:
									status.DiskProvisioningMode = vmopv1.DiskProvisioningModeThick
								default:
									return status, fmt.Errorf(
										"unexpected value for proportionalCapacity %v",
										v)
								}
							}
						}
					}
				}
			}
		}
	}

	// Determine if the policy supports encryption.
	supportsEncryption, err := pbmClient.SupportsEncryption(ctx, profileID)
	if err != nil {
		if !strings.Contains(err.Error(), "Invalid profile ID") {
			return status, fmt.Errorf(
				"failed to determine if policy %q supports encryption: %w",
				profileID,
				err)
		}
	}
	status.Encrypted = supportsEncryption

	// Determine which datastore types use this policy.
	assocEnts, err := pbmClient.QueryAssociatedEntities(ctx, pid)
	if err != nil {
		return status, fmt.Errorf(
			"failed to query associated entities for policy %q: %w",
			profileID, err)
	}
	var dsRefs []vimtypes.ManagedObjectReference
	for _, e := range assocEnts {
		if e.Object.ObjectType == string(pbmtypes.PbmObjectTypeDatastore) {
			dsRefs = append(dsRefs, vimtypes.ManagedObjectReference{
				Type:  string(vimtypes.ManagedObjectTypeDatastore),
				Value: e.Object.Key,
			})
		}
	}
	datastores := make([]mo.Datastore, len(dsRefs))
	pc := property.DefaultCollector(vimClient)
	if err := pc.Retrieve(
		ctx,
		dsRefs,
		[]string{"summary.type"},
		&datastores); err != nil {

		return status, fmt.Errorf(
			"failed to query datastore types for policy %q: %w",
			profileID, err)
	}
	inDSTypes := map[string]struct{}{}
	outDSTypes := map[vmopv1.DatastoreType]struct{}{}
	for _, ds := range datastores {
		if ds.Summary.Type != "" {
			inDSTypes[ds.Summary.Type] = struct{}{}
		}
	}
	for t := range inDSTypes {
		switch vimtypes.HostFileSystemVolumeFileSystemType(t) {
		case vimtypes.HostFileSystemVolumeFileSystemTypeVsan,
			vimtypes.HostFileSystemVolumeFileSystemTypeVsanD,
			vimtypes.HostFileSystemVolumeFileSystemTypeVVOL:

			outDSTypes[vmopv1.DatastoreTypeVSAN] = struct{}{}

		default: // VMFS
			outDSTypes[vmopv1.DatastoreTypeVMFS] = struct{}{}
		}
	}
	for t := range outDSTypes {
		status.DatastoreTypes = append(status.DatastoreTypes, t)
	}
	if len(status.DatastoreTypes) > 1 {
		slices.Sort(status.DatastoreTypes)
	}

	// Collect unique storage class names.
	uniqueStorageClassNameMap := map[string]struct{}{
		p.K8sCompliantName:                  {},
		p.K8sCompliantName + "-latebinding": {},
	}
	for _, n := range p.OtherK8sCompliantNames {
		uniqueStorageClassNameMap[n] = struct{}{}
	}
	for n := range uniqueStorageClassNameMap {
		var (
			obj storagev1.StorageClass
			key = ctrlclient.ObjectKey{Name: n}
		)
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			if err := ctrlclient.IgnoreNotFound(err); err != nil {
				return status, fmt.Errorf(
					"failed to get storage class %q for profile %q: %w",
					n, profileID, err)
			}
		} else {
			status.StorageClasses = append(status.StorageClasses, n)
		}
	}
	if len(status.StorageClasses) > 1 {
		slices.Sort(status.StorageClasses)
	}

	return status, nil
}
