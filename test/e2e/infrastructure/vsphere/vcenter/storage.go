// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"errors"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/pbm"
	"github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

const (
	encryptionCapabilityID        = "ad5a249d-cbc2-43af-9366-694d7664fa52"
	encryptionCapabilityNamespace = "com.vmware.storageprofile.dataservice"
	vSANDirectTypeID              = "vSANDirectType"
	vSANDirect                    = "vSANDirect"
	volumeAllocationNamespace     = "com.vmware.storage.volumeallocation"
	volumeAllocationTypeID        = "VolumeAllocationType"
	fullyInitializedValue         = "Fully initialized"
)

// GetStoragePolicyIDFromName looks up a storage profile by name and returns its ID.
// Useful when configuring WCP namespaces with storage profiles.
func GetStoragePolicyIDFromName(client *vim25.Client, profileName string) (string, error) {
	pbmClient, err := pbm.NewClient(context.Background(), client)
	if err != nil {
		return "", err
	}

	return pbmClient.ProfileIDByName(context.Background(), profileName)
}

// GetOrCreateEncryptionStoragePolicy Gets already created Encryption Storage Policy ID or creates one if not found.
func GetOrCreateEncryptionStoragePolicy(ctx context.Context, client *vim25.Client, profileName, wcpProfileID string) (string, error) {
	pbmClient, err := pbm.NewClient(ctx, client)
	if err != nil {
		return "", err
	}

	policyID, err := pbmClient.ProfileIDByName(ctx, profileName)
	if err == nil {
		return policyID, nil
	}

	if !strings.Contains(err.Error(), "no pbm profile found") {
		return "", err
	}

	m, err := pbmClient.ProfileMap(ctx, wcpProfileID)
	if err != nil {
		return "", err
	}

	wcpProfile := m.Profile[0]

	createSpec, err := pbm.CreateCapabilityProfileSpec(pbm.CapabilityProfileCreateSpec{
		Name:           profileName,
		SubProfileName: "Host based services",
		Description:    "Encryption storage profile + " + wcpProfile.GetPbmProfile().Description,
		CapabilityList: []pbm.Capability{{
			ID:        encryptionCapabilityID,
			Namespace: encryptionCapabilityNamespace,
			PropertyList: []pbm.Property{{
				ID:       encryptionCapabilityID,
				Value:    encryptionCapabilityID, // Value is same as ID in this case
				DataType: "string",
			}},
		}},
		Category: string(types.PbmProfileCategoryEnumREQUIREMENT),
	})
	if err != nil {
		return "", err
	}

	// Add wcpProfileID's capabilities - tagged shared datastore (sharedVmfs-0 / vsanDatastore)
	// To see the result: govc storage.policy.info -dump "VM Service Encryption Policy"
	subProfile := &createSpec.Constraints.(*types.PbmCapabilitySubProfileConstraints).SubProfiles[0]
	if p, ok := wcpProfile.(*types.PbmCapabilityProfile); ok {
		if c, ok := p.Constraints.(*types.PbmCapabilitySubProfileConstraints); ok {
			subProfile.Capability = append(subProfile.Capability, c.SubProfiles[0].Capability...)
		}
	}

	profile, err := pbmClient.CreateProfile(ctx, *createSpec)
	if err != nil {
		return "", err
	}

	return profile.UniqueId, nil
}

// GetOrCreateEZTStoragePolicy Gets already created EZT (Eager Zeroed Thick)
// Storage Policy ID or creates one if not found. This creates a storage policy
// with VMFS "Fully initialized" volume allocation and inherits datastore placement
// tags from the base WCP profile.
func GetOrCreateEZTStoragePolicy(ctx context.Context, client *vim25.Client, profileName, wcpProfileID string) (string, error) {
	pbmClient, err := pbm.NewClient(ctx, client)
	if err != nil {
		return "", err
	}

	// Check if policy already exists.
	policyID, err := pbmClient.ProfileIDByName(ctx, profileName)
	if err == nil {
		return policyID, nil
	}

	// Check for not found error only then proceed for create.
	if !strings.Contains(err.Error(), "no pbm profile found") {
		return "", err
	}

	// Get the base WCP profile to inherit datastore tag/category capabilities.
	m, err := pbmClient.ProfileMap(ctx, wcpProfileID)
	if err != nil {
		return "", err
	}

	wcpProfile := m.Profile[0]

	// Create storage policy with VMFS "Fully initialized" (EZT) volume allocation.
	createSpec, err := pbm.CreateCapabilityProfileSpec(pbm.CapabilityProfileCreateSpec{
		Name:           profileName,
		SubProfileName: "VMFS rules",
		Description:    "EZT storage profile + " + wcpProfile.GetPbmProfile().Description,
		CapabilityList: []pbm.Capability{{
			ID:        volumeAllocationTypeID,
			Namespace: volumeAllocationNamespace,
			PropertyList: []pbm.Property{{
				ID:       volumeAllocationTypeID,
				Value:    fullyInitializedValue, // "Fully initialized" = Eager Zeroed Thick
				DataType: "string",
			}},
		}},
		Category: string(types.PbmProfileCategoryEnumREQUIREMENT),
	})
	if err != nil {
		return "", err
	}

	// Add wcpProfileID's capabilities - tagged shared datastore (for placement).
	// This inherits the tag/category from the base WCP profile
	// (e.g., wcpglobal_tag/wcpglobal_tag_category).
	subProfile := &createSpec.Constraints.(*types.PbmCapabilitySubProfileConstraints).SubProfiles[0]
	if p, ok := wcpProfile.(*types.PbmCapabilityProfile); ok {
		if c, ok := p.Constraints.(*types.PbmCapabilitySubProfileConstraints); ok {
			// Copy all capabilities from the base profile (includes datastore tags)
			subProfile.Capability = append(subProfile.Capability, c.SubProfiles[0].Capability...)
		}
	}

	profile, err := pbmClient.CreateProfile(ctx, *createSpec)
	if err != nil {
		return "", err
	}

	return profile.UniqueId, nil
}

