// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/pbm"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	storagev1 "k8s.io/api/storage/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

const (
	volumeAllocationNamespace = "com.vmware.storage.volumeallocation"
	// storageLocalityID is both the capability id and the property id in this
	// schema.
	storageLocalityID        = "StorageLocality"
	storageLocalityHostLocal = "HostLocalStorage"
)

// IsHostLocalStorageCapabilityPolicy returns true if the policy contains the
// com.vmware.storage.volumeallocation/StorageLocality SPBM capability with its
// property value set to HostLocalStorage. The StorageLocality capability also
// allows the value None (its default), which means no locality preference must
// NOT be treated as host-local.
// This function does NOT match vSAN Direct or vSAN locality/SNA policies.
func IsHostLocalStorageCapabilityPolicy(
	subprofiles *pbmtypes.PbmCapabilitySubProfileConstraints) bool {

	if subprofiles == nil {
		return false
	}
	for _, subprofile := range subprofiles.SubProfiles {
		for _, capIns := range subprofile.Capability {
			if capIns.Id.Namespace != volumeAllocationNamespace ||
				capIns.Id.Id != storageLocalityID {

				continue
			}
			for _, constraint := range capIns.Constraint {
				for _, propInstance := range constraint.PropertyInstance {
					if propInstance.Id == storageLocalityID &&
						storageLocalityValueIsHostLocal(propInstance.Value) {

						return true
					}
				}
			}
		}
	}
	return false
}

// storageLocalityValueIsHostLocal reports whether a StorageLocality property
// value denotes host-local storage. The value is normally a plain string, but
// SPBM may express it as a discrete set of allowed values, in which case any
// member matching is sufficient.
func storageLocalityValueIsHostLocal(value vimtypes.AnyType) bool {
	switch v := value.(type) {
	case string:
		return v == storageLocalityHostLocal
	case pbmtypes.PbmCapabilityDiscreteSet:
		for _, item := range v.Values {
			if s, ok := item.(string); ok && s == storageLocalityHostLocal {
				return true
			}
		}
	case *pbmtypes.PbmCapabilityDiscreteSet:
		if v != nil {
			for _, item := range v.Values {
				if s, ok := item.(string); ok && s == storageLocalityHostLocal {
					return true
				}
			}
		}
	}
	return false
}

// GetStoragePolicyStatus returns the storage policy status for a given profile
// ID.
func GetStoragePolicyStatus(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	pbmClient *pbm.Client,
	profileID string) (infrav1.StoragePolicyStatus, error) {

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

	var status infrav1.StoragePolicyStatus

	// Get the profile and its constraints.
	p, c, err := getCapabilityProfileAndConstraints(
		ctx,
		pbmClient,
		[]pbmtypes.PbmProfileId{
			{
				UniqueId: profileID,
			},
		},
		profileID)
	if err != nil {
		return infrav1.StoragePolicyStatus{}, err
	}

	// Get the information from the constraints.
	if err := parseConstraints(c, &status); err != nil {
		return infrav1.StoragePolicyStatus{}, err
	}

	// Determine if the policy requires host-local storage. This is a distinct
	// capability from the disk format and provisioning mode read above, so it
	// is evaluated separately rather than folded into parseConstraints.
	status.HostLocal = IsHostLocalStorageCapabilityPolicy(c)

	// Determine if the policy supports encryption.
	if err := supportsEncryption(
		ctx,
		pbmClient,
		profileID,
		&status); err != nil {

		return infrav1.StoragePolicyStatus{}, err
	}

	// Determine which datastore types use this policy.
	if err := getAssociatedDatastores(
		ctx,
		vimClient,
		pbmClient,
		profileID,
		&status); err != nil {

		return infrav1.StoragePolicyStatus{}, err
	}

	// Collect unique storage class names.
	if err := getStorageClassNames(
		ctx,
		k8sClient,
		p,
		profileID,
		&status); err != nil {

		return infrav1.StoragePolicyStatus{}, err
	}

	// Collect volume attributes class name only if capability is enabled.
	if pkgcfg.FromContext(ctx).Features.StoragePolicyMutability {
		if err := getVolumeAttributesClassName(
			ctx,
			k8sClient,
			p,
			profileID,
			&status); err != nil {

			return infrav1.StoragePolicyStatus{}, err
		}
	}

	// Sort the slices in the status.
	status.Sort()

	return status, nil
}

