// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providerconfigmap_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/providerconfigmap"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking ConfigMap controller integration tests", intgTestsCM)
}

func intgTestsCM() {
	var (
		ctx *builder.IntegrationTestContext

		cm                *corev1.ConfigMap
		clUUID            = "dummy-cl"
		contentSourceKind = "ContentSource"
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.ProviderConfigMapName,
				Namespace: ctx.PodNamespace,
			},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {
		clExists := func(clName string) bool {
			clObj := &vmopv1.ContentLibraryProvider{}
			return ctx.Client.Get(ctx, client.ObjectKey{Name: clName}, clObj) == nil
		}

		// Verifies that a ContentSource exists, has a matching label and has a ProviderRef set to the content library..
		csExists := func(csName, clName string) bool {
			csObj := &vmopv1.ContentSource{}
			if err := ctx.Client.Get(ctx, client.ObjectKey{Name: csName}, csObj); err != nil {
				return false
			}

			return csObj.Spec.ProviderRef.Name == clName &&
				csObj.Labels[providerconfigmap.TKGContentSourceLabelKey] == providerconfigmap.TKGContentSourceLabelValue
		}

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, cm)).To(Succeed())
		})

		JustAfterEach(func() {
			err := ctx.Client.Delete(ctx, cm)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		When("ConfigMap is created without ContentSource key", func() {
			It("no ContentSource is created", func() {
				csList := &vmopv1.ContentSourceList{}
				err := ctx.Client.List(ctx, csList)
				Expect(err).NotTo(HaveOccurred())
				Expect(csList.Items).To(HaveLen(0))
			})
		})

		When("ConfigMap is created with ContentSource key", func() {
			BeforeEach(func() {
				cm.Data = make(map[string]string)
				cm.Data[config.ContentSourceKey] = clUUID
			})

			It("a ContentSource is created", func() {
				Eventually(func() bool {
					return csExists(clUUID, clUUID) && clExists(clUUID)
				}).Should(BeTrue())

				// Validate that no ContentSources exist in the system namespace.
				Consistently(func() bool {
					bindingList := &vmopv1.ContentSourceBindingList{}
					Expect(ctx.Client.List(ctx, bindingList, client.InNamespace(ctx.PodNamespace))).To(Succeed())
					return len(bindingList.Items) == 0
				}).Should(BeTrue())
			})
		})

		When("ConfigMap's ContentSource key is updated", func() {
			BeforeEach(func() {
				cm.Data = make(map[string]string)
				cm.Data[config.ContentSourceKey] = clUUID
			})

			It("A ContentSource is created that points to the new CL UUID from ConfigMap", func() {
				// Wait for the initial ContentSources to be available.
				Eventually(func() bool {
					return csExists(clUUID, clUUID) && clExists(clUUID)
				}).Should(BeTrue())

				newCLUUID := "new-cl"
				cm.Data[config.ContentSourceKey] = newCLUUID
				Expect(ctx.Client.Update(ctx, cm)).NotTo(HaveOccurred())

				Eventually(func() bool {
					return csExists(newCLUUID, newCLUUID)
				}).Should(BeTrue())

				// For now, only verify that the custom resources exist. Ideally, we should also check that no additional resources are present.
				// However, it is not possible to do that because the OwnerRef is maintained by the ContentSource controller that is not running
				// in this suite.
				Eventually(func() bool {
					return clExists(newCLUUID)
				}).Should(BeTrue())
			})
		})

		Context("VMService FSS is enabled", func() {
			verifyContentSourceBinding := func(namespace string) {
				Eventually(func() bool {
					bindingList := &vmopv1.ContentSourceBindingList{}
					err := ctx.Client.List(ctx, bindingList, client.InNamespace(namespace))
					return err == nil && len(bindingList.Items) == 1 && bindingList.Items[0].ContentSourceRef.Kind == contentSourceKind &&
						bindingList.Items[0].ContentSourceRef.Name == clUUID
				}).Should(BeTrue())
			}

			When("ConfigMap is created with a ContentSource key", func() {
				var workloadNs *corev1.Namespace
				BeforeEach(func() {
					cm.Data = make(map[string]string)
					cm.Data[config.ContentSourceKey] = clUUID

					// Create a user workload namespace.
					// Note: the namespace won't be deleted immediately after calling .Delete,
					// using random namespace name instead, so that we have a new name for each test.
					workloadNs = &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "user-workload-ns-" + uuid.New().String(),
							Labels: map[string]string{
								providerconfigmap.UserWorkloadNamespaceLabel: "cluster-moid",
							},
						},
					}
					Expect(ctx.Client.Create(ctx, workloadNs)).To(Succeed())
				})

				AfterEach(func() {
					err := ctx.Client.Delete(ctx, workloadNs)
					Expect(err == nil || k8serrors.IsNotFound(err) || k8serrors.IsConflict(err)).To(BeTrue())
				})

				When("And a new workload is added after the initial reconciliation", func() {
					var newWorkloadNs *corev1.Namespace
					BeforeEach(func() {
						// Create a user workload namespace with random name.
						newWorkloadNs = &corev1.Namespace{
							ObjectMeta: metav1.ObjectMeta{
								Name: "user-workload-ns-" + uuid.New().String(),
								Labels: map[string]string{
									providerconfigmap.UserWorkloadNamespaceLabel: "cluster-moid",
								},
							},
						}
					})
					AfterEach(func() {
						Expect(ctx.Client.Delete(ctx, newWorkloadNs)).To(Succeed())
					})

					// Create a workload namespace, and create the provider ConfigMap with ContentSource key set. Validate that the ContentSourceBindings are created
					// in the user namespace. Then, create a new workload namespace and ensure that a reconcile is triggered due to the Namespace watch and
					// ContentSourceBindings have been created in the new namespace.
					It("re-triggers the reconcile and creates bindings in the new namespace", func() {
						// Wait for the initial reconcile
						Eventually(func() bool {
							return csExists(clUUID, clUUID) && clExists(clUUID)
						}).Should(BeTrue())

						// Validate that ContentSourceBindings exist in the existing user workload namespace.
						verifyContentSourceBinding(workloadNs.Name)

						// Create a new workload
						Expect(ctx.Client.Create(ctx, newWorkloadNs)).To(Succeed())

						// Validate that ContentSourceBindings exist in the newly created user workload namespace.
						verifyContentSourceBinding(newWorkloadNs.Name)
					})

					When("the initial reconciliation failed", func() {
						It("should ContentSourceBinding resource should be created in the newly added namespace.", func() {
							// Validate that ContentSourceBindings exist in the existing user workload namespace.
							verifyContentSourceBinding(workloadNs.Name)

							// Delete the contentSourceBinding to mock a failed workflow
							binding := &vmopv1.ContentSourceBinding{}
							Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: clUUID, Namespace: workloadNs.Name}, binding)).To(Succeed())
							Expect(ctx.Client.Delete(ctx, binding)).To(Succeed())

							// The namespace.Spec contains finalizers "kubernetes", which puts the namespace in Terminating
							// state when deleted, and ContentSourceBinding resource creation in this namespace would fail.
							Expect(ctx.Client.Delete(ctx, workloadNs)).Should(Succeed())

							// Ensure the namespace is in Terminating state
							Eventually(func() bool {
								obj := &corev1.Namespace{}
								if err := ctx.Client.Get(ctx, client.ObjectKey{Name: workloadNs.Name}, obj); err != nil {
									return false
								}
								return obj.Status.Phase == "Terminating"
							}).Should(BeTrue())

							// Create a new workload
							Expect(ctx.Client.Create(ctx, newWorkloadNs)).To(Succeed())

							// Validate that ContentSourceBindings exist in the newly created user workload namespace.
							verifyContentSourceBinding(newWorkloadNs.Name)
						})
					})
				})
			})
		})
	})
}
