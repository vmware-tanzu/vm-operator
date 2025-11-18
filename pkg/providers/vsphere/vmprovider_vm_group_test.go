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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
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
		vm2     *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
		vmGroup *vmopv1.VirtualMachineGroup
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{
			WithContentLibrary: true,
		}

		vm1 = builder.DummyBasicVirtualMachine("group-placement-vm-1", "")
		vm2 = builder.DummyBasicVirtualMachine("group-placement-vm-2", "")
		vmClass = builder.DummyVirtualMachineClassGenName()

		vmGroup = &vmopv1.VirtualMachineGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vm-group-test",
			},
			Spec: vmopv1.VirtualMachineGroupSpec{
				BootOrder: make([]vmopv1.VirtualMachineGroupBootOrderGroup, 1),
			},
		}
		vmGroup.Spec.BootOrder[0].Members = append(vmGroup.Spec.BootOrder[0].Members,
			vmopv1.GroupMember{Kind: "VirtualMachine", Name: vm1.Name},
			vmopv1.GroupMember{Kind: "VirtualMachine", Name: vm2.Name})
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

		initVM := func(vm *vmopv1.VirtualMachine) {
			vm.Namespace = nsInfo.Namespace
			vm.Spec.ClassName = vmClass.Name
			vm.Spec.ImageName = ctx.ContentLibraryImageName
			vm.Spec.Image.Kind = cvmiKind
			vm.Spec.Image.Name = ctx.ContentLibraryImageName
			vm.Spec.StorageClass = ctx.StorageClassName
			vm.Spec.GroupName = vmGroup.Name
		}
		initVM(vm1)
		initVM(vm2)

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
						Type:   vmopv1.VirtualMachineImageCacheConditionHardwareReady,
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
							DiskType: vmopv1.VolumeTypeClassic,
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
		vm2 = nil
		vmClass = nil
		vmGroup = nil
	})

	assertMemberStatusForVM := func(vm *vmopv1.VirtualMachine, ms vmopv1.VirtualMachineGroupMemberStatus) {
		GinkgoHelper()

		Expect(ms.Name).To(Equal(vm.Name), "Unexpected Name")
		Expect(ms.Kind).To(Equal("VirtualMachine"), "Unexpected Kind")
		Expect(ms.Placement).ToNot(BeNil(), "Missing Placement")
		Expect(pkgcond.IsTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)).To(BeTrue(), "No placement ready condition")
		Expect(ms.Placement.Zone).ToNot(BeEmpty(), "Missing Placement Zone")
		Expect(ms.Placement.Pool).ToNot(BeEmpty(), "Missing Placement Pool")
		Expect(ms.Placement.Node).To(BeEmpty(), "Has Placement Node")
		if pkgcfg.FromContext(ctx).Features.FastDeploy {
			Expect(ms.Placement.Datastores).ToNot(BeEmpty(), "Missing Placement Datastores")
			// Verify against VirtualMachineImageCache.Status
		} else {
			Expect(ms.Placement.Datastores).To(BeEmpty(), "Has Placement Datastores")
		}
	}

	assertNotReadyMemberStatusForVM := func(
		vm *vmopv1.VirtualMachine,
		ms vmopv1.VirtualMachineGroupMemberStatus,
		reason string) {

		GinkgoHelper()

		Expect(ms.Name).To(Equal(vm.Name), "Unexpected Name")
		Expect(ms.Kind).To(Equal("VirtualMachine"), "Unexpected Kind")
		Expect(ms.Placement).To(BeNil(), "Has Placement")

		c := pkgcond.Get(ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
		Expect(c).ToNot(BeNil(), "Condition missing")
		Expect(c.Status).To(Equal(metav1.ConditionFalse))
		Expect(c.Reason).To(Equal(reason))
	}

	Context("Group placement with VMs specifying affinity policies", func() {
		It("should process preferred VM affinity policies during group placement", func() {
			// Add preferred affinity policy to vm1
			vm1.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAffinity: &vmopv1.VMAffinitySpec{
					PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "database",
								},
							},
							TopologyKey: "topology.kubernetes.io/zone",
						},
					},
				},
			}

			groupPlacements := []providers.VMGroupPlacement{
				{
					VMGroup: vmGroup,
					VMMembers: []*vmopv1.VirtualMachine{
						vm1,
						vm2,
					},
				},
			}

			err := vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmGroup.Status.Members).To(HaveLen(2))
			assertMemberStatusForVM(vm1, vmGroup.Status.Members[0])
			assertMemberStatusForVM(vm2, vmGroup.Status.Members[1])
		})

		It("should process required VM affinity policies during group placement", func() {
			// Add required affinity policy to vm1
			vm1.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAffinity: &vmopv1.VMAffinitySpec{
					RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"tier": "frontend",
								},
							},
							TopologyKey: "topology.kubernetes.io/zone",
						},
					},
				},
			}

			groupPlacements := []providers.VMGroupPlacement{
				{
					VMGroup: vmGroup,
					VMMembers: []*vmopv1.VirtualMachine{
						vm1,
						vm2,
					},
				},
			}

			err := vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmGroup.Status.Members).To(HaveLen(2))
			assertMemberStatusForVM(vm1, vmGroup.Status.Members[0])
			assertMemberStatusForVM(vm2, vmGroup.Status.Members[1])
		})
	})

	Context("VSpherePolicies is enabled", func() {
		JustBeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VSpherePolicies = true
			})
		})

		It("should process VM with PolicyEval during group placement", func() {
			groupPlacements := []providers.VMGroupPlacement{
				{
					VMGroup: vmGroup,
					VMMembers: []*vmopv1.VirtualMachine{
						vm1,
						vm2,
					},
				},
			}

			err := vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
			Expect(err).To(HaveOccurred())

			Expect(vmGroup.Status.Members).To(HaveLen(2))
			assertNotReadyMemberStatusForVM(vm1, vmGroup.Status.Members[0], "NotReady")
			assertNotReadyMemberStatusForVM(vm2, vmGroup.Status.Members[1], "NotReady")

			markPolicyEvalReady := func(vm *vmopv1.VirtualMachine) {
				policyEval := &vspherepolv1.PolicyEvaluation{}
				Expect(ctx.Client.Get(ctx, client.ObjectKey{
					Namespace: vm.Namespace,
					Name:      "vm-" + vm.Name},
					policyEval)).To(Succeed())
				policyEval.Status.ObservedGeneration = policyEval.Generation
				pkgcond.MarkTrue(policyEval, vspherepolv1.ReadyConditionType)
				Expect(ctx.Client.Status().Update(ctx, policyEval)).To(Succeed())
			}

			markPolicyEvalReady(vm1)
			err = vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(vsphere.ErrVMGroupPlacementConfigSpec))

			Expect(vmGroup.Status.Members).To(HaveLen(2))
			assertNotReadyMemberStatusForVM(vm1, vmGroup.Status.Members[0], "PendingPlacement")
			assertNotReadyMemberStatusForVM(vm2, vmGroup.Status.Members[1], "NotReady")

			markPolicyEvalReady(vm2)
			err = vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
			Expect(err).ToNot(HaveOccurred())

			Expect(vmGroup.Status.Members).To(HaveLen(2))
			assertMemberStatusForVM(vm1, vmGroup.Status.Members[0])
			assertMemberStatusForVM(vm2, vmGroup.Status.Members[1])
		})
	})
}
