// Copyright (c) 2020-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinesetresourcepolicy"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	finalizer = "vmoperator.vmware.com/virtualmachinesetresourcepolicy"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.V1Alpha3,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController
		reconciler  *virtualmachinesetresourcepolicy.Reconciler

		resourcePolicyCtx *pkgctx.VirtualMachineSetResourcePolicyContext
		resourcePolicy    *vmopv1.VirtualMachineSetResourcePolicy
		vm, vmNoReserved  *vmopv1.VirtualMachine
	)

	BeforeEach(func() {
		resourcePolicy = &vmopv1.VirtualMachineSetResourcePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-rp",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1.VirtualMachineSetResourcePolicySpec{},
		}
		vmNoReserved = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm-no-reserved",
				Namespace: "dummy-ns",
			},
		}
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1.VirtualMachineSpec{
				Reserved: &vmopv1.VirtualMachineReservedSpec{
					ResourcePolicyName: "dummy-rp",
				},
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)

		reconciler = &virtualmachinesetresourcepolicy.Reconciler{
			Client:     ctx.Client,
			Logger:     ctx.Logger,
			VMProvider: ctx.VMProvider,
		}

		resourcePolicyCtx = &pkgctx.VirtualMachineSetResourcePolicyContext{
			Context:        ctx.Context,
			Logger:         ctx.Logger.WithName(resourcePolicy.Namespace).WithName(resourcePolicy.Name),
			ResourcePolicy: resourcePolicy,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		resourcePolicyCtx = nil
		reconciler = nil
	})

	Context("ReconcileNormal", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, resourcePolicy)
		})

		It("will have finalizer set after reconciliation", func() {
			err := reconciler.ReconcileNormal(resourcePolicyCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(resourcePolicy.GetFinalizers()).To(ContainElement(finalizer))
		})
	})

	Context("ReconcileDelete", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, resourcePolicy)
		})

		When("One or more VMs are referencing this policy", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, vm, vmNoReserved)
			})

			AfterEach(func() {
				err := ctx.Client.Delete(ctx, vm)
				Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
			})

			It("will fail to delete the ResourcePolicy", func() {
				err := reconciler.ReconcileNormal(resourcePolicyCtx)
				Expect(err).NotTo(HaveOccurred())

				err = reconciler.ReconcileDelete(resourcePolicyCtx)
				expectedError := fmt.Errorf("failing VirtualMachineSetResourcePolicy deletion since VM: '%s' is referencing it, resourcePolicyName: '%s'", vm.NamespacedName(), resourcePolicy.NamespacedName())
				Expect(err).To(MatchError(expectedError))

				By("will still have finalizer", func() {
					Expect(resourcePolicyCtx.ResourcePolicy.GetFinalizers()).To(ContainElement(finalizer))
				})
			})
		})

		It("will delete the created ResourcePolicy", func() {
			err := reconciler.ReconcileNormal(resourcePolicyCtx)
			Expect(err).NotTo(HaveOccurred())

			err = reconciler.ReconcileDelete(resourcePolicyCtx)
			Expect(err).NotTo(HaveOccurred())

			By("will not have finalizer", func() {
				Expect(resourcePolicyCtx.ResourcePolicy.GetFinalizers()).To(BeEmpty())
			})
		})
	})
}
