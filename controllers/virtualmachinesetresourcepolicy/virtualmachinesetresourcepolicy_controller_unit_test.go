// +build !integration

// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

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
	const (
		nsName             = "sample-ns"
		controllerName     = "virtualmachinesetresourcepolicy-controller"
		resourcePolicyName = "sample-resourcepolicy"
	)
	var (
		initObjects       []runtime.Object
		ctx               *builder.UnitTestContextForController
		reconciler        *virtualmachinesetresourcepolicy.VirtualMachineSetResourcePolicyReconciler
		resourcePolicyCtx *context.VirtualMachineSetResourcePolicyContext
		resourcePolicy    *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy
	)
	BeforeEach(func() {
		resourcePolicy = &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourcePolicyName,
				Namespace: nsName,
			},
			Spec: vmoperatorv1alpha1.VirtualMachineSetResourcePolicySpec{},
		}
	})
	JustBeforeEach(func() {
		initObjects = []runtime.Object{resourcePolicy}
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = &virtualmachinesetresourcepolicy.VirtualMachineSetResourcePolicyReconciler{
			Client:     ctx.Client,
			Logger:     ctx.Logger.WithName("controllers").WithName(controllerName),
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
		It("will have finalizer set after reconciliation", func() {
			err := reconciler.ReconcileNormal(resourcePolicyCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(resourcePolicy.ObjectMeta.Finalizers)).ToNot(BeZero())
			Expect(resourcePolicy.GetFinalizers()).To(ContainElement(finalizer))
		})
	})
	Context("ReconcileDelete", func() {
		It("will delete the created VM and emit corresponding event", func() {
			// Create the resource policy to be deleted
			err := reconciler.ReconcileNormal(resourcePolicyCtx)
			Expect(err).NotTo(HaveOccurred())

			// Delete the created resource policy
			err = reconciler.ReconcileDelete(resourcePolicyCtx)
			Expect(err).NotTo(HaveOccurred())
			fakeProvider := ctx.VmProvider.(*providerfake.FakeVmProvider)
			rpExists, err := fakeProvider.DoesVirtualMachineSetResourcePolicyExist(resourcePolicyCtx, resourcePolicy)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpExists).ToNot(BeTrue())
		})
	})
}
