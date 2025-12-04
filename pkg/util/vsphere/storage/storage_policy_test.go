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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
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
			builder.VCSimTestConfig{})
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
			err           error
			status        vmopv1.StoragePolicyStatus
			datastoreRef  vimtypes.ManagedObjectReference
			datastoreType vimtypes.HostFileSystemVolumeFileSystemType
		)

		BeforeEach(func() {
			datastoreType = vimtypes.HostFileSystemVolumeFileSystemTypeVMFS
		})

		JustBeforeEach(func() {
			datastore := vcSimCtx.SimulatorContext().Map.Any(
				string(vimtypes.ManagedObjectTypeDatastore)).(*simulator.Datastore)
			datastoreRef = datastore.Reference()
			vcSimCtx.SimulatorContext().Map.Update(
				vcSimCtx.SimulatorContext(),
				datastore,
				[]vimtypes.PropertyChange{
					{
						Name: "summary.type",
						Op:   vimtypes.PropertyChangeOpAssign,
						Val:  string(datastoreType),
					},
				})
			Expect(datastoreRef).ToNot(BeZero())

			vcSimCtx.SimulatorContext().For("/pbm").Map.Handler = func(
				ctx *simulator.Context,
				m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

				if m.Name == "PbmQueryAssociatedEntities" {
					return &fakeProfileManager{
						ProfileManager: &pbmsim.ProfileManager{},
						Result: []pbmtypes.PbmQueryProfileResult{
							{
								Object: pbmtypes.PbmServerObjectRef{
									ObjectType: string(pbmtypes.PbmObjectTypeDatastore),
									Key:        datastoreRef.Value,
									ServerUuid: datastoreRef.ServerGUID,
								},
							},
						},
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
					Expect(status.DatastoreTypes).To(HaveLen(1))
					Expect(status.DatastoreTypes[0]).To(Equal(vmopv1.DatastoreTypeVMFS))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(Equal(vmopv1.DiskFormat512n))
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
					Expect(status.DatastoreTypes).To(HaveLen(1))
					Expect(status.DatastoreTypes[0]).To(Equal(vmopv1.DatastoreTypeVMFS))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(Equal(vmopv1.DiskFormat4k))
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
					Expect(status.DatastoreTypes).To(HaveLen(1))
					Expect(status.DatastoreTypes[0]).To(Equal(vmopv1.DatastoreTypeVMFS))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(BeEmpty())
					Expect(status.DiskProvisioningMode).To(Equal(vmopv1.DiskProvisioningModeThin))
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
					Expect(status.DatastoreTypes).To(HaveLen(1))
					Expect(status.DatastoreTypes[0]).To(Equal(vmopv1.DatastoreTypeVMFS))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(BeEmpty())
					Expect(status.DiskProvisioningMode).To(Equal(vmopv1.DiskProvisioningModeThick))
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
					Expect(status.DatastoreTypes).To(HaveLen(1))
					Expect(status.DatastoreTypes[0]).To(Equal(vmopv1.DatastoreTypeVMFS))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(BeEmpty())
					Expect(status.DiskProvisioningMode).To(Equal(vmopv1.DiskProvisioningModeThickEagerZero))
				})
			})

			When("the profileID references a VSAN thin policy", func() {
				BeforeEach(func() {
					datastoreType = vimtypes.HostFileSystemVolumeFileSystemTypeVsan

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
					Expect(status.DatastoreTypes).To(HaveLen(1))
					Expect(status.DatastoreTypes[0]).To(Equal(vmopv1.DatastoreTypeVSAN))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(BeEmpty())
					Expect(status.DiskProvisioningMode).To(Equal(vmopv1.DiskProvisioningModeThin))
				})
			})

			When("the profileID references a VSAN thick policy", func() {
				BeforeEach(func() {
					datastoreType = vimtypes.HostFileSystemVolumeFileSystemTypeVsanD

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
					Expect(status.DatastoreTypes).To(HaveLen(1))
					Expect(status.DatastoreTypes[0]).To(Equal(vmopv1.DatastoreTypeVSAN))
					Expect(status.Encrypted).To(BeFalse())
					Expect(status.DiskFormat).To(BeEmpty())
					Expect(status.DiskProvisioningMode).To(Equal(vmopv1.DiskProvisioningModeThick))
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
					Expect(status.DatastoreTypes).To(HaveLen(1))
					Expect(status.DatastoreTypes[0]).To(Equal(vmopv1.DatastoreTypeVMFS))
					Expect(status.Encrypted).To(BeTrue())
				})
			})
		})
	})
})

type fakeProfileManager struct {
	*pbmsim.ProfileManager
	Result []pbmtypes.PbmQueryProfileResult
}

func (m *fakeProfileManager) PbmQueryAssociatedEntities(
	req *pbmtypes.PbmQueryAssociatedEntities) soap.HasFault {

	body := new(pbmmethods.PbmQueryAssociatedEntitiesBody)
	body.Res = new(pbmtypes.PbmQueryAssociatedEntitiesResponse)
	body.Res.Returnval = m.Result

	return body
}
