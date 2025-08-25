// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package policyevaluation_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	apirecord "k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/vspherepolicy/policyevaluation"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

var _ = Describe("AddToManager", func() {
	var (
		ctx context.Context
		mgr manager.Manager
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create a scheme with the necessary types
		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(vspherepolv1.AddToScheme(scheme)).To(Succeed())

		// Create a fake manager
		var err error
		mgr, err = ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:         scheme,
			LeaderElection: false,
		})
		Expect(err).ToNot(HaveOccurred())
	})

	It("should successfully add controller to manager", func() {
		// Create controller manager context
		ctrlCtx := &pkgctx.ControllerManagerContext{
			Context:   ctx,
			Namespace: "test-namespace",
			Name:      "test-manager",
		}

		// AddToManager should succeed
		err := policyevaluation.AddToManager(ctrlCtx, mgr)
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("Reconcile", func() {
	var (
		ctx        context.Context
		client     ctrlclient.Client
		reconciler *policyevaluation.Reconciler
		obj        *vspherepolv1.PolicyEvaluation
		namespace  string

		withObjs  []ctrlclient.Object
		withFuncs interceptor.Funcs
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()
		namespace = "test-namespace"

		withObjs = nil
		withFuncs = interceptor.Funcs{}

		obj = &vspherepolv1.PolicyEvaluation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy-eval",
				Namespace: namespace,
			},
			Spec: vspherepolv1.PolicyEvaluationSpec{},
		}
	})

	JustBeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(vspherepolv1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(
				&vspherepolv1.PolicyEvaluation{},
				&vspherepolv1.ComputePolicy{},
				&vspherepolv1.TagPolicy{},
			).
			WithObjects(withObjs...).
			WithInterceptorFuncs(withFuncs).
			Build()

		reconciler = policyevaluation.NewReconciler(
			ctx,
			client,
			log.Log.WithName("test"),
			record.New(apirecord.NewFakeRecorder(100)),
		)
	})

	Context("when PolicyEvaluation does not exist", func() {
		It("should return without error", func() {
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent",
					Namespace: namespace,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("when PolicyEvaluation exists", func() {
		BeforeEach(func() {
			withObjs = append(withObjs, obj)
		})

		It("should add finalizer on first reconcile", func() {
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      obj.Name,
					Namespace: obj.Namespace,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			var updated vspherepolv1.PolicyEvaluation
			Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement(policyevaluation.Finalizer))
		})

		Context("with finalizer already present", func() {
			BeforeEach(func() {
				obj.Finalizers = []string{policyevaluation.Finalizer}
			})

			It("should reconcile normally without compute policies", func() {
				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      obj.Name,
						Namespace: obj.Namespace,
					},
				}

				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var updated vspherepolv1.PolicyEvaluation
				Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
				Expect(updated.Status.Policies).To(BeEmpty())
			})

			Context("with matching compute policy", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-compute-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
						},
					}
					withObjs = append(withObjs, computePolicy)
				})

				It("should add matching policy to status", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(1))
					Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
					Expect(updated.Status.Policies[0].Kind).To(Equal("ComputePolicy"))
					Expect(updated.Status.Policies[0].Tags).To(BeEmpty())
				})
			})

			Context("with compute policy that has guest matching", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-compute-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Workload: &vspherepolv1.MatchWorkloadSpec{
									Guest: &vspherepolv1.MatchGuestSpec{
										GuestID: &vspherepolv1.StringMatcherSpec{
											Value: "ubuntu64Guest",
										},
										GuestFamily: &vspherepolv1.GuestFamilyMatcherSpec{
											Value: vspherepolv1.GuestFamilyTypeLinux,
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)
				})

				Context("when PolicyEvaluation has no guest info", func() {
					It("should not match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(BeEmpty())
					})
				})

				Context("when PolicyEvaluation has matching guest info", func() {
					BeforeEach(func() {
						obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
							Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
								GuestID:     "ubuntu64Guest",
								GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
							},
						}
					})

					It("should match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(HaveLen(1))
						Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
						Expect(updated.Status.Policies[0].Kind).To(Equal("ComputePolicy"))
					})
				})

				Context("when PolicyEvaluation has non-matching guest ID", func() {
					BeforeEach(func() {
						obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
							Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
								GuestID:     "windows9_64Guest",
								GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
							},
						}
					})

					It("should not match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(BeEmpty())
					})
				})

				Context("when PolicyEvaluation has non-matching guest family", func() {
					BeforeEach(func() {
						obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
							Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
								GuestID:     "ubuntu64Guest",
								GuestFamily: vspherepolv1.GuestFamilyTypeWindows,
							},
						}
					})

					It("should not match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(BeEmpty())
					})
				})
			})

			Context("with compute policy that has label matching", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-compute-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Workload: &vspherepolv1.MatchWorkloadSpec{
									Labels: []metav1.LabelSelectorRequirement{
										{
											Key:      "app",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"nginx"},
										},
										{
											Key:      "tier",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"web"},
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)
				})

				Context("when PolicyEvaluation has no labels", func() {
					It("should not match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(BeEmpty())
					})
				})

				Context("when PolicyEvaluation has matching labels", func() {
					BeforeEach(func() {
						obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
							Labels: map[string]string{
								"app":     "nginx",
								"tier":    "web",
								"version": "1.0",
							},
						}
					})

					It("should match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(HaveLen(1))
						Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
						Expect(updated.Status.Policies[0].Kind).To(Equal("ComputePolicy"))
					})
				})

				Context("when PolicyEvaluation has partial matching labels", func() {
					BeforeEach(func() {
						obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
							Labels: map[string]string{
								"app": "nginx",
							},
						}
					})

					It("should not match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(BeEmpty())
					})
				})

				Context("when PolicyEvaluation has non-matching label values", func() {
					BeforeEach(func() {
						obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
							Labels: map[string]string{
								"app":  "apache",
								"tier": "web",
							},
						}
					})

					It("should not match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(BeEmpty())
					})
				})
			})

			Context("with compute policy that has tags", func() {
				var (
					computePolicy *vspherepolv1.ComputePolicy
					tagPolicy     *vspherepolv1.TagPolicy
				)

				BeforeEach(func() {
					tagPolicy = &vspherepolv1.TagPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-tag-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.TagPolicySpec{
							Tags: []string{
								"uuid1",
								"uuid2",
							},
						},
					}
					withObjs = append(withObjs, tagPolicy)

					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-compute-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Tags: []string{"test-tag-policy"},
						},
					}
					withObjs = append(withObjs, computePolicy)
				})

				It("should include tags from TagPolicy in result", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(1))
					Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
					Expect(updated.Status.Policies[0].Kind).To(Equal("ComputePolicy"))
					Expect(updated.Status.Policies[0].Tags).To(ConsistOf(
						"uuid1",
						"uuid2"))
				})

				Context("when TagPolicy does not exist", func() {
					BeforeEach(func() {
						// Remove tagPolicy from withObjs so it doesn't exist in the fake client
						withObjs = withObjs[:len(withObjs)-2] // Remove both tagPolicy and computePolicy
						// Add back only computePolicy without tagPolicy
						withObjs = append(withObjs, computePolicy)
					})

					It("should return error", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("failed to get tag policy"))
						Expect(result).To(Equal(ctrl.Result{}))
					})
				})
			})

			Context("with multiple compute policies", func() {
				BeforeEach(func() {
					policies := []*vspherepolv1.ComputePolicy{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "policy-1",
								Namespace: namespace,
							},
							Spec: vspherepolv1.ComputePolicySpec{
								Type: vspherepolv1.PolicyTypeMandatory,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "policy-2",
								Namespace: namespace,
							},
							Spec: vspherepolv1.ComputePolicySpec{
								Type: vspherepolv1.PolicyTypeOptional,
								Match: &vspherepolv1.MatchSpec{
									Workload: &vspherepolv1.MatchWorkloadSpec{
										Labels: []metav1.LabelSelectorRequirement{
											{
												Key:      "env",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"prod"},
											},
										},
									},
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "policy-3",
								Namespace: namespace,
							},
							Spec: vspherepolv1.ComputePolicySpec{
								Type: vspherepolv1.PolicyTypeMandatory,
								Match: &vspherepolv1.MatchSpec{
									Workload: &vspherepolv1.MatchWorkloadSpec{
										Labels: []metav1.LabelSelectorRequirement{
											{
												Key:      "env",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"dev"},
											},
										},
									},
								},
							},
						},
					}

					for _, policy := range policies {
						withObjs = append(withObjs, policy)
					}

					obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
						Labels: map[string]string{
							"env": "prod",
						},
					}
				})

				It("should match multiple policies based on criteria", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(2))

					policyNames := make([]string, len(updated.Status.Policies))
					for i, policy := range updated.Status.Policies {
						policyNames[i] = policy.Name
						Expect(policy.Kind).To(Equal("ComputePolicy"))
					}
					Expect(policyNames).To(ConsistOf("policy-1", "policy-2"))
				})
			})

			Context("with compute policy that has no matching criteria", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "no-criteria-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							// No Match spec - should match everything
						},
					}
					withObjs = append(withObjs, computePolicy)
				})

				It("should match any PolicyEvaluation", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(1))
					Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
				})
			})

			Context("with compute policy that has empty match spec", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "empty-match-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type:  vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								// Empty match spec - should match everything
							},
						},
					}
					withObjs = append(withObjs, computePolicy)
				})

				It("should match any PolicyEvaluation", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(1))
					Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
				})
			})

			Context("with compute policy that has empty workload match spec", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "empty-workload-match-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Workload: &vspherepolv1.MatchWorkloadSpec{
									// Empty workload spec - should match everything
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)
				})

				It("should match any PolicyEvaluation", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(1))
					Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
				})
			})

			Context("with compute policy requiring guest but PolicyEvaluation has nil workload", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "guest-required-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Workload: &vspherepolv1.MatchWorkloadSpec{
									Guest: &vspherepolv1.MatchGuestSpec{
										GuestID: &vspherepolv1.StringMatcherSpec{
											Value: "ubuntu64Guest",
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)

					// Ensure obj.Spec.Workload is nil
					obj.Spec.Workload = nil
				})

				It("should not match the policy", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(BeEmpty())
				})
			})

			Context("with compute policy requiring guest family but PolicyEvaluation has nil guest", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "guest-family-required-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Workload: &vspherepolv1.MatchWorkloadSpec{
									Guest: &vspherepolv1.MatchGuestSpec{
										GuestFamily: &vspherepolv1.GuestFamilyMatcherSpec{
											Value: vspherepolv1.GuestFamilyTypeLinux,
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)

					// Workload exists but Guest is nil
					obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
						Labels: map[string]string{
							"app": "test",
						},
						// Guest is nil
					}
				})

				It("should not match the policy", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(BeEmpty())
				})
			})

			Context("with compute policy that has image label matching", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-compute-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Image: &vspherepolv1.MatchImageSpec{
									Labels: []metav1.LabelSelectorRequirement{
										{
											Key:      "version",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"1.0"},
										},
										{
											Key:      "arch",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"amd64"},
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)
				})

				Context("when PolicyEvaluation has no image info", func() {
					It("should not match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(BeEmpty())
					})
				})

				Context("when PolicyEvaluation has matching image labels", func() {
					BeforeEach(func() {
						obj.Spec.Image = &vspherepolv1.PolicyEvaluationImageSpec{
							Labels: map[string]string{
								"version": "1.0",
								"arch":    "amd64",
								"os":      "ubuntu",
							},
						}
					})

					It("should match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(HaveLen(1))
						Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
						Expect(updated.Status.Policies[0].Kind).To(Equal("ComputePolicy"))
					})
				})

				Context("when PolicyEvaluation has non-matching image labels", func() {
					BeforeEach(func() {
						obj.Spec.Image = &vspherepolv1.PolicyEvaluationImageSpec{
							Labels: map[string]string{
								"version": "2.0",
								"arch":    "arm64",
							},
						}
					})

					It("should not match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(BeEmpty())
					})
				})
			})

			Context("with explicit policies", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "explicit-compute-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Workload: &vspherepolv1.MatchWorkloadSpec{
									Labels: []metav1.LabelSelectorRequirement{
										{
											Key:      "env",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"dev"}, // Won't match our test object
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)

					obj.Spec.Policies = []vspherepolv1.LocalObjectRef{
						{
							APIVersion: "vsphere.policy.vmware.com/v1alpha1",
							Kind:       "ComputePolicy",
							Name:       "explicit-compute-policy",
						},
					}
				})

				It("should include explicitly referenced policies even if they don't match", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(1))
					Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
					Expect(updated.Status.Policies[0].Kind).To(Equal("ComputePolicy"))
				})

				Context("when explicit policy does not exist", func() {
					BeforeEach(func() {
						// Remove the compute policy from withObjs
						withObjs = withObjs[:len(withObjs)-1]
					})

					It("should return error", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("failed to add explicit compute policy"))
						Expect(err.Error()).To(ContainSubstring("failed to get compute policy"))
						Expect(result).To(Equal(ctrl.Result{}))
					})
				})

				Context("with unknown policy kind", func() {
					BeforeEach(func() {
						obj.Spec.Policies = []vspherepolv1.LocalObjectRef{
							{
								APIVersion: "unknown.api/v1",
								Kind:       "UnknownPolicy",
								Name:       "unknown-policy",
							},
						}
					})

					It("should skip unknown policy kinds without error", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(BeEmpty())
					})
				})
			})

			Context("with duplicate policies", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "duplicate-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
						},
					}
					withObjs = append(withObjs, computePolicy)

					// Add the same policy twice in explicit policies
					obj.Spec.Policies = []vspherepolv1.LocalObjectRef{
						{
							APIVersion: "vsphere.policy.vmware.com/v1alpha1",
							Kind:       "ComputePolicy",
							Name:       "duplicate-policy",
						},
						{
							APIVersion: "vsphere.policy.vmware.com/v1alpha1",
							Kind:       "ComputePolicy",
							Name:       "duplicate-policy",
						},
					}
				})

				It("should only include each policy once", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					// Should only have 2 entries: one from automatic matching, one from explicit
					// but since they're the same policy, duplicate prevention should result in only 1
					Expect(updated.Status.Policies).To(HaveLen(1))
					Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
					Expect(updated.Status.Policies[0].Kind).To(Equal("ComputePolicy"))
				})
			})

			Context("with combined workload and image matching", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "combined-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Workload: &vspherepolv1.MatchWorkloadSpec{
									Labels: []metav1.LabelSelectorRequirement{
										{
											Key:      "app",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"nginx"},
										},
									},
								},
								Image: &vspherepolv1.MatchImageSpec{
									Labels: []metav1.LabelSelectorRequirement{
										{
											Key:      "version",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"1.0"},
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)
				})

				Context("when both workload and image labels match", func() {
					BeforeEach(func() {
						obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
							Labels: map[string]string{
								"app": "nginx",
								"env": "prod",
							},
						}
						obj.Spec.Image = &vspherepolv1.PolicyEvaluationImageSpec{
							Labels: map[string]string{
								"version": "1.0",
								"arch":    "amd64",
							},
						}
					})

					It("should match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(HaveLen(1))
						Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
					})
				})

				Context("when workload matches but image doesn't", func() {
					BeforeEach(func() {
						obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
							Labels: map[string]string{
								"app": "nginx",
							},
						}
						obj.Spec.Image = &vspherepolv1.PolicyEvaluationImageSpec{
							Labels: map[string]string{
								"version": "2.0", // Different version
							},
						}
					})

					It("should not match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(BeEmpty())
					})
				})
			})

			Context("with policies that have only guest ID specified", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "guest-id-only-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Workload: &vspherepolv1.MatchWorkloadSpec{
									Guest: &vspherepolv1.MatchGuestSpec{
										GuestID: &vspherepolv1.StringMatcherSpec{
											Value: "ubuntu64Guest",
										},
										// GuestFamily is empty
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)
				})

				Context("when PolicyEvaluation has matching guest ID", func() {
					BeforeEach(func() {
						obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
							Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
								GuestID:     "ubuntu64Guest",
								GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
							},
						}
					})

					It("should match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(HaveLen(1))
						Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
					})
				})
			})

			Context("with policies that have only guest family specified", func() {
				var computePolicy *vspherepolv1.ComputePolicy

				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "guest-family-only-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Workload: &vspherepolv1.MatchWorkloadSpec{
									Guest: &vspherepolv1.MatchGuestSpec{
										// GuestID is empty
										GuestFamily: &vspherepolv1.GuestFamilyMatcherSpec{
											Value: vspherepolv1.GuestFamilyTypeLinux,
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)
				})

				Context("when PolicyEvaluation has matching guest family", func() {
					BeforeEach(func() {
						obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
							Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
								GuestID:     "ubuntu64Guest",
								GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
							},
						}
					})

					It("should match the policy", func() {
						req := ctrl.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.Name,
								Namespace: obj.Namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))

						var updated vspherepolv1.PolicyEvaluation
						Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
						Expect(updated.Status.Policies).To(HaveLen(1))
						Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
					})
				})
			})
		})
	})

	Context("when PolicyEvaluation is being deleted", func() {
		var deletingPolicyEval *vspherepolv1.PolicyEvaluation

		BeforeEach(func() {
			deletingPolicyEval = &vspherepolv1.PolicyEvaluation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "deleting-policy-eval",
					Namespace:  namespace,
					Finalizers: []string{policyevaluation.Finalizer},
				},
				Spec: vspherepolv1.PolicyEvaluationSpec{},
			}
			now := metav1.Now()
			deletingPolicyEval.DeletionTimestamp = &now
			withObjs = append(withObjs, deletingPolicyEval)
		})

		It("should successfully reconcile deletion", func() {
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      deletingPolicyEval.Name,
					Namespace: deletingPolicyEval.Namespace,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should successfully handle deletion without error", func() {
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      deletingPolicyEval.Name,
					Namespace: deletingPolicyEval.Namespace,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// The object should be successfully processed for deletion
			// Note: We can't check the finalizer was removed because the patch
			// operation in the fake client removes the object when finalizers are cleared
		})
	})

	Context("error scenarios", func() {
		BeforeEach(func() {
			obj.Finalizers = []string{policyevaluation.Finalizer}
			withObjs = append(withObjs, obj)
		})

		Context("when compute policy list fails", func() {
			BeforeEach(func() {
				withFuncs = interceptor.Funcs{
					List: func(ctx context.Context, client ctrlclient.WithWatch, list ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
						if _, ok := list.(*vspherepolv1.ComputePolicyList); ok {
							return fmt.Errorf("failed to list compute policies")
						}
						return client.List(ctx, list, opts...)
					},
				}
			})

			It("should return error", func() {
				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      obj.Name,
						Namespace: obj.Namespace,
					},
				}

				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to reconcile compute policies"))
				Expect(err.Error()).To(ContainSubstring("failed to list compute policies"))
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})

		Context("when multiple tag policies with same tags", func() {
			var (
				computePolicy *vspherepolv1.ComputePolicy
				tagPolicy1    *vspherepolv1.TagPolicy
				tagPolicy2    *vspherepolv1.TagPolicy
			)

			BeforeEach(func() {
				tagPolicy1 = &vspherepolv1.TagPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tag-policy-1",
						Namespace: namespace,
					},
					Spec: vspherepolv1.TagPolicySpec{
						Tags: []string{
							"uuid1",
							"uuid2",
						},
					},
				}
				withObjs = append(withObjs, tagPolicy1)

				tagPolicy2 = &vspherepolv1.TagPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tag-policy-2",
						Namespace: namespace,
					},
					Spec: vspherepolv1.TagPolicySpec{
						Tags: []string{
							"uuid3",
							"uuid4",
						},
					},
				}
				withObjs = append(withObjs, tagPolicy2)

				computePolicy = &vspherepolv1.ComputePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multi-tag-policy",
						Namespace: namespace,
					},
					Spec: vspherepolv1.ComputePolicySpec{
						Type: vspherepolv1.PolicyTypeMandatory,
						Tags: []string{"tag-policy-1", "tag-policy-2"},
					},
				}
				withObjs = append(withObjs, computePolicy)
			})

			It("should include tags from multiple tag policies", func() {
				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      obj.Name,
						Namespace: obj.Namespace,
					},
				}

				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var updated vspherepolv1.PolicyEvaluation
				Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
				Expect(updated.Status.Policies).To(HaveLen(1))
				Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
				Expect(updated.Status.Policies[0].Kind).To(Equal("ComputePolicy"))
				Expect(updated.Status.Policies[0].Tags).To(ConsistOf(
					"uuid1", "uuid2", "uuid3", "uuid4"))
			})
		})

		Context("nested MatchSpec with boolean operations", func() {
			var computePolicy *vspherepolv1.ComputePolicy

			Context("with boolean AND operation", func() {
				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "nested-and-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Op: vspherepolv1.MatchesBooleanAnd,
								Workload: &vspherepolv1.MatchWorkloadSpec{
									Labels: []metav1.LabelSelectorRequirement{
										{
											Key:      "app",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"nginx"},
										},
									},
								},
								Match: []vspherepolv1.MatchSpec{
									{
										Workload: &vspherepolv1.MatchWorkloadSpec{
											Guest: &vspherepolv1.MatchGuestSpec{
												GuestFamily: &vspherepolv1.GuestFamilyMatcherSpec{
													Value: vspherepolv1.GuestFamilyTypeLinux,
												},
											},
										},
									},
									{
										Image: &vspherepolv1.MatchImageSpec{
											Name: &vspherepolv1.StringMatcherSpec{
												Value: "nginx:1.20",
											},
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)

					obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
						Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
							GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
						},
						Labels: map[string]string{
							"app": "nginx",
						},
					}
					obj.Spec.Image = &vspherepolv1.PolicyEvaluationImageSpec{
						Name: "nginx:1.20",
					}
				})

				It("should match when all nested conditions are true", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(1))
					Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
				})

				It("should not match when one nested condition fails", func() {
					// Change image name to break one nested condition
					obj.Spec.Image.Name = "nginx:1.19"
					Expect(client.Update(ctx, obj)).To(Succeed())

					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(0))
				})
			})

			Context("with boolean OR operation", func() {
				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "nested-or-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Op: vspherepolv1.MatchesBooleanOr,
								Match: []vspherepolv1.MatchSpec{
									{
										Workload: &vspherepolv1.MatchWorkloadSpec{
											Guest: &vspherepolv1.MatchGuestSpec{
												GuestFamily: &vspherepolv1.GuestFamilyMatcherSpec{
													Value: vspherepolv1.GuestFamilyTypeWindows,
												},
											},
										},
									},
									{
										Image: &vspherepolv1.MatchImageSpec{
											Name: &vspherepolv1.StringMatcherSpec{
												Value: "nginx:1.20",
											},
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)

					obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
						Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
							GuestFamily: vspherepolv1.GuestFamilyTypeLinux, // Not Windows
						},
					}
					obj.Spec.Image = &vspherepolv1.PolicyEvaluationImageSpec{
						Name: "nginx:1.20", // This matches
					}
				})

				It("should match when any nested condition is true", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(1))
					Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
				})

				It("should not match when all nested conditions fail", func() {
					// Change both conditions to fail
					obj.Spec.Workload.Guest.GuestFamily = vspherepolv1.GuestFamilyTypeLinux // Not Windows
					obj.Spec.Image.Name = "apache:2.4"                                     // Not nginx:1.20
					Expect(client.Update(ctx, obj)).To(Succeed())

					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(0))
				})
			})

			Context("with deeply nested MatchSpec", func() {
				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "deeply-nested-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Op: vspherepolv1.MatchesBooleanAnd,
								Match: []vspherepolv1.MatchSpec{
									{
										Op: vspherepolv1.MatchesBooleanOr,
										Match: []vspherepolv1.MatchSpec{
											{
												Workload: &vspherepolv1.MatchWorkloadSpec{
													Guest: &vspherepolv1.MatchGuestSpec{
														GuestFamily: &vspherepolv1.GuestFamilyMatcherSpec{
															Value: vspherepolv1.GuestFamilyTypeLinux,
														},
													},
												},
											},
											{
												Workload: &vspherepolv1.MatchWorkloadSpec{
													Guest: &vspherepolv1.MatchGuestSpec{
														GuestFamily: &vspherepolv1.GuestFamilyMatcherSpec{
															Value: vspherepolv1.GuestFamilyTypeWindows,
														},
													},
												},
											},
										},
									},
									{
										Image: &vspherepolv1.MatchImageSpec{
											Labels: []metav1.LabelSelectorRequirement{
												{
													Key:      "version",
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{"1.20"},
												},
											},
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)

					obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
						Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
							GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
						},
					}
					obj.Spec.Image = &vspherepolv1.PolicyEvaluationImageSpec{
						Labels: map[string]string{
							"version": "1.20",
						},
					}
				})

				It("should handle deeply nested boolean operations correctly", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(1))
					Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
				})
			})

			Context("with mixed workload and image conditions at root level", func() {
				BeforeEach(func() {
					computePolicy = &vspherepolv1.ComputePolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "mixed-conditions-policy",
							Namespace: namespace,
						},
						Spec: vspherepolv1.ComputePolicySpec{
							Type: vspherepolv1.PolicyTypeMandatory,
							Match: &vspherepolv1.MatchSpec{
								Workload: &vspherepolv1.MatchWorkloadSpec{
									Labels: []metav1.LabelSelectorRequirement{
										{
											Key:      "tier",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"web"},
										},
									},
								},
								Image: &vspherepolv1.MatchImageSpec{
									Name: &vspherepolv1.StringMatcherSpec{
										Value: "nginx:1.20",
									},
								},
								Match: []vspherepolv1.MatchSpec{
									{
										Workload: &vspherepolv1.MatchWorkloadSpec{
											Guest: &vspherepolv1.MatchGuestSpec{
												GuestFamily: &vspherepolv1.GuestFamilyMatcherSpec{
													Value: vspherepolv1.GuestFamilyTypeLinux,
												},
											},
										},
									},
								},
							},
						},
					}
					withObjs = append(withObjs, computePolicy)

					obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
						Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
							GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
						},
						Labels: map[string]string{
							"tier": "web",
						},
					}
					obj.Spec.Image = &vspherepolv1.PolicyEvaluationImageSpec{
						Name: "nginx:1.20",
					}
				})

				It("should match when both root level and nested conditions are true", func() {
					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(1))
					Expect(updated.Status.Policies[0].Name).To(Equal(computePolicy.Name))
				})

				It("should not match when root level condition fails", func() {
					// Change root level workload condition to fail
					obj.Spec.Workload.Labels["tier"] = "database"
					Expect(client.Update(ctx, obj)).To(Succeed())

					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(0))
				})

				It("should not match when nested condition fails", func() {
					// Change nested workload guest condition to fail
					obj.Spec.Workload.Guest.GuestFamily = vspherepolv1.GuestFamilyTypeWindows
					Expect(client.Update(ctx, obj)).To(Succeed())

					req := ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.Name,
							Namespace: obj.Namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))

					var updated vspherepolv1.PolicyEvaluation
					Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), &updated)).To(Succeed())
					Expect(updated.Status.Policies).To(HaveLen(0))
				})
			})
		})
	})
})
