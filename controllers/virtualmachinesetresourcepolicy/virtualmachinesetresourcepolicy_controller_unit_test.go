// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinesetresourcepolicy"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking Reconcile", unitTestsReconcile)
}

const (
	finalizer = "virtualmachinesetresourcepolicy.vmoperator.vmware.com"
)

func unitTestsReconcile() {
	var (
		initObjects []runtime.Object
		ctx         *builder.UnitTestContextForController
		reconciler  *virtualmachinesetresourcepolicy.VirtualMachineSetResourcePolicyReconciler

		resourcePolicyCtx *context.VirtualMachineSetResourcePolicyContext
		resourcePolicy    *vmopv1alpha1.VirtualMachineSetResourcePolicy
		vm                *vmopv1alpha1.VirtualMachine
	)
	BeforeEach(func() {
		resourcePolicy = &vmopv1alpha1.VirtualMachineSetResourcePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-rp",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1alpha1.VirtualMachineSetResourcePolicySpec{},
		}
		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ResourcePolicyName: "dummy-rp",
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = &virtualmachinesetresourcepolicy.VirtualMachineSetResourcePolicyReconciler{
			Client:     ctx.Client,
			Logger:     ctx.Logger,
			VMProvider: ctx.VmProvider,
		}

		resourcePolicyCtx = &context.VirtualMachineSetResourcePolicyContext{
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
				initObjects = append(initObjects, vm)
			})

			AfterEach(func() {
				err := ctx.Client.Delete(ctx, vm)
				Expect(err == nil || apiErrors.IsNotFound(err)).To(BeTrue())
			})

			It("will fail to delete the ResourcePolicy", func() {
				err := reconciler.ReconcileNormal(resourcePolicyCtx)
				Expect(err).NotTo(HaveOccurred())

				err = reconciler.ReconcileDelete(resourcePolicyCtx)
				expectedError := fmt.Errorf("failing VirtualMachineSetResourcePolicy deletion since VM: '%s' is referencing it, resourcePolicyName: '%s'", vm.NamespacedName(), resourcePolicy.NamespacedName())
				Expect(err).To(MatchError(expectedError))

				By("provider should return that the policy still exists", func() {
					fakeProvider := ctx.VmProvider.(*providerfake.FakeVmProvider)
					rpExists, err := fakeProvider.DoesVirtualMachineSetResourcePolicyExist(resourcePolicyCtx, resourcePolicy)
					Expect(err).NotTo(HaveOccurred())
					Expect(rpExists).To(BeFalse())
				})
			})
		})

		It("will delete the created ResourcePolicy", func() {
			err := reconciler.ReconcileNormal(resourcePolicyCtx)
			Expect(err).NotTo(HaveOccurred())

			err = reconciler.ReconcileDelete(resourcePolicyCtx)
			Expect(err).NotTo(HaveOccurred())

			By("provider should return that the policy does not exist", func() {
				fakeProvider := ctx.VmProvider.(*providerfake.FakeVmProvider)
				rpExists, err := fakeProvider.DoesVirtualMachineSetResourcePolicyExist(resourcePolicyCtx, resourcePolicy)
				Expect(err).NotTo(HaveOccurred())
				Expect(rpExists).To(BeFalse())
			})
		})
	})
}
