// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmconfig_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

var _ = Describe("FromContext", func() {
	var (
		ctx context.Context
	)
	BeforeEach(func() {
		ctx = context.Background()
	})
	When("ctx is nil", func() {
		BeforeEach(func() {
			ctx = nil
		})
		It("should panic", func() {
			fn := func() {
				_ = vmconfig.FromContext(ctx)
			}
			Expect(fn).To(PanicWith("context is nil"))
		})
	})
	When("value is missing from context", func() {
		It("should panic", func() {
			fn := func() {
				_ = vmconfig.FromContext(ctx)
			}
			Expect(fn).To(PanicWith("value is missing from context"))
		})
	})
	When("map is present in context", func() {
		BeforeEach(func() {
			ctx = vmconfig.NewContext()
		})
		It("should return the value", func() {
			Expect(vmconfig.FromContext(ctx)).To(Equal([]vmconfig.Reconciler(nil)))
		})
	})
})

var _ = Describe("ValidateContext", func() {
	var (
		ctx context.Context
	)
	BeforeEach(func() {
		ctx = context.Background()
	})
	When("ctx is nil", func() {
		BeforeEach(func() {
			ctx = nil
		})
		It("should panic", func() {
			fn := func() {
				_ = vmconfig.ValidateContext(ctx)
			}
			Expect(fn).To(PanicWith("context is nil"))
		})
	})
	When("value is missing from context", func() {
		It("should return false", func() {
			Expect(vmconfig.ValidateContext(ctx)).To(BeFalse())
		})
	})
	When("value is present in context", func() {
		BeforeEach(func() {
			ctx = vmconfig.NewContext()
		})
		It("should return true", func() {
			Expect(vmconfig.ValidateContext(ctx)).To(BeTrue())
		})
	})
})

type reconciler struct {
	name string
}

func (r reconciler) Name() string {
	return r.name
}

func (r reconciler) Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	return nil
}

func (r reconciler) OnResult(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	resultErr error) error {

	return nil
}

var _ = Describe("JoinContext", func() {
	var (
		left  context.Context
		right context.Context
	)
	BeforeEach(func() {
		left = context.Background()
		right = context.Background()
	})
	When("left context is nil", func() {
		BeforeEach(func() {
			left = nil
		})
		It("should panic", func() {
			fn := func() {
				_ = vmconfig.JoinContext(left, right)
			}
			Expect(fn).To(PanicWith("left context is nil"))
		})
	})
	When("right context is nil", func() {
		BeforeEach(func() {
			right = nil
		})
		It("should panic", func() {
			fn := func() {
				_ = vmconfig.JoinContext(left, right)
			}
			Expect(fn).To(PanicWith("right context is nil"))
		})
	})
	When("value is missing from context", func() {
		It("should panic", func() {
			fn := func() {
				_ = vmconfig.JoinContext(left, right)
			}
			Expect(fn).To(PanicWith("value is missing from context"))
		})
	})
	When("the left context has the map", func() {
		BeforeEach(func() {
			left = vmconfig.NewContext()
		})
		It("should return the left context", func() {
			ctx := vmconfig.JoinContext(left, right)
			Expect(ctx).ToNot(BeNil())
			Expect(vmconfig.ValidateContext(ctx)).To(BeTrue())
			Expect(ctx).To(Equal(left))
		})
	})
	When("the right context has the map", func() {
		BeforeEach(func() {
			right = vmconfig.NewContext()
		})
		It("should return a new context", func() {
			ctx := vmconfig.JoinContext(left, right)
			Expect(ctx).ToNot(BeNil())
			Expect(vmconfig.ValidateContext(ctx)).To(BeTrue())
		})
	})
	When("both contexts have the map", func() {
		var (
			v0  vmconfig.Reconciler
			v1  vmconfig.Reconciler
			v2a vmconfig.Reconciler
			v2b vmconfig.Reconciler
		)
		BeforeEach(func() {
			v0 = &reconciler{name: "r0"}
			v1 = &reconciler{name: "r1"}
			v2a = &reconciler{name: "r2"}
			v2b = &reconciler{name: "r2"}

			left = vmconfig.NewContext()
			right = vmconfig.NewContext()

			vmconfig.Register(left, v0)
			vmconfig.Register(right, v1)
			vmconfig.Register(left, v2a)
			vmconfig.Register(right, v2b)

			Expect(vmconfig.FromContext(left)).To(ContainElements(v0, v2a))
			Expect(vmconfig.FromContext(right)).To(ContainElements(v1, v2b))
		})
		It("should return the left context with key/value pairs from the right", func() {
			ctx := vmconfig.JoinContext(left, right)
			Expect(ctx).ToNot(BeNil())
			Expect(vmconfig.ValidateContext(ctx)).To(BeTrue())
			Expect(ctx).To(Equal(left))
			Expect(vmconfig.FromContext(ctx)).To(ContainElements(v0, v1, v2b))
		})
	})
})

var _ = Describe("NewContext", func() {
	It("should return a valid context", func() {
		Expect(vmconfig.ValidateContext(vmconfig.NewContext())).To(BeTrue())
	})
})

var _ = Describe("WithContext", func() {
	When("parent is nil", func() {
		It("should panic", func() {
			fn := func() {
				//nolint:staticcheck
				_ = vmconfig.WithContext(nil)
			}
			Expect(fn).To(PanicWith("parent context is nil"))
		})
	})
	When("parent is not nil", func() {
		It("should return a context", func() {
			ctx := vmconfig.WithContext(context.Background())
			Expect(ctx).ToNot(BeNil())
		})
	})
})