func getStorageClassNames(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	p *pbmtypes.PbmCapabilityProfile,
	profileID string,
	status *infrav1.StoragePolicyStatus) error {

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
		err := k8sClient.Get(ctx, key, &obj)
		if err != nil {
			if ctrlclient.IgnoreNotFound(err) == nil {
				continue
			}
			return fmt.Errorf(
				"failed to get storage class %q for profile %q: %w",
				n, profileID, err)
		}
		status.StorageClasses = append(status.StorageClasses, n)
	}
	return nil
}

func getVolumeAttributesClassName(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	p *pbmtypes.PbmCapabilityProfile,
	profileID string,
	status *infrav1.StoragePolicyStatus) error {

	var (
		obj storagev1.VolumeAttributesClass
		key = ctrlclient.ObjectKey{Name: p.K8sCompliantName}
	)
	err := k8sClient.Get(ctx, key, &obj)
	if err != nil {
		if ctrlclient.IgnoreNotFound(err) == nil {
			return nil
		}
		return fmt.Errorf(
			"failed to get volume attributes class %q for profile %q: %w",
			p.K8sCompliantName, profileID, err)
	}
	status.VolumeAttributesClass = p.K8sCompliantName

	return nil
}

func getAssociatedDatastores(
	ctx context.Context,
	vimClient *vim25.Client,
	pbmClient *pbm.Client,
	profileID string,
	status *infrav1.StoragePolicyStatus) error {

	results, err := pbmClient.CheckRequirements(
		ctx,
		nil,
		nil,
		[]pbmtypes.BasePbmPlacementRequirement{
			&pbmtypes.PbmPlacementCapabilityProfileRequirement{
				ProfileId: pbmtypes.PbmProfileId{
					UniqueId: profileID,
				},
			},
		})
	if err != nil {
		return fmt.Errorf(
			"failed to check requirements for policy %q: %w",
			profileID, err)
	}

	var dsRefs []vimtypes.ManagedObjectReference
	for _, r := range results {
		if strings.EqualFold(
			r.Hub.HubType,
			string(vimtypes.ManagedObjectTypeDatastore)) {

			dsRefs = append(dsRefs, vimtypes.ManagedObjectReference{
				Type:  string(vimtypes.ManagedObjectTypeDatastore),
				Value: r.Hub.HubId,
			})
		}
	}

	var datastores []mo.Datastore
	status.Datastores = make([]infrav1.Datastore, len(dsRefs))
	pc := property.DefaultCollector(vimClient)
	if err := pc.Retrieve(
		ctx,
		dsRefs,
		[]string{"summary.type"},
		&datastores); err != nil {

		return fmt.Errorf(
			"failed to query datastore types for policy %q: %w",
			profileID, err)
	}
	for i, ds := range datastores {
		status.Datastores[i].ID.ObjectID = ds.Reference().Value
		status.Datastores[i].ID.ServerID = ds.Reference().ServerGUID
		switch vimtypes.HostFileSystemVolumeFileSystemType(ds.Summary.Type) {
		case vimtypes.HostFileSystemVolumeFileSystemTypeVsan,
			vimtypes.HostFileSystemVolumeFileSystemTypeVsanD,
			vimtypes.HostFileSystemVolumeFileSystemTypeVVOL:

			status.Datastores[i].Type = infrav1.DatastoreTypeVSAN

		default: // VMFS
			status.Datastores[i].Type = infrav1.DatastoreTypeVMFS
		}
	}
	return nil
}

