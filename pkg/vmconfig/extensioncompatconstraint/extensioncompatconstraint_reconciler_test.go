// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package extensioncompatconstraint_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	vmconfextensioncompatconstraint "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/extensioncompatconstraint"
)

func makeVM() *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "vm"},
	}
}

var _ = Describe("New", func() {
	It("returns a non-nil Reconciler", func() {
		Expect(vmconfextensioncompatconstraint.New()).ToNot(BeNil())
	})
	It("has name 'extensioncompatconstraint'", func() {
		Expect(vmconfextensioncompatconstraint.New().Name()).To(Equal("extensioncompatconstraint"))
	})
})

var _ = Describe("OnResult", func() {
	It("is a no-op", func() {
		r := vmconfextensioncompatconstraint.New()
		Expect(r.OnResult(context.Background(), makeVM(), mo.VirtualMachine{}, nil)).To(Succeed())
	})
})

var _ = Describe("Reconcile", func() {

	var (
		ctx        context.Context
		vm         *vmopv1.VirtualMachine
		moVM       mo.VirtualMachine
		configSpec *vimtypes.VirtualMachineConfigSpec
		r          vmconfig.Reconciler
	)

	BeforeEach(func() {
		r = vmconfextensioncompatconstraint.New()
		ctx = pkgcfg.NewContextWithDefaultConfig()
		vm = makeVM()
		moVM = mo.VirtualMachine{}
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
	})

	Context("a panic is expected", func() {
		When("ctx is nil", func() {
			JustBeforeEach(func() {
				ctx = nil
			})
			It("panics", func() {
				fn := func() {
					_ = r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})

		When("vm is nil", func() {
			JustBeforeEach(func() {
				vm = nil
			})
			It("panics", func() {
				fn := func() {
					_ = r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})

		When("configSpec is nil", func() {
			JustBeforeEach(func() {
				configSpec = nil
			})
			It("panics", func() {
				fn := func() {
					_ = r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("configSpec is nil"))
			})
		})
	})

	When("moVM.Config is nil", func() {
		BeforeEach(func() {
			moVM = mo.VirtualMachine{}
		})

		It("does not modify the ConfigSpec", func() {
			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
			Expect(configSpec.ExtensionCompatibilityConstraint).To(BeNil())
		})
	})

	When("moVM.Config's constraint set does not match the desired set", func() {
		BeforeEach(func() {
			moVM = mo.VirtualMachine{
				Config: &vimtypes.VirtualMachineConfigInfo{},
			}
		})

		It("sets the full desired set on the ConfigSpec", func() {
			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
			Expect(configSpec.ExtensionCompatibilityConstraint).ToNot(BeNil())
			Expect(configSpec.ExtensionCompatibilityConstraint.Constraint).To(HaveLen(6))
		})
	})
})