// GetOrCreateWorkerStoragePolicy returns an existing vSphere profile ID for profileName, or creates
// a policy that duplicates the supervisor primary (WCP global) storage policy placement rules.
// After creation, WCPSVC syncs a Kubernetes StorageClass; callers should wait for that object.
// Pattern matches vks-gce2e EZT helper (duplicate base capabilities under a new name).
func GetOrCreateWorkerStoragePolicy(ctx context.Context, client *vim25.Client, profileName, wcpProfileID string) (string, error) {
	pbmClient, err := pbm.NewClient(ctx, client)
	if err != nil {
		return "", err
	}

	policyID, err := pbmClient.ProfileIDByName(ctx, profileName)
	if err == nil {
		return policyID, nil
	}

	if !strings.Contains(err.Error(), "no pbm profile found") {
		return "", err
	}

	m, err := pbmClient.ProfileMap(ctx, wcpProfileID)
	if err != nil {
		return "", err
	}

	wcpProfile := m.Profile[0]

	createSpec, err := pbm.CreateCapabilityProfileSpec(pbm.CapabilityProfileCreateSpec{
		Name:           profileName,
		SubProfileName: "Datastore placement",
		Description:    "Worker storage profile (clone of WCP global capabilities) — " + wcpProfile.GetPbmProfile().Description,
		CapabilityList: []pbm.Capability{},
		Category:       string(types.PbmProfileCategoryEnumREQUIREMENT),
	})
	if err != nil {
		return "", err
	}

	subProfile := &createSpec.Constraints.(*types.PbmCapabilitySubProfileConstraints).SubProfiles[0]
	if p, ok := wcpProfile.(*types.PbmCapabilityProfile); ok {
		if c, ok := p.Constraints.(*types.PbmCapabilitySubProfileConstraints); ok {
			subProfile.Capability = append(subProfile.Capability, c.SubProfiles[0].Capability...)
		}
	}

	if len(subProfile.Capability) == 0 {
		return "", errors.New("could not copy capability constraints from base WCP storage policy")
	}

	profile, err := pbmClient.CreateProfile(ctx, *createSpec)
	if err != nil {
		return "", err
	}

	return profile.UniqueId, nil
}

// GetOrCreateVsanDirectStoragePolicyID Gets already created VSAN Direct Storage Policy ID or Creates one if not found.
func GetOrCreateVsanDirectStoragePolicyID(ctx context.Context, client *vim25.Client, profileName string) (string, error) {
	pbmClient, err := pbm.NewClient(ctx, client)
	if err != nil {
		return "", err
	}

	policyID, err := pbmClient.ProfileIDByName(ctx, profileName)
	if err == nil {
		return policyID, nil
	}
	// Check for not found error only then proceed for create
	if !strings.Contains(err.Error(), "no pbm profile found") {
		return "", err
	}

	createSpec, err := pbm.CreateCapabilityProfileSpec(pbm.CapabilityProfileCreateSpec{
		Name:           profileName,
		SubProfileName: "Storage sub profile",
		Description:    "vSAN Direct Storage profile",
		CapabilityList: []pbm.Capability{{
			ID:        vSANDirectTypeID,
			Namespace: vSANDirect,
			PropertyList: []pbm.Property{{
				ID:       vSANDirectTypeID,
				Value:    vSANDirect,
				DataType: "string",
			}},
		}},
		Category: string(types.PbmProfileCategoryEnumREQUIREMENT),
	})
	if err != nil {
		return "", err
	}

	profile, err := pbmClient.CreateProfile(ctx, *createSpec)
	if err != nil {
		return "", err
	}

	return profile.UniqueId, nil
}

// IsVSANDEnabledCluster checks if given wcp enabled cluster is enabled with vsand capability.
func IsVSANDEnabledCluster(ctx context.Context, client *vim25.Client, kubeconfigPath string) (bool, error) {
	clusterMOID := GetClusterMoIDFromKubeconfig(ctx, kubeconfigPath)
	if clusterMOID == "" {
		return false, errors.New("could not fetch cluster moid from wcp cluster config")
	}

	cluster := object.NewClusterComputeResource(
		client,
		vimtypes.ManagedObjectReference{
			Type:  "ClusterComputeResource",
			Value: clusterMOID,
		},
	)

	return clusterConfiguredWithVsand(ctx, cluster)
}

func clusterConfiguredWithVsand(ctx context.Context, cluster *object.ClusterComputeResource) (bool, error) {
	var cr mo.ComputeResource

	err := cluster.Properties(ctx, cluster.Reference(), []string{"datastore"}, &cr)
	if err != nil {
		return false, err
	}

	if len(cr.Datastore) == 0 {
		return false, errors.New("no datastores in cluster")
	}

	var datastores []mo.Datastore

	pc := property.DefaultCollector(cluster.Client())

	err = pc.Retrieve(ctx, cr.Datastore, []string{"summary"}, &datastores)
	if err != nil {
		return false, err
	}

	for _, d := range datastores {
		if d.Summary.Type == "vsanD" {
			return true, nil
		}
	}

	return false, nil
}
