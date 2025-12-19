// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storage_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/pbm"
	pbmmethods "github.com/vmware/govmomi/pbm/methods"
	pbmsim "github.com/vmware/govmomi/pbm/simulator"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	storutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/storage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = storutil.GetStoragePolicyStatus

var _ = Describe("GetStoragePolicyStatus", func() {
	var (
		ctx        context.Context
		vcSimCtx   *builder.TestContextForVCSim
		k8sClient  ctrlclient.Client
		vimClient  *vim25.Client
		pbmClient  *pbm.Client
		withObjs   []ctrlclient.Object
		withFuncs  interceptor.Funcs
		profileID  string
		profileMap pbmProfileMap
	)

	BeforeEach(func() {
		vcSimCtx = builder.NewTestContextForVCSim(
			ctxop.WithContext(pkgcfg.NewContextWithDefaultConfig()),
			builder.VCSimTestConfig{
				NumDatastores: 3,
			})
		ctx = vcSimCtx
		ctx = vmconfig.WithContext(ctx)
		ctx = pkgctx.WithRestClient(ctx, vcSimCtx.RestClient)

		withFuncs = interceptor.Funcs{}
		withObjs = []ctrlclient.Object{}

		vimClient = vcSimCtx.VCClient.Client

		{
			var err error
			pbmClient, err = pbm.NewClient(ctx, vimClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(pbmClient).ToNot(BeNil())
		}

		Expect(createProfiles(ctx, pbmClient)).To(Succeed())

		{
			var err error
			profileMap, err = getProfileMap(ctx, pbmClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(profileMap).ToNot(BeZero())
		}

		Expect(len(profileMap.Profile)).To(BeNumerically(">=", len(profiles)))
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(
			withFuncs,
			withObjs...)
	})

	AfterEach(func() {
		vcSimCtx.AfterEach()
		vcSimCtx = nil
	})

	When("it should panic", func() {
		var (
			fn func()
		)

		JustBeforeEach(func() {
			fn = func() {
				_, _ = storutil.GetStoragePolicyStatus(
					ctx,
					k8sClient,
					vimClient,
					pbmClient,
					profileID)
			}
		})

		When("ctx is nil", func() {
			JustBeforeEach(func() {
				ctx = nil
			})
			It("panics", func() {
				Expect(fn).To(PanicWith("ctx is nil"))
			})
		})
		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				Expect(fn).To(PanicWith("k8sClient is nil"))
			})
		})
		When("vimClient is nil", func() {
			JustBeforeEach(func() {
				vimClient = nil
			})
			It("should panic", func() {
				Expect(fn).To(PanicWith("vimClient is nil"))
			})
		})
		When("pbmClient is nil", func() {
			JustBeforeEach(func() {
				pbmClient = nil
			})
			It("should panic", func() {
				Expect(fn).To(PanicWith("pbmClient is nil"))
			})
		})
		When("profileID is empty", func() {
			JustBeforeEach(func() {
				profileID = ""
			})
			It("should panic", func() {
				Expect(fn).To(PanicWith("profileID is empty"))
			})
		})
	})

	When("it should not panic", func() {
		var (
			err    error
			status infrav1.StoragePolicyStatus

			numDatastores int

			datastore1Ref     vimtypes.ManagedObjectReference
			datastore1Type    vimtypes.HostFileSystemVolumeFileSystemType
			datastore1K8sType infrav1.DatastoreType

			datastore2Ref     vimtypes.ManagedObjectReference
			datastore2Type    vimtypes.HostFileSystemVolumeFileSystemType
			datastore2K8sType infrav1.DatastoreType
		)

		BeforeEach(func() {
			numDatastores = 1

			datastore1Type = vimtypes.HostFileSystemVolumeFileSystemTypeVMFS
			datastore1K8sType = infrav1.DatastoreTypeVMFS

			datastore2Type = vimtypes.HostFileSystemVolumeFileSystemTypeVMFS
			datastore2K8sType = infrav1.DatastoreTypeVMFS
		})

		JustBeforeEach(func() {
			datastores := vcSimCtx.SimulatorContext().Map.All(
				string(vimtypes.ManagedObjectTypeDatastore))
			for i := range datastores {
				ds := datastores[i].(*simulator.Datastore)
				switch i {
				case 0:
					datastore1Ref = ds.Reference()
					ds.Summary.Type = string(datastore1Type)
				case 1:
					datastore2Ref = ds.Reference()
					ds.Summary.Type = string(datastore2Type)
				}
			}
			Expect(datastore1Ref).ToNot(BeZero())
			Expect(datastore2Ref).ToNot(BeZero())

			vcSimCtx.SimulatorContext().For("/pbm").Map.Handler = func(
				ctx *simulator.Context,
				m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

				if m.Name == "PbmCheckRequirements" {

					r := []pbmtypes.PbmPlacementCompatibilityResult{
						{
							Hub: pbmtypes.PbmPlacementHub{
								HubType: string(vimtypes.ManagedObjectTypeDatastore),
								HubId:   datastore1Ref.Value,
							},
						},
					}
					if numDatastores > 1 {
						r = append(r, pbmtypes.PbmPlacementCompatibilityResult{
							Hub: pbmtypes.PbmPlacementHub{
								HubType: string(vimtypes.ManagedObjectTypeDatastore),
								HubId:   datastore2Ref.Value,
							},
						})
					}

					return &fakePlacementSolver{
						PlacementSolver: &pbmsim.PlacementSolver{},
						Result:          r,
					}, nil
				}

				return nil, nil
			}

			status, err = storutil.GetStoragePolicyStatus(
				ctx,
				k8sClient,
				vimClient,
				pbmClient,
				profileID)
		})

		When("the profileID references a non-existent policy", func() {
			BeforeEach(func() {
				profileID = "fake"
			})
			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(`failed to retrieve content for profile "fake"`))
				Expect(status).To(BeZero())
			})
		})

		When("the profileID references an existing policy", func() {
			When("the profileID references a 512n policy", func() {
				BeforeEach(func() {
					profileID = profileMap.namesToID["sector-format-512"]

					withObjs = append(withObjs,
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "sector-format-512",
							},
						},
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "sector-format-512-latebinding",
							},
						},
					)
				})
				JustBeforeEach(func() {
					Expect(profileID).ToNot(BeEmpty())
				})
				It("should return the policy status", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(status).ToNot(BeZero())

					Expect(status.StorageClasses).To(HaveLen(2))
					Expect(status.StorageClasses[0]).To(Equal("sector-format-512"))
					Expect(status.StorageClasses[1]).To(Equal("sector-format-512-latebinding"))
					Expect(status.Datastores).To(HaveLen(1))
					Expect(status.Datastores[0].ID.ObjectID).To(Equal(datastore1Ref.Value))
					Expect(status.Datastores[0].ID.ServerID).To(Equal(datastore1Ref.ServerGUID))
					Expect(status.Datastores[0].Type).To(Equal(datastore1K8sType))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(Equal(infrav1.DiskFormat512n))
					Expect(status.DiskProvisioningMode).To(BeEmpty())
				})
			})

			When("the profileID references a 4k policy", func() {
				BeforeEach(func() {
					profileID = profileMap.namesToID["sector-format-4k"]

					withObjs = append(withObjs,
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "sector-format-4k",
							},
						},
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "sector-format-4k-latebinding",
							},
						},
					)
				})
				JustBeforeEach(func() {
					Expect(profileID).ToNot(BeEmpty())
				})
				It("should return the policy status", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(status).ToNot(BeZero())

					Expect(status.StorageClasses).To(HaveLen(2))
					Expect(status.StorageClasses[0]).To(Equal("sector-format-4k"))
					Expect(status.StorageClasses[1]).To(Equal("sector-format-4k-latebinding"))
					Expect(status.Datastores).To(HaveLen(1))
					Expect(status.Datastores[0].ID.ObjectID).To(Equal(datastore1Ref.Value))
					Expect(status.Datastores[0].ID.ServerID).To(Equal(datastore1Ref.ServerGUID))
					Expect(status.Datastores[0].Type).To(Equal(datastore1K8sType))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(Equal(infrav1.DiskFormat4k))
					Expect(status.DiskProvisioningMode).To(BeEmpty())
				})
			})

			When("the profileID references a VMFS thin policy", func() {
				BeforeEach(func() {
					profileID = profileMap.namesToID["vmfs-provisioning-thin"]

					withObjs = append(withObjs,
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vmfs-provisioning-thin",
							},
						},
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vmfs-provisioning-thin-latebinding",
							},
						},
					)
				})
				JustBeforeEach(func() {
					Expect(profileID).ToNot(BeEmpty())
				})
				It("should return the policy status", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(status).ToNot(BeZero())

					Expect(status.StorageClasses).To(HaveLen(2))
					Expect(status.StorageClasses[0]).To(Equal("vmfs-provisioning-thin"))
					Expect(status.StorageClasses[1]).To(Equal("vmfs-provisioning-thin-latebinding"))
					Expect(status.Datastores).To(HaveLen(1))
					Expect(status.Datastores[0].ID.ObjectID).To(Equal(datastore1Ref.Value))
					Expect(status.Datastores[0].ID.ServerID).To(Equal(datastore1Ref.ServerGUID))
					Expect(status.Datastores[0].Type).To(Equal(datastore1K8sType))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(BeEmpty())
					Expect(status.DiskProvisioningMode).To(Equal(infrav1.DiskProvisioningModeThin))
				})
			})

			When("the profileID references a VMFS thick policy", func() {
				BeforeEach(func() {
					profileID = profileMap.namesToID["vmfs-provisioning-thick"]

					withObjs = append(withObjs,
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vmfs-provisioning-thick",
							},
						},
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vmfs-provisioning-thick-latebinding",
							},
						},
					)
				})
				JustBeforeEach(func() {
					Expect(profileID).ToNot(BeEmpty())
				})
				It("should return the policy status", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(status).ToNot(BeZero())

					Expect(status.StorageClasses).To(HaveLen(2))
					Expect(status.StorageClasses[0]).To(Equal("vmfs-provisioning-thick"))
					Expect(status.StorageClasses[1]).To(Equal("vmfs-provisioning-thick-latebinding"))
					Expect(status.Datastores).To(HaveLen(1))
					Expect(status.Datastores[0].ID.ObjectID).To(Equal(datastore1Ref.Value))
					Expect(status.Datastores[0].ID.ServerID).To(Equal(datastore1Ref.ServerGUID))
					Expect(status.Datastores[0].Type).To(Equal(datastore1K8sType))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(BeEmpty())
					Expect(status.DiskProvisioningMode).To(Equal(infrav1.DiskProvisioningModeThick))
				})
			})

			When("the profileID references a VMFS thick-eager-zero policy", func() {
				BeforeEach(func() {
					profileID = profileMap.namesToID["vmfs-provisioning-thick-eager-zero"]

					withObjs = append(withObjs,
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vmfs-provisioning-thick-eager-zero",
							},
						},
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vmfs-provisioning-thick-eager-zero-latebinding",
							},
						},
					)
				})
				JustBeforeEach(func() {
					Expect(profileID).ToNot(BeEmpty())
				})
				It("should return the policy status", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(status).ToNot(BeZero())

					Expect(status.StorageClasses).To(HaveLen(2))
					Expect(status.StorageClasses[0]).To(Equal("vmfs-provisioning-thick-eager-zero"))
					Expect(status.StorageClasses[1]).To(Equal("vmfs-provisioning-thick-eager-zero-latebinding"))
					Expect(status.Datastores).To(HaveLen(1))
					Expect(status.Datastores[0].ID.ObjectID).To(Equal(datastore1Ref.Value))
					Expect(status.Datastores[0].ID.ServerID).To(Equal(datastore1Ref.ServerGUID))
					Expect(status.Datastores[0].Type).To(Equal(datastore1K8sType))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(BeEmpty())
					Expect(status.DiskProvisioningMode).To(Equal(infrav1.DiskProvisioningModeThickEagerZero))
				})
			})

			When("the profileID references a VSAN thin policy", func() {
				BeforeEach(func() {
					datastore1Type = vimtypes.HostFileSystemVolumeFileSystemTypeVsan
					datastore1K8sType = infrav1.DatastoreTypeVSAN

					profileID = profileMap.namesToID["vsan-provisioning-thin"]

					withObjs = append(withObjs,
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vsan-provisioning-thin",
							},
						},
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vsan-provisioning-thin-latebinding",
							},
						},
					)
				})
				JustBeforeEach(func() {
					Expect(profileID).ToNot(BeEmpty())
				})
				It("should return the policy status", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(status).ToNot(BeZero())

					Expect(status.StorageClasses).To(HaveLen(2))
					Expect(status.StorageClasses[0]).To(Equal("vsan-provisioning-thin"))
					Expect(status.StorageClasses[1]).To(Equal("vsan-provisioning-thin-latebinding"))
					Expect(status.Datastores).To(HaveLen(1))
					Expect(status.Datastores[0].ID.ObjectID).To(Equal(datastore1Ref.Value))
					Expect(status.Datastores[0].ID.ServerID).To(Equal(datastore1Ref.ServerGUID))
					Expect(status.Datastores[0].Type).To(Equal(datastore1K8sType))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(BeEmpty())
					Expect(status.DiskProvisioningMode).To(Equal(infrav1.DiskProvisioningModeThin))
				})
			})

			When("the profileID references a VSAN thick policy", func() {
				BeforeEach(func() {
					datastore1Type = vimtypes.HostFileSystemVolumeFileSystemTypeVsanD
					datastore1K8sType = infrav1.DatastoreTypeVSAN

					profileID = profileMap.namesToID["vsan-provisioning-thick"]

					withObjs = append(withObjs,
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vsan-provisioning-thick",
							},
						},
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vsan-provisioning-thick-latebinding",
							},
						},
					)
				})
				JustBeforeEach(func() {
					Expect(profileID).ToNot(BeEmpty())
				})
				It("should return the policy status", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(status).ToNot(BeZero())

					Expect(status.StorageClasses).To(HaveLen(2))
					Expect(status.StorageClasses[0]).To(Equal("vsan-provisioning-thick"))
					Expect(status.StorageClasses[1]).To(Equal("vsan-provisioning-thick-latebinding"))
					Expect(status.Datastores).To(HaveLen(1))
					Expect(status.Datastores[0].ID.ObjectID).To(Equal(datastore1Ref.Value))
					Expect(status.Datastores[0].ID.ServerID).To(Equal(datastore1Ref.ServerGUID))
					Expect(status.Datastores[0].Type).To(Equal(datastore1K8sType))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(BeEmpty())
					Expect(status.DiskProvisioningMode).To(Equal(infrav1.DiskProvisioningModeThick))
				})
			})

			When("the profileID references an encrypted policy", func() {
				BeforeEach(func() {
					profileID = pbmsim.DefaultEncryptionProfileID

					withObjs = append(withObjs,
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vm-encryption-policy",
							},
						},
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vm-encryption-policy-latebinding",
							},
						},
					)
				})
				JustBeforeEach(func() {
					Expect(profileID).ToNot(BeEmpty())
				})
				It("should return the policy status", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(status).ToNot(BeZero())

					Expect(status.StorageClasses).To(HaveLen(2))
					Expect(status.StorageClasses[0]).To(Equal("vm-encryption-policy"))
					Expect(status.StorageClasses[1]).To(Equal("vm-encryption-policy-latebinding"))
					Expect(status.Datastores).To(HaveLen(1))
					Expect(status.Datastores[0].ID.ObjectID).To(Equal(datastore1Ref.Value))
					Expect(status.Datastores[0].ID.ServerID).To(Equal(datastore1Ref.ServerGUID))
					Expect(status.Datastores[0].Type).To(Equal(datastore1K8sType))
					Expect(status.Encrypted).To(BeTrue())
				})

				Context("backed by two datastores", func() {
					BeforeEach(func() {
						numDatastores = 2
					})
					It("should return the policy status", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(status).ToNot(BeZero())

						Expect(status.StorageClasses).To(HaveLen(2))
						Expect(status.StorageClasses[0]).To(Equal("vm-encryption-policy"))
						Expect(status.StorageClasses[1]).To(Equal("vm-encryption-policy-latebinding"))
						Expect(status.Datastores).To(ConsistOf(
							infrav1.Datastore{
								ID: infrav1.ManagedObjectID{
									ObjectID: datastore1Ref.Value,
									ServerID: datastore1Ref.ServerGUID,
								},
								Type: datastore1K8sType,
							},
							infrav1.Datastore{
								ID: infrav1.ManagedObjectID{
									ObjectID: datastore2Ref.Value,
									ServerID: datastore2Ref.ServerGUID,
								},
								Type: datastore2K8sType,
							},
						))
						Expect(status.Encrypted).To(BeTrue())
					})
				})
			})
		})
	})
})

type fakePlacementSolver struct {
	*pbmsim.PlacementSolver
	Result []pbmtypes.PbmPlacementCompatibilityResult
}

func (m *fakePlacementSolver) PbmCheckRequirements(
	req *pbmtypes.PbmCheckRequirements) soap.HasFault {

	body := new(pbmmethods.PbmCheckRequirementsBody)
	body.Res = new(pbmtypes.PbmCheckRequirementsResponse)
	body.Res.Returnval = m.Result

	return body
}
