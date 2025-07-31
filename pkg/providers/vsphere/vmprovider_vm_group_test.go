// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmGroupTests() {

	var (
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		vm1     *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
		vmGroup *vmopv1.VirtualMachineGroup
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{
			WithContentLibrary: true,
		}

		vm1 = builder.DummyBasicVirtualMachine("group-placement-vm-1", "")
		vmClass = builder.DummyVirtualMachineClassGenName()

		vmGroup = &vmopv1.VirtualMachineGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vm-group-test",
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.FastDeploy = true
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())
		Expect(ctx.Client.Status().Update(ctx, vmClass)).To(Succeed())

		vm1.Namespace = nsInfo.Namespace
		vm1.Spec.ClassName = vmClass.Name
		vm1.Spec.ImageName = ctx.ContentLibraryImageName
		vm1.Spec.Image.Kind = cvmiKind
		vm1.Spec.Image.Name = ctx.ContentLibraryImageName
		vm1.Spec.StorageClass = ctx.StorageClassName
		vm1.Spec.GroupName = vmGroup.Name

		vmGroup.Namespace = nsInfo.Namespace

		{
			// TODO: Put this test builder to reduce duplication.

			vmic := vmopv1.VirtualMachineImageCache{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: pkgcfg.FromContext(ctx).PodNamespace,
					Name:      pkgutil.VMIName(ctx.ContentLibraryItemID),
				},
			}
			Expect(ctx.Client.Create(ctx, &vmic)).To(Succeed())

			vmicm := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vmic.Namespace,
					Name:      vmic.Name,
				},
				Data: map[string]string{
					"value": ovfEnvelopeYAML,
				},
			}
			Expect(ctx.Client.Create(ctx, &vmicm)).To(Succeed())

			vmic.Status = vmopv1.VirtualMachineImageCacheStatus{
				OVF: &vmopv1.VirtualMachineImageCacheOVFStatus{
					ConfigMapName:   vmic.Name,
					ProviderVersion: ctx.ContentLibraryItemVersion,
				},
				Conditions: []metav1.Condition{
					{
						Type:   vmopv1.VirtualMachineImageCacheConditionOVFReady,
						Status: metav1.ConditionTrue,
					},
				},
			}
			Expect(ctx.Client.Status().Update(ctx, &vmic)).To(Succeed())

			pkgcond.MarkTrue(
				&vmic,
				vmopv1.VirtualMachineImageCacheConditionFilesReady)
			vmic.Status.Locations = []vmopv1.VirtualMachineImageCacheLocationStatus{
				{
					DatacenterID: ctx.Datacenter.Reference().Value,
					DatastoreID:  ctx.Datastore.Reference().Value,
					Files: []vmopv1.VirtualMachineImageCacheFileStatus{
						{
							ID:       ctx.ContentLibraryItemDiskPath,
							Type:     vmopv1.VirtualMachineImageCacheFileTypeDisk,
							DiskType: vmopv1.VirtualMachineStorageDiskTypeClassic,
						},
						{
							ID:   ctx.ContentLibraryItemNVRAMPath,
							Type: vmopv1.VirtualMachineImageCacheFileTypeOther,
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:   vmopv1.ReadyConditionType,
							Status: metav1.ConditionTrue,
						},
					},
				},
			}
			Expect(ctx.Client.Status().Update(ctx, &vmic)).To(Succeed())
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}

		vm1 = nil
		vmClass = nil
		vmGroup = nil
	})

	assertMemberStatusForVM := func(vm *vmopv1.VirtualMachine, ms vmopv1.VirtualMachineGroupMemberStatus) {
		ExpectWithOffset(1, ms.Name).To(Equal(vm.Name), "Unexpected Name")
		ExpectWithOffset(1, ms.Kind).To(Equal("VirtualMachine"), "Unexpected Kind")
		ExpectWithOffset(1, ms.Placement).ToNot(BeNil(), "Missing Placement")
		ExpectWithOffset(1, pkgcond.IsTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)).To(BeTrue(), "No placement ready condition")
		ExpectWithOffset(1, ms.Placement.Name).ToNot(BeEmpty(), "Missing Placement Name")
		ExpectWithOffset(1, ms.Placement.Zone).ToNot(BeEmpty(), "Missing Placement Zone")
		ExpectWithOffset(1, ms.Placement.Pool).ToNot(BeEmpty(), "Missing Placement Pool")
		ExpectWithOffset(1, ms.Placement.Node).To(BeEmpty(), "Has Placement Node")
		if pkgcfg.FromContext(ctx).Features.FastDeploy {
			ExpectWithOffset(1, ms.Placement.Datastores).ToNot(BeEmpty(), "Missing Placement Datastores")
			// Verify against VirtualMachineImageCache.Status
		} else {
			ExpectWithOffset(1, ms.Placement.Datastores).To(BeEmpty(), "Has Placement Datastores")
		}
	}

	Context("Group Placement", func() {

		// vcsim PlaceVmsXCluster() only allows one ConfigSpec at the moment.
		It("VM Group with one VM member", func() {
			groupPlacements := []providers.VMGroupPlacement{
				{
					VMGroup: vmGroup,
					VMMembers: []*vmopv1.VirtualMachine{
						vm1,
					},
				},
			}

			err := vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmGroup.Status.Members).To(HaveLen(1))
			assertMemberStatusForVM(vm1, vmGroup.Status.Members[0])
		})
	})
}
