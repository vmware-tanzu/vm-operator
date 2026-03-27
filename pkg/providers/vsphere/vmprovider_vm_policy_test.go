// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/mo"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmconfpolicy "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/policy"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmPolicyTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
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

	var (
		policyTag1ID string
		policyTag2ID string
		policyTag3ID string

		tagMgr *tags.Manager
	)

	JustBeforeEach(func() {
		var err error

		tagMgr = tags.NewManager(ctx.RestClient)

		// Create a category for the policy tags
		categoryID, err := tagMgr.CreateCategory(ctx, &tags.Category{
			Name:            "my-policy-category",
			Description:     "Category for policy tags",
			AssociableTypes: []string{"VirtualMachine"},
		})
		Expect(err).ToNot(HaveOccurred())

		policyTag1ID, err = tagMgr.CreateTag(ctx, &tags.Tag{
			Name:       "my-policy-tag-1",
			CategoryID: categoryID,
		})
		Expect(err).ToNot(HaveOccurred())

		policyTag2ID, err = tagMgr.CreateTag(ctx, &tags.Tag{
			Name:       "my-policy-tag-2",
			CategoryID: categoryID,
		})
		Expect(err).ToNot(HaveOccurred())

		policyTag3ID, err = tagMgr.CreateTag(ctx, &tags.Tag{
			Name:       "my-policy-tag-3",
			CategoryID: categoryID,
		})
		Expect(err).ToNot(HaveOccurred())
	})

	When("Capability is enabled", func() {
		JustBeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VSpherePolicies = true
			})
		})

		When("creating a VM", func() {
			When("async create is enabled", func() {
				JustBeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.AsyncCreateEnabled = true
					})
				})
				It("should successfully create VM", func() {
					By("Setting up VM with policy evaluation objects", func() {
						// Set VM UID for proper PolicyEvaluation naming
						vm.UID = "test-vm-policy-uid"

						// Create a PolicyEvaluation object that will be found during policy reconciliation
						policyEval := &vspherepolv1.PolicyEvaluation{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:  vm.Namespace,
								Name:       "vm-" + vm.Name,
								Generation: 1,
								OwnerReferences: []metav1.OwnerReference{
									{
										APIVersion:         vmopv1.GroupVersion.String(),
										Kind:               "VirtualMachine",
										Name:               vm.Name,
										UID:                vm.UID,
										Controller:         ptr.To(true),
										BlockOwnerDeletion: ptr.To(true),
									},
								},
							},
							Spec: vspherepolv1.PolicyEvaluationSpec{
								Workload: &vspherepolv1.PolicyEvaluationWorkloadSpec{
									Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
										GuestID:     "ubuntu64Guest",
										GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
									},
								},
							},
							Status: vspherepolv1.PolicyEvaluationStatus{
								ObservedGeneration: 1,
								Policies: []vspherepolv1.PolicyEvaluationResult{
									{
										Name: "test-active-policy",
										Kind: "ComputePolicy",
										Tags: []string{policyTag1ID, policyTag2ID},
									},
								},
								Conditions: []metav1.Condition{
									*conditions.TrueCondition(vspherepolv1.ReadyConditionType),
								},
							},
						}

						// Create the PolicyEvaluation in the fake Kubernetes client
						Expect(ctx.Client.Create(ctx, policyEval)).To(Succeed())
					})

					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					// Verify VM was created successfully
					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					// Verify placement condition is ready (indicates vmconfpolicy.Reconcile was called)
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())

					// Verify that policy tags were added to ExtraConfig
					By("VM has policy tags in ExtraConfig", func() {
						Expect(o.Config.ExtraConfig).ToNot(BeNil())

						ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()

						// Verify tags are present
						Expect(ecMap).To(HaveKey(vmconfpolicy.ExtraConfigPolicyTagsKey))
						activeTags := ecMap[vmconfpolicy.ExtraConfigPolicyTagsKey]
						Expect(activeTags).To(ContainSubstring(policyTag1ID))
						Expect(activeTags).To(ContainSubstring(policyTag2ID))
					})
				})

			})
			When("async create is disabled", func() {
				JustBeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.AsyncCreateEnabled = false
					})
				})

				It("should successfully create VM and call vmconfig policy.Reconcile during placement", func() {
					By("Setting up VM with policy evaluation objects", func() {
						// Set VM UID for proper PolicyEvaluation naming
						vm.UID = "test-vm-sync-policy-uid"

						// Create a PolicyEvaluation object that will be found during policy reconciliation
						policyEval := &vspherepolv1.PolicyEvaluation{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:  vm.Namespace,
								Name:       "vm-" + vm.Name,
								Generation: 1,
								OwnerReferences: []metav1.OwnerReference{
									{
										APIVersion:         vmopv1.GroupVersion.String(),
										Kind:               "VirtualMachine",
										Name:               vm.Name,
										UID:                vm.UID,
										Controller:         ptr.To(true),
										BlockOwnerDeletion: ptr.To(true),
									},
								},
							},
							Spec: vspherepolv1.PolicyEvaluationSpec{
								Workload: &vspherepolv1.PolicyEvaluationWorkloadSpec{
									Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
										GuestID:     "ubuntu64Guest",
										GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
									},
								},
							},
							Status: vspherepolv1.PolicyEvaluationStatus{
								ObservedGeneration: 1,
								Policies: []vspherepolv1.PolicyEvaluationResult{
									{
										Name: "test-sync-active-policy",
										Kind: "ComputePolicy",
										Tags: []string{policyTag1ID, policyTag2ID},
									},
								},
								Conditions: []metav1.Condition{
									*conditions.TrueCondition(vspherepolv1.ReadyConditionType),
								},
							},
						}

						// Create the PolicyEvaluation in the fake Kubernetes client
						Expect(ctx.Client.Create(ctx, policyEval)).To(Succeed())
					})

					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					// Verify VM was created successfully
					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					// Verify placement condition is ready (indicates vmconfpolicy.Reconcile was called)
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())

					// Verify that policy tags were added to ExtraConfig
					By("VM has policy tags in ExtraConfig", func() {
						Expect(o.Config.ExtraConfig).ToNot(BeNil())

						ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()

						// Verify tags are present
						Expect(ecMap).To(HaveKey(vmconfpolicy.ExtraConfigPolicyTagsKey))
						activeTags := ecMap[vmconfpolicy.ExtraConfigPolicyTagsKey]
						Expect(activeTags).To(ContainSubstring(policyTag1ID))
						Expect(activeTags).To(ContainSubstring(policyTag2ID))
					})
				})
			})
		})

		When("updating a VM", func() {
			It("should update VM with policy tags during reconfiguration", func() {
				By("Setting up VM with policy evaluation objects", func() {
					// Set VM UID for proper PolicyEvaluation naming
					vm.UID = "test-vm-policy-uid"

					// Create a PolicyEvaluation object that will be found during policy reconciliation
					policyEval := &vspherepolv1.PolicyEvaluation{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:  vm.Namespace,
							Name:       "vm-" + vm.Name,
							Generation: 1,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         vmopv1.GroupVersion.String(),
									Kind:               "VirtualMachine",
									Name:               vm.Name,
									UID:                vm.UID,
									Controller:         ptr.To(true),
									BlockOwnerDeletion: ptr.To(true),
								},
							},
						},
						Spec: vspherepolv1.PolicyEvaluationSpec{
							Workload: &vspherepolv1.PolicyEvaluationWorkloadSpec{
								Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
									GuestID:     "ubuntu64Guest",
									GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
								},
							},
						},
						Status: vspherepolv1.PolicyEvaluationStatus{
							ObservedGeneration: 1,
							Conditions: []metav1.Condition{
								*conditions.TrueCondition(vspherepolv1.ReadyConditionType),
							},
						},
					}

					// Create the PolicyEvaluation in the fake Kubernetes client
					Expect(ctx.Client.Create(ctx, policyEval)).To(Succeed())
				})

				// First create the VM
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				By("Adding a non-policy tag to the VM", func() {
					mgr := tags.NewManager(ctx.RestClient)

					Expect(mgr.AttachTag(ctx, ctx.TagID, vcVM.Reference())).To(Succeed())

					list, err := mgr.GetAttachedTags(ctx, vcVM.Reference())
					Expect(err).ToNot(HaveOccurred())
					Expect(list).To(HaveLen(1))
					Expect(list[0].ID).To(Equal(ctx.TagID))
				})

				By("Setting up PolicyEvaluation for update", func() {
					// Create a PolicyEvaluation object with updated tags
					policyEval := &vspherepolv1.PolicyEvaluation{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: vm.Namespace,
							Name:      "vm-" + vm.Name,
						},
					}

					Expect(ctx.Client.Get(
						ctx,
						client.ObjectKeyFromObject(policyEval),
						policyEval)).To(Succeed())

					// Create a PolicyEvaluation object with updated tags
					policyEval.Status = vspherepolv1.PolicyEvaluationStatus{
						ObservedGeneration: policyEval.Generation,
						Policies: []vspherepolv1.PolicyEvaluationResult{
							{
								Name: "test-updated-active-policy",
								Kind: "ComputePolicy",
								Tags: []string{policyTag1ID, policyTag2ID, policyTag3ID},
							},
						},
						Conditions: []metav1.Condition{
							*conditions.TrueCondition(vspherepolv1.ReadyConditionType),
						},
					}

					// Update the PolicyEvaluation in the fake Kubernetes client
					Expect(ctx.Client.Status().Update(ctx, policyEval)).To(Succeed())
				})

				// Trigger VM update.
				vcVM, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				// Get VM properties.
				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

				// Verify that updated policy tags were added to ExtraConfig
				By("VM has updated policy tags in ExtraConfig", func() {
					Expect(o.Config.ExtraConfig).ToNot(BeNil())

					ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()

					// Verify updated tags are present
					Expect(ecMap).To(HaveKey(vmconfpolicy.ExtraConfigPolicyTagsKey))
					activeTags := ecMap[vmconfpolicy.ExtraConfigPolicyTagsKey]
					Expect(activeTags).To(ContainSubstring(policyTag1ID))
					Expect(activeTags).To(ContainSubstring(policyTag2ID))
					Expect(activeTags).To(ContainSubstring(policyTag3ID))
				})
			})
		})
	})

	When("Capability is disabled", func() {
		JustBeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VSpherePolicies = false
			})
		})

		When("creating a VM", func() {
			When("async create is enabled", func() {
				JustBeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.AsyncCreateEnabled = true
					})
				})
				It("should successfully create VM without calling vmconfpolicy.Reconcile", func() {
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					// Verify VM was created successfully
					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					// Verify placement condition is ready even without policy reconciliation
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())
				})

			})
			When("async create is disabled", func() {
				JustBeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.AsyncCreateEnabled = false
					})
				})

				It("should successfully create VM without calling vmconfpolicy.Reconcile", func() {
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					// Verify VM was created successfully
					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					// Verify placement condition is ready even without policy reconciliation
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())

					// Verify that no policy tags were added to ExtraConfig (policy disabled)
					By("VM should not have policy tags in ExtraConfig", func() {
						if o.Config.ExtraConfig != nil {
							ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()

							// Verify no tags are present
							Expect(ecMap).ToNot(HaveKey(vmconfpolicy.ExtraConfigPolicyTagsKey))
						}
					})
				})
			})
		})

		When("updating a VM", func() {
			It("should update VM without adding policy tags", func() {
				// First create the VM
				_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				// Trigger VM update.
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				// Get VM properties.
				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

				// Verify that no policy tags were added to ExtraConfig during update
				By("VM should not have policy tags in ExtraConfig after update", func() {
					if o.Config.ExtraConfig != nil {
						ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()

						// Verify no tags are present
						Expect(ecMap).ToNot(HaveKey(vmconfpolicy.ExtraConfigPolicyTagsKey))
					}
				})
			})
		})
	})
}
