// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"math/rand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmGroupTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass

		zoneName string
	)

	BeforeEach(func() {
		parentCtx = pkgcfg.NewContextWithDefaultConfig()
		parentCtx = ctxop.WithContext(parentCtx)
		parentCtx = ovfcache.WithContext(parentCtx)
		parentCtx = cource.WithContext(parentCtx)
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = false
			config.AsyncSignalEnabled = false
		})
		testConfig = builder.VCSimTestConfig{
			WithContentLibrary: true,
		}

		vmClass = builder.DummyVirtualMachineClassGenName()
		vm = builder.DummyBasicVirtualMachine("test-vm", "")

		if vm.Spec.Network == nil {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
		}
		vm.Spec.Network.Disabled = true
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSimWithParentContext(
			parentCtx, testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(
			ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

		clusterVMI1 := &vmopv1.ClusterVirtualMachineImage{}

		if testConfig.WithContentLibrary {
			Expect(ctx.Client.Get(
				ctx, client.ObjectKey{Name: ctx.ContentLibraryItem1Name},
				clusterVMI1)).To(Succeed())
		} else {
			vsphere.SkipVMImageCLProviderCheck = true
			clusterVMI1 = builder.DummyClusterVirtualMachineImage("DC0_C0_RP0_VM0")
			Expect(ctx.Client.Create(ctx, clusterVMI1)).To(Succeed())
			conditions.MarkTrue(clusterVMI1, vmopv1.ReadyConditionType)
			Expect(ctx.Client.Status().Update(ctx, clusterVMI1)).To(Succeed())
		}

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.ImageName = clusterVMI1.Name
		vm.Spec.Image.Kind = cvmiKind
		vm.Spec.Image.Name = clusterVMI1.Name
		vm.Spec.StorageClass = ctx.StorageClassName

		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

		zoneName = ctx.GetFirstZoneName()
		vm.Labels[corev1.LabelTopologyZone] = zoneName
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
	})

	AfterEach(func() {
		vsphere.SkipVMImageCLProviderCheck = false

		if vm != nil &&
			!pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
			By("Assert vm.Status.Crypto is nil when BYOK is disabled", func() {
				Expect(vm.Status.Crypto).To(BeNil())
			})
		}

		vmClass = nil
		vm = nil

		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	BeforeEach(func() {
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.Features.VMGroups = true
		})
	})

	var vmg vmopv1.VirtualMachineGroup
	JustBeforeEach(func() {
		// Remove explicit zone label so group placement can work
		if vm.Labels != nil {
			delete(vm.Labels, corev1.LabelTopologyZone)
			Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
		}

		vmg = vmopv1.VirtualMachineGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vmg",
				Namespace: vm.Namespace,
			},
		}
		Expect(ctx.Client.Create(ctx, &vmg)).To(Succeed())
	})

	Context("VM Creation", func() {
		When("spec.groupName is set to a non-existent group", func() {
			JustBeforeEach(func() {
				vm.Spec.GroupName = "vmg-invalid"
			})
			Specify("it should return an error creating VM", func() {
				err := createOrUpdateVM(ctx, vmProvider, vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("VM is not linked to its group"))
			})
		})

		When("spec.groupName is set to a group to which the VM does not belong", func() {
			JustBeforeEach(func() {
				vm.Spec.GroupName = vmg.Name
			})
			Specify("it should return an error creating VM", func() {
				err := createOrUpdateVM(ctx, vmProvider, vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("VM is not linked to its group"))
			})
		})

		When("spec.groupName is set to a group to which the VM does belong", func() {
			JustBeforeEach(func() {
				vm.Spec.GroupName = vmg.Name
				vmg.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{
								Name: vm.Name,
								Kind: "VirtualMachine",
							},
						},
					},
				}
				Expect(ctx.Client.Update(ctx, &vmg)).To(Succeed())
			})

			When("VM Group placement condition is not ready", func() {
				JustBeforeEach(func() {
					vmg.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
						{
							Name: vm.Name,
							Kind: "VirtualMachine",
							Conditions: []metav1.Condition{
								{
									Type:   vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
									Status: metav1.ConditionFalse,
								},
							},
						},
					}
					Expect(ctx.Client.Status().Update(ctx, &vmg)).To(Succeed())
				})
				Specify("it should return an error creating VM", func() {
					err := createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).To(HaveOccurred())
					Expect(pkgerr.IsNoRequeueError(err)).To(BeTrue())
					Expect(err.Error()).To(ContainSubstring("VM Group placement is not ready"))
				})
			})

			When("VM Group placement condition is ready", func() {
				var (
					groupZone string
					groupHost string
					groupPool string
				)
				JustBeforeEach(func() {
					// Ensure the group zone is different to verify the placement actually from group.
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
					groupZone = ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))]
					for groupZone == vm.Labels[corev1.LabelTopologyZone] {
						groupZone = ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))]
					}

					ccrs := ctx.GetAZClusterComputes(groupZone)
					Expect(ccrs).ToNot(BeEmpty())
					ccr := ccrs[0]
					hosts, err := ccr.Hosts(ctx)
					Expect(err).ToNot(HaveOccurred())
					Expect(hosts).ToNot(BeEmpty())
					groupHost = hosts[0].Reference().Value

					nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, groupZone, "")
					Expect(nsRP).ToNot(BeNil())
					groupPool = nsRP.Reference().Value

					vmg.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
						{
							Name: vm.Name,
							Kind: "VirtualMachine",
							Conditions: []metav1.Condition{
								{
									Type:   vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
									Status: metav1.ConditionTrue,
								},
							},
							Placement: &vmopv1.VirtualMachinePlacementStatus{
								Zone: groupZone,
								Node: groupHost,
								Pool: groupPool,
							},
						},
					}
					Expect(ctx.Client.Status().Update(ctx, &vmg)).To(Succeed())
				})
				Specify("it should successfully create VM from group's placement", func() {
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())
					By("VM is placed in the expected zone from group", func() {
						Expect(vm.Status.Zone).To(Equal(groupZone))
					})
					By("VM is placed in the expected host from group", func() {
						vmHost, err := vcVM.HostSystem(ctx)
						Expect(err).ToNot(HaveOccurred())
						Expect(vmHost.Reference().Value).To(Equal(groupHost))
					})
					By("VM is created in the expected pool from group", func() {
						rp, err := vcVM.ResourcePool(ctx)
						Expect(err).ToNot(HaveOccurred())
						Expect(rp.Reference().Value).To(Equal(groupPool))
					})
					By("VM has expected group linked condition", func() {
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
					})
				})
			})
		})
	})

	Context("VM Update", func() {
		JustBeforeEach(func() {
			// Unset groupName to ensure the VM can be created.
			vm.Spec.GroupName = ""
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
		})

		When("spec.groupName is set to a non-existent group", func() {
			JustBeforeEach(func() {
				vm.Spec.GroupName = "vmg-invalid"
			})
			Specify("vm should have group linked condition set to false", func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
			})
		})

		When("spec.groupName is set to a group to which the VM does not belong", func() {
			JustBeforeEach(func() {
				vm.Spec.GroupName = vmg.Name
			})
			Specify("vm should have group linked condition set to false", func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
			})
		})

		When("spec.groupName is set to a group to which the VM does belong", func() {
			JustBeforeEach(func() {
				vm.Spec.GroupName = vmg.Name
				vmg.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{
								Name: vm.Name,
								Kind: "VirtualMachine",
							},
						},
					},
				}
				Expect(ctx.Client.Update(ctx, &vmg)).To(Succeed())
			})
			Specify("vm should have group linked condition set to true", func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
			})

			When("spec.groupName no longer points to group", func() {
				Specify("vm should no longer have group linked condition", func() {
					vm.Spec.GroupName = ""
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					c := conditions.Get(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
					Expect(c).To(BeNil())
				})
			})
		})
	})

	Context("Zone Label Override for VM Groups", func() {
		var (
			vm       *vmopv1.VirtualMachine
			vmGroup  *vmopv1.VirtualMachineGroup
			vmClass  *vmopv1.VirtualMachineClass
			zoneName string
		)

		BeforeEach(func() {
			vmClass = builder.DummyVirtualMachineClassGenName()
			vm = builder.DummyBasicVirtualMachine("test-vm-zone-override", "")
			vmGroup = &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-group-zone-override",
				},
				Spec: vmopv1.VirtualMachineGroupSpec{
					BootOrder: []vmopv1.VirtualMachineGroupBootOrderGroup{
						{
							Members: []vmopv1.GroupMember{
								{Kind: "VirtualMachine", Name: vm.ObjectMeta.Name},
							},
						},
					},
				},
			}
		})

		JustBeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMGroups = true
			})

			vmClass.Namespace = nsInfo.Namespace
			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

			vmGroup.Namespace = nsInfo.Namespace
			Expect(ctx.Client.Create(ctx, vmGroup)).To(Succeed())

			clusterVMImage := &vmopv1.ClusterVirtualMachineImage{}
			Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: ctx.ContentLibraryItem1Name}, clusterVMImage)).To(Succeed())

			vm.Namespace = nsInfo.Namespace
			vm.Spec.ClassName = vmClass.Name
			vm.Spec.ImageName = clusterVMImage.Name
			vm.Spec.Image.Kind = cvmiKind
			vm.Spec.Image.Name = clusterVMImage.Name
			vm.Spec.StorageClass = ctx.StorageClassName

			vm.Spec.GroupName = vmGroup.Name

			zoneName = ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))]
			vm.Labels = map[string]string{
				corev1.LabelTopologyZone: zoneName,
			}

			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
		})

		Context("when VM has explicit zone label and is part of group", func() {
			It("should create VM in specified zone, not using group placement", func() {
				err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
				Expect(err).To(HaveOccurred())
				Expect(pkgerr.IsNoRequeueError(err)).To(BeTrue())
				Expect(err).To(MatchError(vsphere.ErrCreate))

				// Verify VM was created
				Expect(vm.Status.UniqueID).ToNot(BeEmpty())
			})
		})

		Context("when VM has explicit zone label but is not linked to group", func() {
			BeforeEach(func() {
				vmGroup.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{}
			})

			It("should fail to create VM", func() {
				err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("VM is not linked to its group"))
			})
		})
	})
}
