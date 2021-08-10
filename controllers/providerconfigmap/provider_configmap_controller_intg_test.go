// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providerconfigmap_test

import (
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/providerconfigmap"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking ConfigMap controller integration tests", intgTestsCM)
}

func intgTestsCM() {
	var (
		ctx *builder.IntegrationTestContext

		cm     *corev1.ConfigMap
		clUUID = "dummy-cl"

		// represents the VM Service FSS. This should be manipulated atomically to avoid races where
		// the controller is trying to read this _while_ the tests are updating it.
		vmServiceFSS uint32
	)

	BeforeEach(func() {
		// Modify the helper function to return the custom value of the FSS
		lib.IsVMServiceFSSEnabled = func() bool {
			return atomic.LoadUint32(&vmServiceFSS) != 0
		}

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
			clList := &vmopv1alpha1.ContentLibraryProviderList{}
			Expect(ctx.Client.List(ctx, clList)).To(Succeed())

			for _, cl := range clList.Items {
				if cl.Name == clName {
					return true
				}
			}

			return false
		}

		// Verifies that a ContentSource exists, has a matching label and has a ProviderRef set to the content library..
		csExists := func(csName, clName string) bool {
			csList := &vmopv1alpha1.ContentSourceList{}
			Expect(ctx.Client.List(ctx, csList)).To(Succeed())

			return len(csList.Items) == 1 && csList.Items[0].Name == csName && csList.Items[0].Spec.ProviderRef.Name == clName &&
				csList.Items[0].Labels[providerconfigmap.TKGContentSourceLabelKey] == providerconfigmap.TKGContentSourceLabelValue
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
				csList := &vmopv1alpha1.ContentSourceList{}
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

				Eventually(func() bool {
					return clExists(clUUID)
				}).Should(BeTrue())

				// Validate that no ContentSources exist in the system namespace.
				bindingList := &vmopv1alpha1.ContentSourceBindingList{}
				Expect(ctx.Client.List(ctx, bindingList, client.InNamespace(ctx.Namespace))).To(Succeed())
				Expect(bindingList.Items).To(BeEmpty())
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
			var oldVMServiceFSSState uint32

			BeforeEach(func() {
				oldVMServiceFSSState = vmServiceFSS
				atomic.StoreUint32(&vmServiceFSS, 1)
			})

			AfterEach(func() {
				atomic.StoreUint32(&vmServiceFSS, oldVMServiceFSSState)
			})

			When("ConfigMap is created with a ContentSource key", func() {
				var workloadNs *corev1.Namespace
				BeforeEach(func() {
					cm.Data = make(map[string]string)
					cm.Data[config.ContentSourceKey] = clUUID

					// Create a user workload namespace.
					workloadNs = &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "user-workload-ns-1",
							Labels: map[string]string{
								providerconfigmap.UserWorkloadNamespaceLabel: "cluster-moid",
							},
						},
					}
					Expect(ctx.Client.Create(ctx, workloadNs)).To(Succeed())
				})

				AfterEach(func() {
					Expect(ctx.Client.Delete(ctx, workloadNs)).To(Succeed())
				})

				When("And a new workload is added after the initial reconciliation", func() {
					var newWorkloadNs *corev1.Namespace
					BeforeEach(func() {
						// Create a user workload namespace.
						newWorkloadNs = &corev1.Namespace{
							ObjectMeta: metav1.ObjectMeta{
								Name: "user-workload-ns-2",
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

						Eventually(func() bool {
							return clExists(clUUID)
						}).Should(BeTrue())

						// Validate that ContentSourceBindings exist in the existing user workload namespace.
						Eventually(func() bool {
							bindingList := &vmopv1alpha1.ContentSourceBindingList{}
							err := ctx.Client.List(ctx, bindingList, client.InNamespace(workloadNs.Name))
							return err == nil && len(bindingList.Items) == 1 && bindingList.Items[0].ContentSourceRef.Kind == "ContentSource" &&
								bindingList.Items[0].ContentSourceRef.Name == clUUID
						}).Should(BeTrue())

						// Create a new workload
						Expect(ctx.Client.Create(ctx, newWorkloadNs)).To(Succeed())

						// Validate that ContentSourceBindings exist in the newly created user workload namespace.
						Eventually(func() bool {
							bindingList := &vmopv1alpha1.ContentSourceBindingList{}
							err := ctx.Client.List(ctx, bindingList, client.InNamespace(newWorkloadNs.Name))
							return err == nil && len(bindingList.Items) == 1 && bindingList.Items[0].ContentSourceRef.Kind == "ContentSource" &&
								bindingList.Items[0].ContentSourceRef.Name == clUUID
						}).Should(BeTrue())
					})
				})
			})
		})
	})
}
