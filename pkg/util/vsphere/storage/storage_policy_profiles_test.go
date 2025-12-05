// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storage_test

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/pbm"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

func createProfiles(
	ctx context.Context,
	pbmClient *pbm.Client) error {

	for _, p := range profiles {
		if _, err := pbmClient.CreateProfile(ctx, p); err != nil {
			return fmt.Errorf("failed to create profile %s: %w", p.Name, err)
		}
	}
	return nil
}

type pbmProfileMap struct {
	*pbm.ProfileMap
	namesToID map[string]string
}

func getProfileMap(
	ctx context.Context,
	pbmClient *pbm.Client) (pbmProfileMap, error) {

	ipm, err := pbmClient.ProfileMap(ctx)
	if err != nil {
		return pbmProfileMap{}, err
	}

	pm := pbmProfileMap{
		ProfileMap: ipm,
		namesToID:  map[string]string{},
	}

	for _, p := range pm.Profile {
		if cp, ok := p.(*pbmtypes.PbmCapabilityProfile); ok {
			pm.namesToID[cp.Name] = cp.ProfileId.UniqueId
		}
	}

	return pm, nil
}

var profiles = []pbmtypes.PbmCapabilityProfileCreateSpec{
	{
		Name:        "sector-format-512",
		Description: "",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "Storage format",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "com.vmware.storage.storageformat",
								Id:        "SectorSize",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "SectorSize",
											Value: "512n",
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category:         "REQUIREMENT",
		K8sCompliantName: "sector-format-512",
	},
	{
		Name:        "sector-format-4k",
		Description: "",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "Storage format",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "com.vmware.storage.storageformat",
								Id:        "SectorSize",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "SectorSize",
											Value: "4Kn",
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category:         "REQUIREMENT",
		K8sCompliantName: "sector-format-4k",
	},
	{
		Name:        "vmfs-provisioning-thin",
		Description: "",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "VMFS rules",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "com.vmware.storage.volumeallocation",
								Id:        "VolumeAllocationType",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "VolumeAllocationType",
											Value: "Conserve space when possible",
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category:         "REQUIREMENT",
		K8sCompliantName: "vmfs-provisioning-thin",
	},
	{
		Name:        "vmfs-provisioning-thick",
		Description: "",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "VMFS rules",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "com.vmware.storage.volumeallocation",
								Id:        "VolumeAllocationType",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "VolumeAllocationType",
											Value: "Reserve space",
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category:         "REQUIREMENT",
		K8sCompliantName: "vmfs-provisioning-thick",
	},
	{
		Name:        "vmfs-provisioning-thick-eager-zero",
		Description: "",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "VMFS rules",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "com.vmware.storage.volumeallocation",
								Id:        "VolumeAllocationType",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "VolumeAllocationType",
											Value: "Fully initialized",
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category:         "REQUIREMENT",
		K8sCompliantName: "vmfs-provisioning-thick-eager-zero",
	},
	{
		Name:        "vsan-provisioning-thin",
		Description: "",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "VSAN",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "autoManagedRAID",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "autoManagedRAID",
											Value: true,
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "iopsLimit",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "iopsLimit",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(0),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category:         "REQUIREMENT",
		K8sCompliantName: "vsan-provisioning-thin",
	},
	{
		Name:        "vsan-provisioning-thick",
		Description: "",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "VSAN",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "autoManagedRAID",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "autoManagedRAID",
											Value: true,
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "iopsLimit",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "iopsLimit",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(100),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category:         "REQUIREMENT",
		K8sCompliantName: "vsan-provisioning-thick",
	},
	{
		Name:        "Host-local PMem Default Storage Policy",
		Description: "Storage policy used as default for Host-local PMem datastores",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "PMem sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "PMem",
								Id:        "PMemType",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "PMemType",
											Value: "LocalPMem",
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "Management Storage Policy - Large",
		Description: "Management Storage policy used for VMC large cluster",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "vSAN VMC Large sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(2),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-1 (Mirroring) - Performance",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(100),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "Management Storage Policy - Regular",
		Description: "Management Storage policy used for VMC regular cluster",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "vSAN VMC Small sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-1 (Mirroring) - Performance",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(100),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "Management Storage Policy - Single Node",
		Description: "Management Storage policy used for VMC single node cluster",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "vSAN VMC single node sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(100),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "Management Storage Policy - Stretched",
		Description: "Management Storage policy used for VMC stretched cluster",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "vSAN VMC Stretched sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "subFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "subFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-1 (Mirroring) - Performance",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(100),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "Management Storage Policy - Stretched ESA",
		Description: "Management Storage policy used for ESA stretched cluster",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "vSAN VMC Stretched ESA sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "subFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "subFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "stripeWidth",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "stripeWidth",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "forceProvisioning",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "forceProvisioning",
											Value: false,
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-5/6 (Erasure Coding) - Capacity",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(100),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "storageType",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "storageType",
											Value: "Allflash",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "cacheReservation",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "cacheReservation",
											Value: int32(0),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "Management Storage Policy - Stretched Lite",
		Description: "Management Storage policy used for smaller VMC Stretched Cluster configuration.",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "vSAN VMC Stretched Lite sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "subFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "subFailuresToTolerate",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "stripeWidth",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "stripeWidth",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "forceProvisioning",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "forceProvisioning",
											Value: false,
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-1 (Mirroring) - Performance",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(100),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "Management Storage policy - Encryption",
		Description: "Management Storage policy used for encrypting VM",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "vSAN VMC Small sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-1 (Mirroring) - Performance",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(100),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "com.vmware.storageprofile.dataservice",
								Id:        "ad5a249d-cbc2-43af-9366-694d7664fa52",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "ad5a249d-cbc2-43af-9366-694d7664fa52",
											Value: "ad5a249d-cbc2-43af-9366-694d7664fa52",
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "Management Storage policy - Thin",
		Description: "Management Storage policy used for VMC regular cluster which requires THIN provisioning",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "vSAN VMC Small sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-1 (Mirroring) - Performance",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(0),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "New VM Storage Policy",
		Description: "",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "VSAN",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "autoManagedRAID",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "autoManagedRAID",
											Value: true,
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "iopsLimit",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "iopsLimit",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(100),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "VM Encryption Policy",
		Description: "Sample storage policy for VMware's VM and virtual disk encryption",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "sp-1",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "com.vmware.storageprofile.dataservice",
								Id:        "ad5a249d-cbc2-43af-9366-694d7664fa52",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "ad5a249d-cbc2-43af-9366-694d7664fa52",
											Value: "ad5a249d-cbc2-43af-9366-694d7664fa52",
										},
									},
								},
							},
						},
					},
					ForceProvision: vimtypes.NewBool(false),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "vSAN Default Storage Policy",
		Description: "Storage policy used as default for vSAN datastores",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "VSAN sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "stripeWidth",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "stripeWidth",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "forceProvisioning",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "forceProvisioning",
											Value: false,
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "cacheReservation",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "cacheReservation",
											Value: int32(0),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "vSAN ESA Auto RAID Policy",
		Description: "Default vSAN ESA auto raid storage policy for vSAN datastore",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "VSAN sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "autoManagedRAID",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "autoManagedRAID",
											Value: true,
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "vSAN ESA Default Policy - RAID5",
		Description: "Default vSAN ESA RAID5 storage policy for vSAN datastore.",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "VSAN sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "stripeWidth",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "stripeWidth",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "forceProvisioning",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "forceProvisioning",
											Value: false,
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "cacheReservation",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "cacheReservation",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-5/6 (Erasure Coding) - Capacity",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "storageType",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "storageType",
											Value: "Allflash",
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "vSAN ESA Default Policy - RAID6",
		Description: "Default vSAN ESA RAID6 storage policy for vSAN datastore.",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "VSAN sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(2),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "stripeWidth",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "stripeWidth",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "forceProvisioning",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "forceProvisioning",
											Value: false,
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "cacheReservation",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "cacheReservation",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-5/6 (Erasure Coding) - Capacity",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "storageType",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "storageType",
											Value: "Allflash",
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "vSAN Stretched ESA Default Policy",
		Description: "Default vSAN Stretched ESA RAID1 storage policy for vSAN datastore.",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "vSAN Stretched ESA sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "subFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "subFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "stripeWidth",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "stripeWidth",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "forceProvisioning",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "forceProvisioning",
											Value: false,
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-1 (Mirroring) - Performance",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "storageType",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "storageType",
											Value: "Allflash",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "cacheReservation",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "cacheReservation",
											Value: int32(0),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "vSAN Stretched ESA Default Policy - RAID5",
		Description: "Default vSAN Stretched ESA RAID5 storage policy for vSAN datastore.",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "vSAN Stretched ESA sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "subFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "subFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "stripeWidth",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "stripeWidth",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "forceProvisioning",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "forceProvisioning",
											Value: false,
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-5/6 (Erasure Coding) - Capacity",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "storageType",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "storageType",
											Value: "Allflash",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "cacheReservation",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "cacheReservation",
											Value: int32(0),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "vSAN Stretched ESA Default Policy - RAID6",
		Description: "Default vSAN Stretched ESA RAID6 storage policy for vSAN datastore.",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "vSAN Stretched ESA sub-profile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "hostFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "hostFailuresToTolerate",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "subFailuresToTolerate",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "subFailuresToTolerate",
											Value: int32(2),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "stripeWidth",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "stripeWidth",
											Value: int32(1),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "forceProvisioning",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "forceProvisioning",
											Value: false,
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "replicaPreference",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "replicaPreference",
											Value: "RAID-5/6 (Erasure Coding) - Capacity",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "proportionalCapacity",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "proportionalCapacity",
											Value: int32(0),
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "storageType",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "storageType",
											Value: "Allflash",
										},
									},
								},
							},
						},
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "VSAN",
								Id:        "cacheReservation",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id:    "cacheReservation",
											Value: int32(0),
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
	{
		Name:        "VVol No Requirements Policy",
		Description: "Allow the datastore to determine the best placement strategy for storage objects",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilityConstraints{},
		Category:    "REQUIREMENT",
	},
	{
		Name:        "wcpglobal_storage_profile",
		Description: "wcp global profile",
		ResourceType: pbmtypes.PbmProfileResourceType{
			ResourceType: "STORAGE",
		},
		Constraints: &pbmtypes.PbmCapabilitySubProfileConstraints{
			PbmCapabilityConstraints: pbmtypes.PbmCapabilityConstraints{},
			SubProfiles: []pbmtypes.PbmCapabilitySubProfile{
				{
					Name: "tag subprofile",
					Capability: []pbmtypes.PbmCapabilityInstance{
						{
							Id: pbmtypes.PbmCapabilityMetadataUniqueId{
								Namespace: "http://www.vmware.com/storage/tag",
								Id:        "wcpglobal_tag_category",
							},
							Constraint: []pbmtypes.PbmCapabilityConstraintInstance{
								{
									PropertyInstance: []pbmtypes.PbmCapabilityPropertyInstance{
										{
											Id: "com.vmware.storage.tag.wcpglobal_tag_category.property",
											Value: &pbmtypes.PbmCapabilityDiscreteSet{
												Values: []vimtypes.AnyType{"wcpglobal_tag"},
											},
										},
									},
								},
							},
						},
					},
					ForceProvision: (*bool)(nil),
				},
			},
		},
		Category: "REQUIREMENT",
	},
}
