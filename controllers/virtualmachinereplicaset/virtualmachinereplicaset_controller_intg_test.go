// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinereplicaset_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	finalizerName = "virtualmachinereplicaset.vmoperator.vmware.com"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.V1Alpha3,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {

	var (
		ctx *builder.IntegrationTestContext

		rs    *vmopv1.VirtualMachineReplicaSet
		rsKey types.NamespacedName
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		rs = &vmopv1.VirtualMachineReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-replicaset",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1.VirtualMachineReplicaSetSpec{
				Replicas: ptrTo(int32(2)),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"appname": "db",
					},
				},
				Template: vmopv1.VirtualMachineTemplateSpec{
					ObjectMeta: vmopv1common.ObjectMeta{
						Labels: map[string]string{
							"appname": "db",
						},
						Annotations: make(map[string]string),
					},
					Spec: vmopv1.VirtualMachineSpec{
						ImageName:  "dummy-image",
						ClassName:  "dummy-class",
						PowerState: vmopv1.VirtualMachinePowerStateOn,
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						},
					},
				},
			},
		}
		rsKey = types.NamespacedName{Name: rs.Name, Namespace: rs.Namespace}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	getVirtualMachineReplicaSet := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1.VirtualMachineReplicaSet {
		rs := &vmopv1.VirtualMachineReplicaSet{}
		if err := ctx.Client.Get(ctx, objKey, rs); err != nil {
			return nil
		}
		return rs
	}

	waitForReplicaSetFinalizer := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) {
		EventuallyWithOffset(1, func() []string {
			if rs := getVirtualMachineReplicaSet(ctx, objKey); rs != nil {
				return rs.GetFinalizers()
			}
			return nil
		}).Should(ContainElement(finalizerName), "waiting for VirtualMachineReplicaSet finalizer")
	}

	ensureReplicas := func(ctx *builder.IntegrationTestContext, labels map[string]string, desiredReplicas int) {
		Eventually(func(g Gomega) int {
			vmList := &vmopv1.VirtualMachineList{}
			err := ctx.Client.List(ctx, vmList, client.InNamespace(ctx.Namespace), client.MatchingLabels(labels))
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(vmList).ToNot(BeNil())
			return len(vmList.Items)
		}, 10*time.Second, 1*time.Second).Should(Equal(desiredReplicas))
	}

	Context("Reconcile", func() {
		dummyInstanceUUID := "instanceUUID1234"

		BeforeEach(func() {
			providerfake.SetCreateOrUpdateFunction(
				ctx,
				intgFakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					// Used below just to check for something in the Status is
					// updated.
					vm.Status.InstanceUUID = dummyInstanceUUID
					return nil
				})
		})

		AfterEach(func() {
			By("Delete VirtualMachineReplicaSet", func() {
				if err := ctx.Client.Delete(ctx, rs); err == nil {
					rs := &vmopv1.VirtualMachineReplicaSet{}
					// If ReplicaSet is still around because of finalizer, try to cleanup for next test.
					if err := ctx.Client.Get(ctx, rsKey, rs); err == nil && len(rs.Finalizers) > 0 {
						rs.Finalizers = nil
						_ = ctx.Client.Update(ctx, rs)
					}
				} else {
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}
			})

			// Integration tests use env-test which only starts API server.
			// Since garbage collection is handled by kube-controller-manager, we
			// can't really test that all replicas have been garbage collected
			// once the replica set has been deleted.
		})

		It("Reconciles after VirtualMachineReplicaSet creation", func() {
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())

			By("VirtualMachineReplicaSet should have finalizer added", func() {
				waitForReplicaSetFinalizer(ctx, rsKey)
			})

			By("Sufficient replicas must be created", func() {
				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))
			})
		})

		It("Scale up VirtualMachineReplicaSet", func() {
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())

			By("VirtualMachineReplicaSet should have finalizer added", func() {
				waitForReplicaSetFinalizer(ctx, rsKey)
			})

			By("Sufficient replicas must be created", func() {
				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))
			})

			By("Modifying the spec.replicas", func() {
				_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, rs, func() error {
					numReplicas := *rs.Spec.Replicas + 1
					rs.Spec.Replicas = &numReplicas
					return nil
				})
				Expect(err).ToNot(HaveOccurred())
				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))
			})
		})

		It("Scale down VirtualMachineReplicaSet", func() {
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())

			By("VirtualMachineReplicaSet should have finalizer added", func() {
				waitForReplicaSetFinalizer(ctx, rsKey)
			})

			By("Sufficient replicas must be created", func() {
				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))
			})

			By("Modifying the spec.replicas", func() {
				_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, rs, func() error {
					numReplicas := *rs.Spec.Replicas - 1
					rs.Spec.Replicas = &numReplicas
					return nil
				})
				Expect(err).ToNot(HaveOccurred())

				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))
			})
		})

		It("Reconciles after VirtualMachineReplicaSet deletion", func() {
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			// Wait for initial reconcile.
			waitForReplicaSetFinalizer(ctx, rsKey)

			Expect(ctx.Client.Delete(ctx, rs)).To(Succeed())
			By("Finalizer should be removed after deletion", func() {
				Eventually(func() []string {
					if vm := getVirtualMachineReplicaSet(ctx, rsKey); vm != nil {
						return vm.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizerName))
			})
		})
	})
}

func ptrTo[T any](t T) *T {
	return &t
}