func supportsEncryption(
	ctx context.Context,
	pbmClient *pbm.Client,
	profileID string,
	status *infrav1.StoragePolicyStatus) error {

	supportsEncryption, err := pbmClient.SupportsEncryption(ctx, profileID)
	if err != nil {
		if !strings.Contains(err.Error(), "Invalid profile ID") {
			return fmt.Errorf(
				"failed to determine if policy %q supports encryption: %w",
				profileID,
				err)
		}
	}
	status.Encrypted = supportsEncryption
	return nil
}

func getCapabilityProfileAndConstraints(
	ctx context.Context,
	pbmClient *pbm.Client,
	pid []pbmtypes.PbmProfileId,
	profileID string) (
	*pbmtypes.PbmCapabilityProfile,
	*pbmtypes.PbmCapabilitySubProfileConstraints,
	error) {

	profiles, err := pbmClient.RetrieveContent(ctx, pid)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"failed to retrieve content for profile %q: %w", profileID, err)
	}

	if len(profiles) == 0 {
		return nil, nil,
			fmt.Errorf("failed to find profile %q", profileID)
	}
	if len(profiles) > 1 {
		return nil, nil,
			fmt.Errorf("failed to find unique profile %q, count=%d",
				profileID,
				len(profiles))
	}

	p, ok := profiles[0].(*pbmtypes.PbmCapabilityProfile)
	if !ok {
		return nil, nil,
			fmt.Errorf("failed to assert PbmCapabilityProfile %q, type=%T",
				profileID,
				profiles[0])
	}

	c, ok := p.Constraints.(*pbmtypes.PbmCapabilitySubProfileConstraints)
	if !ok {
		return nil, nil,
			fmt.Errorf("failed to assert PbmCapabilitySubProfileConstraints %q, type=%T",
				profileID,
				p.Constraints)
	}
	return p, c, nil
}

func parseConstraints(
	c *pbmtypes.PbmCapabilitySubProfileConstraints,
	status *infrav1.StoragePolicyStatus) error {

	for _, sp := range c.SubProfiles {
		for _, cap := range sp.Capability {
			switch cap.Id.Namespace {
			case "com.vmware.storage.storageformat":
				if cap.Id.Id == "SectorSize" {
					for _, ci := range cap.Constraint {
						for _, pi := range ci.PropertyInstance {
							if pi.Id == "SectorSize" {
								switch pi.Value {
								case "512n", "512N":
									status.DiskFormat = infrav1.DiskFormat512n
								case "4kn", "4Kn", "4KN":
									status.DiskFormat = infrav1.DiskFormat4k
								default:
									return fmt.Errorf(
										"unexpected value for SectorSize %v",
										pi.Value)
								}
							}
						}
					}
				}
			case "com.vmware.storage.volumeallocation":
				if cap.Id.Id == "VolumeAllocationType" {
					for _, ci := range cap.Constraint {
						for _, pi := range ci.PropertyInstance {
							if pi.Id == "VolumeAllocationType" {
								switch pi.Value {
								case "Conserve space when possible":
									status.DiskProvisioningMode = infrav1.DiskProvisioningModeThin
								case "Reserve space":
									status.DiskProvisioningMode = infrav1.DiskProvisioningModeThick
								case "Fully initialized":
									status.DiskProvisioningMode = infrav1.DiskProvisioningModeThickEagerZero
								default:
									return fmt.Errorf(
										"unexpected value for VolumeAllocationType %v",
										pi.Value)
								}
							}
						}
					}
				}
			case "VSAN":
				if cap.Id.Id == "proportionalCapacity" {
					for _, ci := range cap.Constraint {
						for _, pi := range ci.PropertyInstance {
							if pi.Id == "proportionalCapacity" {
								v, ok := pi.Value.(int32)
								if !ok {
									return fmt.Errorf(
										"unexpected value for proportionalCapacity %[1]v, type=%[1]T",
										pi.Value)
								}
								switch v {
								case 0:
									status.DiskProvisioningMode = infrav1.DiskProvisioningModeThin
								case 100:
									status.DiskProvisioningMode = infrav1.DiskProvisioningModeThick
								default:
									return fmt.Errorf(
										"unexpected value for proportionalCapacity %v",
										pi.Value)
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}
