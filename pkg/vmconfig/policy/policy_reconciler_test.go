// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"context"
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	vmconfpolicy "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/policy"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("New", func() {
	It("should return a reconciler", func() {
		Expect(vmconfpolicy.New()).ToNot(BeNil())
	})
})

var _ = Describe("Name", func() {
	It("should return 'policy'", func() {
		Expect(vmconfpolicy.New().Name()).To(Equal("policy"))
	})
})

var _ = Describe("OnResult", func() {
	It("should return nil", func() {
		var ctx context.Context
		Expect(vmconfpolicy.New().OnResult(ctx, nil, mo.VirtualMachine{}, nil)).To(Succeed())
	})
})

var _ = Describe("Reconcile", func() {
	var (
		ctx        context.Context
		vcSimCtx   *builder.TestContextForVCSim
		k8sClient  ctrlclient.Client
		vimClient  *vim25.Client
		moVM       mo.VirtualMachine
		vm         *vmopv1.VirtualMachine
		tagMgr     *tags.Manager
		withObjs   []ctrlclient.Object
		withFuncs  interceptor.Funcs
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		vcSimCtx = builder.NewTestContextForVCSim(
			ctxop.WithContext(pkgcfg.NewContextWithDefaultConfig()),
			builder.VCSimTestConfig{})
		ctx = vcSimCtx
		ctx = vmconfig.WithContext(ctx)
		ctx = pkgctx.WithRestClient(ctx, vcSimCtx.RestClient)

		vimClient = vcSimCtx.VCClient.Client
		tagMgr = tags.NewManager(vcSimCtx.RestClient)

		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
			Guest: &vimtypes.GuestInfo{
				GuestId:     "ubuntu64Guest",
				GuestFamily: string(vimtypes.VirtualMachineGuestOsFamilyLinuxGuest),
			},
		}

		configSpec = &vimtypes.VirtualMachineConfigSpec{}

		vm = &vmopv1.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: vmopv1.GroupVersion.String(),
				Kind:       "VirtualMachine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "my-namespace",
				Name:      "my-vm",
				UID:       "test-vm-uid",
			},
			Spec: vmopv1.VirtualMachineSpec{},
		}

		withFuncs = interceptor.Funcs{}
		withObjs = []ctrlclient.Object{}
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
	})

	AfterEach(func() {
		vcSimCtx.AfterEach()
		vcSimCtx = nil
	})

	Context("a panic is expected", func() {
		When("ctx is nil", func() {
			JustBeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})
		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("k8sClient is nil"))
			})
		})
		When("vimClient is nil", func() {
			JustBeforeEach(func() {
				vimClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vimClient is nil"))
			})
		})
		When("vm is nil", func() {
			JustBeforeEach(func() {
				vm = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})
		When("configSpec is nil", func() {
			JustBeforeEach(func() {
				configSpec = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("configSpec is nil"))
			})
		})
		When("restClient is nil", func() {
			JustBeforeEach(func() {
				ctx = context.Background()
			})
			It("should panic", func() {
				fn := func() {
					_ = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("restClient is nil"))
			})
		})
	})

	When("no panic is expected", func() {

		var (
			policyTag1ID string
			policyTag2ID string
			policyTag3ID string

			tagPolicy1     *vspherepolv1.TagPolicy
			tagPolicy2     *vspherepolv1.TagPolicy
			computePolicy1 *vspherepolv1.ComputePolicy
			computePolicy2 *vspherepolv1.ComputePolicy
		)

		BeforeEach(func() {
			var err error

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

			tagPolicy1 = &vspherepolv1.TagPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-tag-policy-1",
					Namespace: vm.Namespace,
				},
				Spec: vspherepolv1.TagPolicySpec{
					Tags: []string{
						policyTag1ID,
						policyTag2ID,
					},
				},
			}
			tagPolicy2 = &vspherepolv1.TagPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-tag-policy-2",
					Namespace: vm.Namespace,
				},
				Spec: vspherepolv1.TagPolicySpec{
					Tags: []string{
						policyTag3ID,
					},
				},
			}

			computePolicy1 = &vspherepolv1.ComputePolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-compute-policy-1",
					Namespace: vm.Namespace,
				},
				Spec: vspherepolv1.ComputePolicySpec{
					EnforcementMode: vspherepolv1.PolicyEnforcementModeMandatory,
					Tags:            []string{tagPolicy1.Name},
				},
			}
			computePolicy2 = &vspherepolv1.ComputePolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-compute-policy-2",
					Namespace: vm.Namespace,
				},
				Spec: vspherepolv1.ComputePolicySpec{
					EnforcementMode: vspherepolv1.PolicyEnforcementModeMandatory,
					Tags:            []string{tagPolicy2.Name},
				},
			}

			withObjs = append(withObjs,
				tagPolicy1,
				tagPolicy2,
				computePolicy1,
				computePolicy2)
		})

		When("a vm is being created", func() {
			BeforeEach(func() {
				moVM = mo.VirtualMachine{}
				configSpec.GuestId = "ubuntu64Guest"
			})

			When("the policy object is created", func() {
				It("should return ErrPolicyNotReady", func() {
					err := vmconfpolicy.Reconcile(
						ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(errors.Is(err, vmconfpolicy.ErrPolicyNotReady)).To(BeTrue())
				})
			})

			When("the policy object is updated", func() {
				BeforeEach(func() {
					// Create a PolicyEvaluation that requires policyTag1ID and policyTag2ID
					policyEval := &vspherepolv1.PolicyEvaluation{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:  vm.Namespace,
							Name:       "vm-" + vm.Name,
							Generation: 1,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         vm.APIVersion,
									Kind:               vm.Kind,
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
									GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
								},
							},
						},
						Status: vspherepolv1.PolicyEvaluationStatus{
							ObservedGeneration: 1,
							Policies: []vspherepolv1.PolicyEvaluationResult{{
								Tags: []string{policyTag1ID, policyTag2ID},
							}},
						},
					}
					withObjs = append(withObjs, policyEval)
				})

				It("should return ErrPolicyNotReady", func() {
					err := vmconfpolicy.Reconcile(
						ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(errors.Is(err, vmconfpolicy.ErrPolicyNotReady)).To(BeTrue())
				})
			})

			When("the policy generations do not match", func() {
				BeforeEach(func() {
					// Create a PolicyEvaluation that requires policyTag1ID and policyTag2ID
					policyEval := &vspherepolv1.PolicyEvaluation{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:  vm.Namespace,
							Name:       "vm-" + vm.Name,
							Generation: 2,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         vm.APIVersion,
									Kind:               vm.Kind,
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
									GuestID:     configSpec.GuestId,
									GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
								},
							},
						},
						Status: vspherepolv1.PolicyEvaluationStatus{
							ObservedGeneration: 1,
							Policies: []vspherepolv1.PolicyEvaluationResult{{
								Tags: []string{policyTag1ID, policyTag2ID},
							}},
						},
					}
					withObjs = append(withObjs, policyEval)
				})

				It("should return ErrPolicyNotReady", func() {
					err := vmconfpolicy.Reconcile(
						ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(errors.Is(err, vmconfpolicy.ErrPolicyNotReady)).To(BeTrue())
				})
			})

			When("the vm is subject to a single policy", func() {

				BeforeEach(func() {
					// Create a PolicyEvaluation that requires policyTag1ID and policyTag2ID
					policyEval := &vspherepolv1.PolicyEvaluation{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:  vm.Namespace,
							Name:       "vm-" + vm.Name,
							Generation: 1,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         vm.APIVersion,
									Kind:               vm.Kind,
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
									GuestID:     configSpec.GuestId,
									GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
								},
							},
						},
						Status: vspherepolv1.PolicyEvaluationStatus{
							ObservedGeneration: 1,
							Policies: []vspherepolv1.PolicyEvaluationResult{{
								Tags: []string{policyTag1ID, policyTag2ID},
							}},
							Conditions: []metav1.Condition{
								*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
							},
						},
					}
					withObjs = append(withObjs, policyEval)
				})

				It("should associate the policy's tags with the vm", func() {
					Expect(vmconfpolicy.Reconcile(
						ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
					ec := object.OptionValueList(configSpec.ExtraConfig)
					recordedPolicyCSV, _ := ec.GetString(
						vmconfpolicy.ExtraConfigPolicyTagsKey)
					recordedPolicies := strings.Split(recordedPolicyCSV, ",")
					Expect(recordedPolicies).To(ConsistOf(policyTag1ID, policyTag2ID))

					Expect(configSpec.TagSpecs).To(ConsistOf(
						tagSpec(vimtypes.ArrayUpdateOperationAdd, policyTag1ID),
						tagSpec(vimtypes.ArrayUpdateOperationAdd, policyTag2ID)))
				})
			})

			When("the vm is subject to multiple policies", func() {
				BeforeEach(func() {
					// Create a PolicyEvaluation with multiple policy results
					policyEval := &vspherepolv1.PolicyEvaluation{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:  vm.Namespace,
							Name:       "vm-" + vm.Name,
							Generation: 1,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         vm.APIVersion,
									Kind:               vm.Kind,
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
									GuestID:     configSpec.GuestId,
									GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
								},
							},
						},
						Status: vspherepolv1.PolicyEvaluationStatus{
							ObservedGeneration: 1,
							Policies: []vspherepolv1.PolicyEvaluationResult{
								{Tags: []string{policyTag1ID, policyTag2ID}}, // From policy 1
								{Tags: []string{policyTag3ID}},               // From policy 2
							},
							Conditions: []metav1.Condition{
								*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
							},
						},
					}
					withObjs = append(withObjs, policyEval)
				})

				It("should associate the policies' tags with the vm", func() {
					Expect(vmconfpolicy.Reconcile(
						ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
					ec := object.OptionValueList(configSpec.ExtraConfig)
					recordedPolicyCSV, _ := ec.GetString(
						vmconfpolicy.ExtraConfigPolicyTagsKey)
					recordedPolicies := strings.Split(recordedPolicyCSV, ",")
					Expect(recordedPolicies).To(ConsistOf(policyTag1ID, policyTag2ID, policyTag3ID))

					Expect(configSpec.TagSpecs).To(ConsistOf(
						tagSpec(vimtypes.ArrayUpdateOperationAdd, policyTag1ID),
						tagSpec(vimtypes.ArrayUpdateOperationAdd, policyTag2ID),
						tagSpec(vimtypes.ArrayUpdateOperationAdd, policyTag3ID)))
				})
			})
		})

		When("a vm is being updated", func() {
			var (
				simVM *simulator.VirtualMachine
			)
			BeforeEach(func() {
				simVM = vcSimCtx.SimulatorContext().Map.
					Any("VirtualMachine").(*simulator.VirtualMachine)

				moVM = mo.VirtualMachine{
					ManagedEntity: mo.ManagedEntity{
						ExtensibleManagedObject: mo.ExtensibleManagedObject{
							Self: simVM.Self,
						},
					},
					Config:  simVM.Config,
					Guest:   simVM.Guest,
					Summary: simVM.Summary,
					Runtime: simVM.Runtime,
				}
			})

			When("a vm has existing tags", func() {
				JustBeforeEach(func() {
					// Populate the context with the tags associated with the VM.
					tagObjs, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
					Expect(err).ToNot(HaveOccurred())
					tagIDs := make([]string, len(tagObjs))
					for i := range tagObjs {
						tagIDs[i] = tagObjs[i].ID
					}
					ctx = pkgctx.WithVMTags(ctx, tagIDs)
				})

				Context("related to policy and unrelated to policy", func() {
					BeforeEach(func() {
						Expect(tagMgr.AttachMultipleTagsToObject(
							ctx,
							[]string{vcSimCtx.TagID, policyTag3ID},
							moVM.Self,
						)).To(Succeed())
					})

					When("the vm is not subject to any policy", func() {

						BeforeEach(func() {
							// Create a PolicyEvaluation with no resulting policy tags
							policyEval := &vspherepolv1.PolicyEvaluation{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:  vm.Namespace,
									Name:       "vm-" + vm.Name,
									Generation: 1,
									OwnerReferences: []metav1.OwnerReference{
										{
											APIVersion:         vm.APIVersion,
											Kind:               vm.Kind,
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
											GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
										},
									},
								},
								Status: vspherepolv1.PolicyEvaluationStatus{
									ObservedGeneration: 1,
									Policies:           []vspherepolv1.PolicyEvaluationResult{}, // No policies apply
									Conditions: []metav1.Condition{
										*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
									},
								},
							}
							withObjs = append(withObjs, policyEval)
						})

						It("should remove the policy-related tags from the vm", func() {

							// VM has policy tags but no policies apply, should remove policy tags
							// Add policy tags to ExtraConfig to simulate they were previously applied
							moVM.Config.ExtraConfig = append(moVM.Config.ExtraConfig,
								&vimtypes.OptionValue{
									Key:   vmconfpolicy.ExtraConfigPolicyTagsKey,
									Value: policyTag3ID, // This tag was attached but policy doesn't apply
								})

							err := vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
							Expect(err).ToNot(HaveOccurred())

							// Verify that the policy tag was detached
							attachedTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
							Expect(err).ToNot(HaveOccurred())
							tagIDs := make([]string, len(attachedTags))
							for i, tag := range attachedTags {
								tagIDs[i] = tag.ID
							}
							// Should only have the non-policy tag
							Expect(tagIDs).To(ContainElement(vcSimCtx.TagID))
							Expect(tagIDs).ToNot(ContainElement(policyTag3ID))

							// BMV: I think the test setup for this is a little funky: policyTag3ID
							// does not look to have a PolicyEval for it, so that's why it isn't here.
							ecTags, ok := object.OptionValueList(configSpec.ExtraConfig).
								GetString(vmconfpolicy.ExtraConfigPolicyTagsKey)
							Expect(ok).To(BeTrue())
							Expect(ecTags).To(BeEmpty())
						})
					})

					When("the vm is subject to a single policy", func() {
						When("that policy's tags are already associated with the vm", func() {
							BeforeEach(func() {
								// Create a PolicyEvaluation that requires policyTag3ID
								policyEval := &vspherepolv1.PolicyEvaluation{
									ObjectMeta: metav1.ObjectMeta{
										Namespace:  vm.Namespace,
										Name:       "vm-" + vm.Name,
										Generation: 1,
										OwnerReferences: []metav1.OwnerReference{
											{
												APIVersion:         vm.APIVersion,
												Kind:               vm.Kind,
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
												GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
											},
										},
									},
									Status: vspherepolv1.PolicyEvaluationStatus{
										ObservedGeneration: 1,
										Policies: []vspherepolv1.PolicyEvaluationResult{{
											Tags: []string{policyTag3ID},
										}},
										Conditions: []metav1.Condition{
											*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
										},
									},
								}
								withObjs = append(withObjs, policyEval)

								// Add policy tags to ExtraConfig to simulate they were previously applied
								moVM.Config.ExtraConfig = append(moVM.Config.ExtraConfig,
									&vimtypes.OptionValue{
										Key:   vmconfpolicy.ExtraConfigPolicyTagsKey,
										Value: policyTag3ID,
									})
							})

							It("should not modify the VM's tag associations", func() {
								// Record initial state
								initialTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
								Expect(err).ToNot(HaveOccurred())
								initialTagCount := len(initialTags)

								err = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
								Expect(err).ToNot(HaveOccurred())

								// Verify no changes were made
								finalTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
								Expect(err).ToNot(HaveOccurred())
								Expect(len(finalTags)).To(Equal(initialTagCount))

								// Verify specific tags still exist
								finalTagIDs := make([]string, len(finalTags))
								for i, tag := range finalTags {
									finalTagIDs[i] = tag.ID
								}
								Expect(finalTagIDs).To(ContainElement(vcSimCtx.TagID))
								Expect(finalTagIDs).To(ContainElement(policyTag3ID))

								Expect(configSpec.ExtraConfig).To(BeEmpty())
							})
						})
						When("that policy's tags are not already associated with the vm", func() {
							BeforeEach(func() {
								// Create a PolicyEvaluation that requires policyTag1ID and policyTag2ID
								policyEval := &vspherepolv1.PolicyEvaluation{
									ObjectMeta: metav1.ObjectMeta{
										Namespace:  vm.Namespace,
										Name:       "vm-" + vm.Name,
										Generation: 1,
										OwnerReferences: []metav1.OwnerReference{
											{
												APIVersion:         vm.APIVersion,
												Kind:               vm.Kind,
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
												GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
											},
										},
									},
									Status: vspherepolv1.PolicyEvaluationStatus{
										ObservedGeneration: 1,
										Policies: []vspherepolv1.PolicyEvaluationResult{{
											Tags: []string{policyTag1ID, policyTag2ID},
										}},
										Conditions: []metav1.Condition{
											*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
										},
									},
								}
								withObjs = append(withObjs, policyEval)

								// Remove policyTag3ID from attached tags, simulate policy requires different tags
								Expect(tagMgr.DetachTag(ctx, policyTag3ID, moVM.Self)).To(Succeed())
							})

							It("should associate the policy's tags with the vm", func() {
								// Record initial tags
								initialTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
								Expect(err).ToNot(HaveOccurred())
								initialTagIDs := make([]string, len(initialTags))
								for i, tag := range initialTags {
									initialTagIDs[i] = tag.ID
								}

								err = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
								Expect(err).ToNot(HaveOccurred())

								// Verify policy tags were attached
								finalTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
								Expect(err).ToNot(HaveOccurred())
								finalTagIDs := make([]string, len(finalTags))
								for i, tag := range finalTags {
									finalTagIDs[i] = tag.ID
								}

								// Should have all original tags plus new policy tags
								for _, tagID := range initialTagIDs {
									Expect(finalTagIDs).To(ContainElement(tagID))
								}
								// Should have the new policy tags
								Expect(finalTagIDs).To(ContainElement(policyTag1ID))
								Expect(finalTagIDs).To(ContainElement(policyTag2ID))

								// Should have new policy tags added
								ecTags, ok := object.OptionValueList(configSpec.ExtraConfig).
									GetString(vmconfpolicy.ExtraConfigPolicyTagsKey)
								Expect(ok).To(BeTrue())
								Expect(strings.Split(ecTags, ",")).To(ConsistOf(
									policyTag1ID, policyTag2ID))
							})
						})
					})

					When("the vm is subject to multiple policies", func() {
						When("the tags on the vm are not for any of the policies", func() {
							BeforeEach(func() {
								// Create a PolicyEvaluation with multiple policy results
								policyEval := &vspherepolv1.PolicyEvaluation{
									ObjectMeta: metav1.ObjectMeta{
										Namespace:  vm.Namespace,
										Name:       "vm-" + vm.Name,
										Generation: 1,
										OwnerReferences: []metav1.OwnerReference{
											{
												APIVersion:         vm.APIVersion,
												Kind:               vm.Kind,
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
												GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
											},
										},
									},
									Status: vspherepolv1.PolicyEvaluationStatus{
										ObservedGeneration: 1,
										Policies: []vspherepolv1.PolicyEvaluationResult{
											{Tags: []string{policyTag1ID, policyTag2ID}}, // From policy 1
											{Tags: []string{policyTag3ID}},               // From policy 2
										},
										Conditions: []metav1.Condition{
											*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
										},
									},
								}
								withObjs = append(withObjs, policyEval)
							})

							It("should associate the policies' tags with the vm", func() {
								// VM currently has policyTag3ID and vcSimCtx.TagID
								// But policies require policyTag1ID and policyTag2ID (from computePolicy1)
								// So it should add the required tags

								initialTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
								Expect(err).ToNot(HaveOccurred())
								initialCount := len(initialTags)

								err = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
								Expect(err).ToNot(HaveOccurred())

								finalTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
								Expect(err).ToNot(HaveOccurred())
								finalTagIDs := make([]string, len(finalTags))
								for i, tag := range finalTags {
									finalTagIDs[i] = tag.ID
								}

								// Should have more tags than initially (new policy tags added)
								Expect(len(finalTags)).To(BeNumerically(">=", initialCount))
								// Should contain non-policy tag
								Expect(finalTagIDs).To(ContainElement(vcSimCtx.TagID))
								// Should contain all required policy tags
								Expect(finalTagIDs).To(ContainElement(policyTag1ID))
								Expect(finalTagIDs).To(ContainElement(policyTag2ID))
								Expect(finalTagIDs).To(ContainElement(policyTag3ID))

								// Should have new policy tags added
								ecTags, ok := object.OptionValueList(configSpec.ExtraConfig).
									GetString(vmconfpolicy.ExtraConfigPolicyTagsKey)
								Expect(ok).To(BeTrue())
								Expect(strings.Split(ecTags, ",")).To(ConsistOf(
									policyTag1ID, policyTag2ID, policyTag3ID))
							})
						})
						When("the tags on the vm are for some of the policies", func() {
							BeforeEach(func() {
								// Create a PolicyEvaluation with multiple policy results
								policyEval := &vspherepolv1.PolicyEvaluation{
									ObjectMeta: metav1.ObjectMeta{
										Namespace:  vm.Namespace,
										Name:       "vm-" + vm.Name,
										Generation: 1,
										OwnerReferences: []metav1.OwnerReference{
											{
												APIVersion:         vm.APIVersion,
												Kind:               vm.Kind,
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
												GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
											},
										},
									},
									Status: vspherepolv1.PolicyEvaluationStatus{
										ObservedGeneration: 1,
										Policies: []vspherepolv1.PolicyEvaluationResult{
											{Tags: []string{policyTag1ID, policyTag2ID}}, // From policy 1
											{Tags: []string{policyTag3ID}},               // From policy 2
										},
										Conditions: []metav1.Condition{
											*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
										},
									},
								}
								withObjs = append(withObjs, policyEval)

								// Attach only some of the required policy tags
								Expect(tagMgr.AttachTag(ctx, policyTag1ID, moVM.Self)).To(Succeed())
								// policyTag2ID and policyTag3ID are still needed
							})

							It("should associate the missing tags with the vm", func() {
								initialTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
								Expect(err).ToNot(HaveOccurred())
								initialCount := len(initialTags)

								err = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
								Expect(err).ToNot(HaveOccurred())

								finalTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
								Expect(err).ToNot(HaveOccurred())
								finalTagIDs := make([]string, len(finalTags))
								for i, tag := range finalTags {
									finalTagIDs[i] = tag.ID
								}

								// Should have all original tags plus missing policy tags
								Expect(len(finalTags)).To(BeNumerically(">=", initialCount))
								// Should still have the original policy tag
								Expect(finalTagIDs).To(ContainElement(policyTag1ID))
								// Should have the missing policy tags
								Expect(finalTagIDs).To(ContainElement(policyTag2ID))
								Expect(finalTagIDs).To(ContainElement(policyTag3ID))

								// Should have existing policyTag1ID, and the two newly added
								// policyTag2ID and policyTag3ID policies
								ecTags, ok := object.OptionValueList(configSpec.ExtraConfig).
									GetString(vmconfpolicy.ExtraConfigPolicyTagsKey)
								Expect(ok).To(BeTrue())
								Expect(strings.Split(ecTags, ",")).To(ConsistOf(
									policyTag1ID, policyTag2ID, policyTag3ID))
							})
						})
					})
				})
			})

			When("a vm does not have existing tags", func() {
				When("the vm is not subject to any policy", func() {
					BeforeEach(func() {
						// Create a PolicyEvaluation with no policy results
						policyEval := &vspherepolv1.PolicyEvaluation{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:  vm.Namespace,
								Name:       "vm-" + vm.Name,
								Generation: 1,
								OwnerReferences: []metav1.OwnerReference{
									{
										APIVersion:         vm.APIVersion,
										Kind:               vm.Kind,
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
										GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
									},
								},
							},
							Status: vspherepolv1.PolicyEvaluationStatus{
								ObservedGeneration: 1,
								Policies:           []vspherepolv1.PolicyEvaluationResult{}, // No policies apply
								Conditions: []metav1.Condition{
									*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
								},
							},
						}
						withObjs = append(withObjs, policyEval)
					})

					It("should not associate any tags with the vm", func() {
						// Ensure VM has no tags
						initialTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
						Expect(err).ToNot(HaveOccurred())
						Expect(initialTags).To(BeEmpty())

						err = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
						Expect(err).ToNot(HaveOccurred())
						Expect(configSpec.ExtraConfig).To(BeEmpty())

						finalTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
						Expect(err).ToNot(HaveOccurred())
						Expect(finalTags).To(BeEmpty())

						// Ensure VM status has no policies.
						Expect(vm.Status.Policies).To(HaveLen(0))

						// Reconcile again so the VM status is updated.
						err = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
						Expect(err).ToNot(HaveOccurred())
						Expect(configSpec.ExtraConfig).To(BeEmpty())

						// Ensure VM status still has no policies.
						Expect(vm.Status.Policies).To(HaveLen(0))
					})
				})
				When("the vm is subject to a single policy", func() {
					BeforeEach(func() {
						// Create a PolicyEvaluation with single policy result
						policyEval := &vspherepolv1.PolicyEvaluation{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:  vm.Namespace,
								Name:       "vm-" + vm.Name,
								Generation: 1,
								OwnerReferences: []metav1.OwnerReference{
									{
										APIVersion:         vm.APIVersion,
										Kind:               vm.Kind,
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
										GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
									},
								},
							},
							Status: vspherepolv1.PolicyEvaluationStatus{
								ObservedGeneration: 1,
								Policies: []vspherepolv1.PolicyEvaluationResult{
									{
										APIVersion: vspherepolv1.GroupVersion.String(),
										Kind:       "ComputePolicy",
										Name:       "my-policy-1",
										Tags:       []string{policyTag1ID},
										Generation: 2,
									},
								},
								Conditions: []metav1.Condition{
									*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
								},
							},
						}
						withObjs = append(withObjs, policyEval)
					})

					It("should associate the policy's tags with the vm", func() {
						// Ensure VM starts with no tags
						initialTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
						Expect(err).ToNot(HaveOccurred())
						Expect(initialTags).To(BeEmpty())

						err = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
						Expect(err).ToNot(HaveOccurred())

						finalTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
						Expect(err).ToNot(HaveOccurred())
						finalTagIDs := make([]string, len(finalTags))
						for i, tag := range finalTags {
							finalTagIDs[i] = tag.ID
						}

						ecTags, ok := object.OptionValueList(configSpec.ExtraConfig).
							GetString(vmconfpolicy.ExtraConfigPolicyTagsKey)
						Expect(ok).To(BeTrue())
						Expect(strings.Split(ecTags, ",")).To(ConsistOf(policyTag1ID))

						// Should now have policy tags
						Expect(len(finalTags)).To(BeNumerically(">", 0))
						Expect(finalTagIDs).To(ContainElement(policyTag1ID))

						// Ensure the VM status was not updated yet.
						Expect(vm.Status.Policies).To(HaveLen(0))

						// Reconcile again so the VM status is updated.
						err = vmconfpolicy.Reconcile(
							pkgctx.WithVMTags(ctx, finalTagIDs),
							k8sClient,
							vimClient,
							vm,
							moVM,
							configSpec)
						Expect(err).ToNot(HaveOccurred())

						// Ensure VM status has the policy.
						Expect(vm.Status.Policies).To(ConsistOf(
							[]vmopv1.PolicyStatus{
								{
									PolicySpec: vmopv1.PolicySpec{
										APIVersion: vspherepolv1.GroupVersion.String(),
										Kind:       "ComputePolicy",
										Name:       "my-policy-1",
									},
									Generation: 2,
								},
							},
						))
					})
				})

				When("the vm is subject to multiple policies", func() {
					BeforeEach(func() {
						// Create a PolicyEvaluation with multiple policy results
						policyEval := &vspherepolv1.PolicyEvaluation{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:  vm.Namespace,
								Name:       "vm-" + vm.Name,
								Generation: 1,
								OwnerReferences: []metav1.OwnerReference{
									{
										APIVersion:         vm.APIVersion,
										Kind:               vm.Kind,
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
										GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
									},
								},
							},
							Status: vspherepolv1.PolicyEvaluationStatus{
								ObservedGeneration: 1,
								Policies: []vspherepolv1.PolicyEvaluationResult{
									{
										APIVersion: vspherepolv1.GroupVersion.String(),
										Kind:       "ComputePolicy",
										Name:       "my-policy-1",
										Tags:       []string{policyTag1ID, policyTag2ID},
										Generation: 1,
									},
									{
										APIVersion: vspherepolv1.GroupVersion.String(),
										Kind:       "ComputePolicy",
										Name:       "my-policy-2",
										Tags:       []string{policyTag3ID},
										Generation: 1,
									},
								},
								Conditions: []metav1.Condition{
									*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
								},
							},
						}
						withObjs = append(withObjs, policyEval)
					})

					It("should associate the policies' tags with the vm", func() {
						// Ensure VM starts with no tags
						initialTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
						Expect(err).ToNot(HaveOccurred())
						Expect(initialTags).To(BeEmpty())

						err = vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
						Expect(err).ToNot(HaveOccurred())

						finalTags, err := tagMgr.GetAttachedTags(ctx, moVM.Self)
						Expect(err).ToNot(HaveOccurred())
						finalTagIDs := make([]string, len(finalTags))
						for i, tag := range finalTags {
							finalTagIDs[i] = tag.ID
						}

						// Should have tags from multiple policies
						Expect(len(finalTags)).To(BeNumerically(">", 0))
						Expect(finalTagIDs).To(ContainElement(policyTag1ID))
						Expect(finalTagIDs).To(ContainElement(policyTag2ID))
						Expect(finalTagIDs).To(ContainElement(policyTag3ID))

						ecTags, ok := object.OptionValueList(configSpec.ExtraConfig).
							GetString(vmconfpolicy.ExtraConfigPolicyTagsKey)
						Expect(ok).To(BeTrue())
						Expect(strings.Split(ecTags, ",")).To(ConsistOf(
							policyTag1ID, policyTag2ID, policyTag3ID))

						// Ensure the VM status was not updated yet.
						Expect(vm.Status.Policies).To(HaveLen(0))

						// Reconcile again so the VM status is updated.
						err = vmconfpolicy.Reconcile(
							pkgctx.WithVMTags(ctx, finalTagIDs),
							k8sClient,
							vimClient,
							vm,
							moVM,
							configSpec)
						Expect(err).ToNot(HaveOccurred())

						// Ensure VM status has the policies.
						Expect(vm.Status.Policies).To(ConsistOf(
							[]vmopv1.PolicyStatus{
								{
									PolicySpec: vmopv1.PolicySpec{
										APIVersion: vspherepolv1.GroupVersion.String(),
										Kind:       "ComputePolicy",
										Name:       "my-policy-1",
									},
									Generation: 1,
								},
								{
									PolicySpec: vmopv1.PolicySpec{
										APIVersion: vspherepolv1.GroupVersion.String(),
										Kind:       "ComputePolicy",
										Name:       "my-policy-2",
									},
									Generation: 1,
								},
							},
						))
					})
				})
			})

			When("VM has image spec", func() {
				Context("with image found having labels", func() {
					BeforeEach(func() {
						vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
							Kind: "VirtualMachineImage",
							Name: "test-image",
						}
						image := &vmopv1.VirtualMachineImage{
							TypeMeta: metav1.TypeMeta{
								APIVersion: vmopv1.GroupVersion.String(),
								Kind:       "VirtualMachineImage",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-image",
								Namespace: vm.Namespace,
								Labels: map[string]string{
									"os":      "ubuntu",
									"version": "22.04",
								},
							},
						}
						withObjs = append(withObjs, image)

						// Create a ready PolicyEvaluation
						policyEval := &vspherepolv1.PolicyEvaluation{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:  vm.Namespace,
								Name:       "vm-" + vm.Name,
								Generation: 1,
								OwnerReferences: []metav1.OwnerReference{
									{
										APIVersion:         vm.APIVersion,
										Kind:               vm.Kind,
										Name:               vm.Name,
										UID:                vm.UID,
										Controller:         ptr.To(true),
										BlockOwnerDeletion: ptr.To(true),
									},
								},
							},
							Spec: vspherepolv1.PolicyEvaluationSpec{
								Image: &vspherepolv1.PolicyEvaluationImageSpec{
									Name: "test-image",
									Labels: map[string]string{
										"os":      "ubuntu",
										"version": "22.04",
									},
								},
								Workload: &vspherepolv1.PolicyEvaluationWorkloadSpec{
									Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
										GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
									},
								},
							},
							Status: vspherepolv1.PolicyEvaluationStatus{
								ObservedGeneration: 1,
								Policies:           []vspherepolv1.PolicyEvaluationResult{},
								Conditions: []metav1.Condition{
									*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
								},
							},
						}
						withObjs = append(withObjs, policyEval)
					})

					It("should process VM with image criteria", func() {
						err := vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				Context("with image not found for new VM", func() {
					BeforeEach(func() {
						// New VM (no Config)
						moVM = mo.VirtualMachine{}
						vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
							Kind: "VirtualMachineImage",
							Name: "missing-image",
						}
						// Image is not in withObjs, so it won't be found
					})

					It("should return error for new VM with missing image", func() {
						err := vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("failed to get VM"))
						Expect(err.Error()).To(ContainSubstring("image"))
					})
				})

				Context("with image not found for existing VM", func() {
					BeforeEach(func() {
						// Existing VM (has Config)
						vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
							Kind: "VirtualMachineImage",
							Name: "missing-image",
						}

						// Create a ready PolicyEvaluation without image spec
						policyEval := &vspherepolv1.PolicyEvaluation{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:  vm.Namespace,
								Name:       "vm-" + vm.Name,
								Generation: 1,
								OwnerReferences: []metav1.OwnerReference{
									{
										APIVersion:         vm.APIVersion,
										Kind:               vm.Kind,
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
										GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
									},
								},
							},
							Status: vspherepolv1.PolicyEvaluationStatus{
								ObservedGeneration: 1,
								Policies:           []vspherepolv1.PolicyEvaluationResult{},
								Conditions: []metav1.Condition{
									*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
								},
							},
						}
						withObjs = append(withObjs, policyEval)
					})

					It("should not error for existing VM with missing image", func() {
						err := vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				Context("with image found but no labels", func() {
					BeforeEach(func() {
						vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
							Kind: "VirtualMachineImage",
							Name: "test-image-no-labels",
						}
						image := &vmopv1.VirtualMachineImage{
							TypeMeta: metav1.TypeMeta{
								APIVersion: vmopv1.GroupVersion.String(),
								Kind:       "VirtualMachineImage",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-image-no-labels",
								Namespace: vm.Namespace,
								// No labels
							},
						}
						withObjs = append(withObjs, image)

						// Create a ready PolicyEvaluation
						policyEval := &vspherepolv1.PolicyEvaluation{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:  vm.Namespace,
								Name:       "vm-" + vm.Name,
								Generation: 1,
								OwnerReferences: []metav1.OwnerReference{
									{
										APIVersion:         vm.APIVersion,
										Kind:               vm.Kind,
										Name:               vm.Name,
										UID:                vm.UID,
										Controller:         ptr.To(true),
										BlockOwnerDeletion: ptr.To(true),
									},
								},
							},
							Spec: vspherepolv1.PolicyEvaluationSpec{
								Image: &vspherepolv1.PolicyEvaluationImageSpec{
									Name: "test-image-no-labels",
								},
								Workload: &vspherepolv1.PolicyEvaluationWorkloadSpec{
									Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
										GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
									},
								},
							},
							Status: vspherepolv1.PolicyEvaluationStatus{
								ObservedGeneration: 1,
								Policies:           []vspherepolv1.PolicyEvaluationResult{},
								Conditions: []metav1.Condition{
									*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
								},
							},
						}
						withObjs = append(withObjs, policyEval)
					})

					It("should process VM with image name but no labels", func() {
						err := vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
						Expect(err).ToNot(HaveOccurred())
					})
				})
			})

			When("VM has explicit policies", func() {
				BeforeEach(func() {
					vm.Spec.Policies = []vmopv1.PolicySpec{
						{
							APIVersion: "policy.vmware.com/v1",
							Kind:       "SecurityPolicy",
							Name:       "security-policy-1",
						},
						{
							APIVersion: "policy.vmware.com/v1",
							Kind:       "NetworkPolicy",
							Name:       "network-policy-1",
						},
					}

					// Create a ready PolicyEvaluation with policies
					policyEval := &vspherepolv1.PolicyEvaluation{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:  vm.Namespace,
							Name:       "vm-" + vm.Name,
							Generation: 1,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         vm.APIVersion,
									Kind:               vm.Kind,
									Name:               vm.Name,
									UID:                vm.UID,
									Controller:         ptr.To(true),
									BlockOwnerDeletion: ptr.To(true),
								},
							},
						},
						Spec: vspherepolv1.PolicyEvaluationSpec{
							Policies: []vspherepolv1.LocalObjectRef{
								{
									APIVersion: "policy.vmware.com/v1",
									Kind:       "SecurityPolicy",
									Name:       "security-policy-1",
								},
								{
									APIVersion: "policy.vmware.com/v1",
									Kind:       "NetworkPolicy",
									Name:       "network-policy-1",
								},
							},
							Workload: &vspherepolv1.PolicyEvaluationWorkloadSpec{
								Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
									GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
								},
							},
						},
						Status: vspherepolv1.PolicyEvaluationStatus{
							ObservedGeneration: 1,
							Policies:           []vspherepolv1.PolicyEvaluationResult{},
							Conditions: []metav1.Condition{
								*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
							},
						},
					}
					withObjs = append(withObjs, policyEval)
				})

				It("should process VM with explicit policies", func() {
					err := vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			When("VM has workload labels", func() {
				BeforeEach(func() {
					vm.Labels = map[string]string{
						"app":  "web-server",
						"tier": "frontend",
					}

					// Create a ready PolicyEvaluation with workload labels
					policyEval := &vspherepolv1.PolicyEvaluation{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:  vm.Namespace,
							Name:       "vm-" + vm.Name,
							Generation: 1,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         vm.APIVersion,
									Kind:               vm.Kind,
									Name:               vm.Name,
									UID:                vm.UID,
									Controller:         ptr.To(true),
									BlockOwnerDeletion: ptr.To(true),
								},
							},
						},
						Spec: vspherepolv1.PolicyEvaluationSpec{
							Workload: &vspherepolv1.PolicyEvaluationWorkloadSpec{
								Labels: map[string]string{
									"app":  "web-server",
									"tier": "frontend",
								},
								Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
									GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
								},
							},
						},
						Status: vspherepolv1.PolicyEvaluationStatus{
							ObservedGeneration: 1,
							Policies:           []vspherepolv1.PolicyEvaluationResult{},
							Conditions: []metav1.Condition{
								*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
							},
						},
					}
					withObjs = append(withObjs, policyEval)
				})

				It("should process VM with workload labels", func() {
					err := vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			When("guestFamily is empty but guestID is provided", func() {
				BeforeEach(func() {
					// Clear guestFamily but keep guestID
					moVM.Guest.GuestFamily = ""
					moVM.Guest.GuestId = "windows9_64Guest"

					// Create a ready PolicyEvaluation with derived guest family
					policyEval := &vspherepolv1.PolicyEvaluation{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:  vm.Namespace,
							Name:       "vm-" + vm.Name,
							Generation: 1,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         vm.APIVersion,
									Kind:               vm.Kind,
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
									GuestID:     "windows9_64Guest",
									GuestFamily: vspherepolv1.GuestFamilyTypeWindows,
								},
							},
						},
						Status: vspherepolv1.PolicyEvaluationStatus{
							ObservedGeneration: 1,
							Policies:           []vspherepolv1.PolicyEvaluationResult{},
							Conditions: []metav1.Condition{
								*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
							},
						},
					}
					withObjs = append(withObjs, policyEval)
				})

				It("should derive guestFamily from guestID", func() {
					err := vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			When("VM has no guest info", func() {
				BeforeEach(func() {
					// Create a moVM without any guest info configured
					moVM.Guest = &vimtypes.GuestInfo{
						GuestId:     "",
						GuestFamily: "",
					}
					configSpec.GuestId = ""

					// Create a ready PolicyEvaluation without workload spec
					policyEval := &vspherepolv1.PolicyEvaluation{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:  vm.Namespace,
							Name:       "vm-" + vm.Name,
							Generation: 1,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         vm.APIVersion,
									Kind:               vm.Kind,
									Name:               vm.Name,
									UID:                vm.UID,
									Controller:         ptr.To(true),
									BlockOwnerDeletion: ptr.To(true),
								},
							},
						},
						Spec: vspherepolv1.PolicyEvaluationSpec{},
						Status: vspherepolv1.PolicyEvaluationStatus{
							ObservedGeneration: 1,
							Policies:           []vspherepolv1.PolicyEvaluationResult{},
							Conditions: []metav1.Condition{
								*pkgcond.TrueCondition(vspherepolv1.ReadyConditionType),
							},
						},
					}
					withObjs = append(withObjs, policyEval)
				})

				It("should process VM without guest info", func() {
					err := vmconfpolicy.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})
})

func tagSpec(
	op vimtypes.ArrayUpdateOperation, //nolint:unparam
	uuid string) vimtypes.TagSpec {

	return vimtypes.TagSpec{
		ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{
			Operation: op,
		},
		Id: vimtypes.TagId{
			Uuid: uuid,
		},
	}
}
