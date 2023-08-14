// Copyright (c) 2020-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	virtualmachineclass "github.com/vmware-tanzu/vm-operator/controllers/virtualmachineclass/v1alpha2"
	vmopContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking Reconcile", unitTestsReconcile)
}

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler *virtualmachineclass.Reconciler
		vmClassCtx *vmopContext.VirtualMachineClassContextA2
		vmClass    *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		vmClass = &vmopv1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-vmclass",
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = virtualmachineclass.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
		)

		vmClassCtx = &vmopContext.VirtualMachineClassContextA2{
			Context: ctx,
			Logger:  ctx.Logger.WithName(vmClass.Name),
			VMClass: vmClass,
		}
	})

	Context("ReconcileNormal", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, vmClass)
		})

		It("returns success", func() {
			err := reconciler.ReconcileNormal(vmClassCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmClassCtx.VMClass.Status.Ready).To(BeTrue())
		})
	})
}
