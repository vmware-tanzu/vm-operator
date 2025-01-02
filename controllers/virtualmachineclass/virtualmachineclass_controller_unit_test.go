// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineclass_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineclass"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
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

		reconciler *virtualmachineclass.Reconciler
		vmClass    *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		initObjects = nil
		vmClass = &vmopv1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-vmclass",
			},
		}
	})

	JustBeforeEach(func() {
		initObjects = append(initObjects, vmClass)
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = virtualmachineclass.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
		)
	})

	Context("Reconcile", func() {
		var (
			err  error
			name string
		)

		BeforeEach(func() {
			err = nil
			name = vmClass.Name
		})

		JustBeforeEach(func() {
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: vmClass.Namespace,
					Name:      name,
				}})
		})

		When("Deleted", func() {
			BeforeEach(func() {
				vmClass.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				vmClass.Finalizers = append(vmClass.Finalizers, "fake.com/finalizer")
			})
			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("Normal", func() {
			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("Class not found", func() {
			BeforeEach(func() {
				name = "invalid"
			})
			It("ignores the error", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

}
