// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package operation_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
)

var _ = Describe("IsCreate", func() {
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
				_ = ctxop.IsCreate(ctx)
			}
			Expect(fn).To(PanicWith("context is nil"))
		})
	})
	When("value is missing from context", func() {
		It("should panic", func() {
			fn := func() {
				_ = ctxop.IsCreate(ctx)
			}
			Expect(fn).To(PanicWith("value is missing from context"))
		})
	})
	When("operation is present in context", func() {
		BeforeEach(func() {
			ctx = ctxop.WithContext(context.Background())
		})
		It("should return a operation", func() {
			Expect(ctxop.IsCreate(ctx)).To(BeFalse())
		})
	})
})

var _ = Describe("IsUpdate", func() {
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
				_ = ctxop.IsUpdate(ctx)
			}
			Expect(fn).To(PanicWith("context is nil"))
		})
	})
	When("value is missing from context", func() {
		It("should panic", func() {
			fn := func() {
				_ = ctxop.IsUpdate(ctx)
			}
			Expect(fn).To(PanicWith("value is missing from context"))
		})
	})
	When("operation is present in context", func() {
		BeforeEach(func() {
			ctx = ctxop.WithContext(context.Background())
		})
		It("should return a operation", func() {
			Expect(ctxop.IsUpdate(ctx)).To(BeFalse())
		})
	})
})

var _ = Describe("MarkCreate", func() {
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
				ctxop.MarkCreate(ctx)
			}
			Expect(fn).To(PanicWith("context is nil"))
		})
	})
	When("value is missing from context", func() {
		It("should panic", func() {
			fn := func() {
				ctxop.MarkCreate(ctx)
			}
			Expect(fn).To(PanicWith("value is missing from context"))
		})
	})
	When("operation is present in context", func() {
		var ok bool
		BeforeEach(func() {
			ctx = ctxop.WithContext(context.Background())
		})
		JustBeforeEach(func() {
			ctxop.MarkCreate(ctx)
			ok = ctxop.IsCreate(ctx)
		})
		When("operation is not yet marked", func() {
			It("should mark the operation", func() {
				Expect(ok).To(BeTrue())
			})
		})
		When("operation is already marked update", func() {
			BeforeEach(func() {
				ctxop.MarkUpdate(ctx)
			})
			It("should override the existing operation", func() {
				Expect(ok).To(BeTrue())
			})
		})
	})
})

var _ = Describe("MarkUpdate", func() {
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
				ctxop.MarkUpdate(ctx)
			}
			Expect(fn).To(PanicWith("context is nil"))
		})
	})
	When("value is missing from context", func() {
		It("should panic", func() {
			fn := func() {
				ctxop.MarkUpdate(ctx)
			}
			Expect(fn).To(PanicWith("value is missing from context"))
		})
	})
	When("operation is present in context", func() {
		var ok bool
		BeforeEach(func() {
			ctx = ctxop.WithContext(context.Background())
		})
		JustBeforeEach(func() {
			ctxop.MarkUpdate(ctx)
			ok = ctxop.IsUpdate(ctx)
		})
		When("operation is not yet marked", func() {
			It("should mark the operation", func() {
				Expect(ok).To(BeTrue())
			})
		})
		When("operation is already marked create", func() {
			BeforeEach(func() {
				ctxop.MarkCreate(ctx)
			})
			It("should not override the existing operation", func() {
				Expect(ok).To(BeFalse())
			})
		})
	})
})

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
				_ = ctxop.JoinContext(left, right)
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
				_ = ctxop.JoinContext(left, right)
			}
			Expect(fn).To(PanicWith("right context is nil"))
		})
	})
	When("value is missing from context", func() {
		It("should panic", func() {
			fn := func() {
				_ = ctxop.JoinContext(left, right)
			}
			Expect(fn).To(PanicWith("value is missing from context"))
		})
	})
	When("the left context has a operation", func() {
		BeforeEach(func() {
			left = ctxop.WithContext(context.Background())
			ctxop.MarkCreate(left)
			ctxop.MarkUpdate(left)
		})
		It("should return the left context", func() {
			ctx := ctxop.JoinContext(left, right)
			Expect(ctx).ToNot(BeNil())
			Expect(ctxop.IsCreate(ctx)).To(BeTrue())
		})
	})
	When("the right context has a operation", func() {
		BeforeEach(func() {
			right = ctxop.WithContext(context.Background())
			ctxop.MarkCreate(right)
		})
		It("should return a new context", func() {
			ctx := ctxop.JoinContext(left, right)
			Expect(ctx).ToNot(BeNil())
			Expect(ctxop.IsCreate(ctx)).To(BeTrue())
		})
	})
	When("both contexts have a operation", func() {
		BeforeEach(func() {
			left = ctxop.WithContext(context.Background())
			right = ctxop.WithContext(context.Background())
		})
		When("the left operation is create and the right operation is update", func() {
			BeforeEach(func() {
				ctxop.MarkCreate(left)
				ctxop.MarkUpdate(right)
			})
			It("should return the left context with operation from the left", func() {
				ctx := ctxop.JoinContext(left, right)
				Expect(ctx).ToNot(BeNil())
				Expect(ctxop.IsCreate(ctx)).To(BeTrue())
			})
		})
		When("the left operation is update and the right operation is create", func() {
			BeforeEach(func() {
				ctxop.MarkUpdate(left)
				ctxop.MarkCreate(right)
			})
			It("should return the left context with operation from the right", func() {
				ctx := ctxop.JoinContext(left, right)
				Expect(ctx).ToNot(BeNil())
				Expect(ctxop.IsCreate(ctx)).To(BeTrue())
			})
		})
	})
})

var _ = Describe("WithContext", func() {
	When("parent is nil", func() {
		It("should panic", func() {
			fn := func() {
				//nolint:staticcheck
				_ = ctxop.WithContext(nil)
			}
			Expect(fn).To(PanicWith("parent context is nil"))
		})
	})
	When("parent is not nil", func() {
		It("should return a context", func() {
			ctx := ctxop.WithContext(context.Background())
			Expect(ctx).ToNot(BeNil())
		})
	})
})
