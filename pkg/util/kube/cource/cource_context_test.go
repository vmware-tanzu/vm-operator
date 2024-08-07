// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cource_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
)

var _ = Describe("FromContext", func() {
	var (
		ctx context.Context
		key any
	)
	BeforeEach(func() {
		ctx = context.Background()
		key = "1"
	})
	When("ctx is nil", func() {
		BeforeEach(func() {
			ctx = nil
		})
		It("should panic", func() {
			fn := func() {
				_ = cource.FromContext(ctx, key)
			}
			Expect(fn).To(PanicWith("context is nil"))
		})
	})
	When("map is missing from context", func() {
		It("should panic", func() {
			fn := func() {
				_ = cource.FromContext(ctx, key)
			}
			Expect(fn).To(PanicWith("map is missing from context"))
		})
	})
	When("map is present in context", func() {
		BeforeEach(func() {
			ctx = cource.NewContext()
		})
		It("should return a channel", func() {
			Expect(cource.FromContext(ctx, key)).ToNot(BeNil())
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
				_ = cource.ValidateContext(ctx)
			}
			Expect(fn).To(PanicWith("context is nil"))
		})
	})
	When("map is missing from context", func() {
		It("should return false", func() {
			Expect(cource.ValidateContext(ctx)).To(BeFalse())
		})
	})
	When("map is present in context", func() {
		BeforeEach(func() {
			ctx = cource.NewContext()
		})
		It("should return true", func() {
			Expect(cource.ValidateContext(ctx)).To(BeTrue())
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
				_ = cource.JoinContext(left, right)
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
				_ = cource.JoinContext(left, right)
			}
			Expect(fn).To(PanicWith("right context is nil"))
		})
	})
	When("map is missing from context", func() {
		It("should panic", func() {
			fn := func() {
				_ = cource.JoinContext(left, right)
			}
			Expect(fn).To(PanicWith("map is missing from context"))
		})
	})
	When("the left context has the map", func() {
		BeforeEach(func() {
			left = cource.NewContext()
		})
		It("should return the left context", func() {
			ctx := cource.JoinContext(left, right)
			Expect(ctx).ToNot(BeNil())
			Expect(cource.ValidateContext(ctx)).To(BeTrue())
			Expect(ctx).To(Equal(left))
		})
	})
	When("the right context has the map", func() {
		BeforeEach(func() {
			right = cource.NewContext()
		})
		It("should return a new context", func() {
			ctx := cource.JoinContext(left, right)
			Expect(ctx).ToNot(BeNil())
			Expect(cource.ValidateContext(ctx)).To(BeTrue())
		})
	})
	When("both contexts have the map", func() {
		var (
			c0 chan event.GenericEvent
			c1 chan event.GenericEvent
			c2 chan event.GenericEvent
		)
		BeforeEach(func() {
			left = cource.NewContext()
			right = cource.NewContext()

			c0 = cource.FromContext(left, 0)
			_ = cource.FromContext(left, 1)
			c1 = cource.FromContext(right, 1)
			c2 = cource.FromContext(right, 2)
		})
		It("should return the left context with key/value pairs from the right", func() {
			ctx := cource.JoinContext(left, right)
			Expect(ctx).ToNot(BeNil())
			Expect(cource.ValidateContext(ctx)).To(BeTrue())
			Expect(ctx).To(Equal(left))
			Expect(cource.FromContext(left, 0)).To(Equal(c0))
			Expect(cource.FromContext(left, 1)).To(Equal(c1))
			Expect(cource.FromContext(left, 2)).To(Equal(c2))
		})
	})
})

var _ = Describe("WithContext", func() {
	When("parent is nil", func() {
		It("should panic", func() {
			fn := func() {
				//nolint:staticcheck
				_ = cource.WithContext(nil)
			}
			Expect(fn).To(PanicWith("parent context is nil"))
		})
	})
	When("parent is not nil", func() {
		It("should return a context", func() {
			ctx := cource.WithContext(context.Background())
			Expect(ctx).ToNot(BeNil())
		})
	})
})
