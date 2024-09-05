// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package generic_test

import (
	"context"
	"maps"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
)

type simpleContextKeyType uint8

const simpleContextKeyValue simpleContextKeyType = 0

type simpleContextValueType uint8

const (
	simpleContextValue0 simpleContextValueType = iota
	simpleContextValue1
	simpleContextValue2
)

//nolint:unparam
func simpleSetContext(parent context.Context, newVal simpleContextValueType) {
	ctxgen.SetContext(
		parent,
		simpleContextKeyValue,
		func(curVal simpleContextValueType) simpleContextValueType {
			return newVal
		})
}

func simpleFromContext(ctx context.Context) simpleContextValueType {
	return ctxgen.FromContext(
		ctx,
		simpleContextKeyValue,
		func(val simpleContextValueType) simpleContextValueType {
			return val
		})
}

func simpleWithContext(parent context.Context) context.Context {
	return ctxgen.WithContext(
		parent,
		simpleContextKeyValue,
		func() simpleContextValueType { return simpleContextValue0 })
}

func simpleNewContext() context.Context {
	return ctxgen.NewContext(
		simpleContextKeyValue,
		func() simpleContextValueType { return simpleContextValue0 })
}

func simpleValidateContext(ctx context.Context) bool {
	return ctxgen.ValidateContext[simpleContextValueType](ctx, simpleContextKeyValue)
}

func simpleJoinContext(left, right context.Context) context.Context {
	return ctxgen.JoinContext(
		left,
		right,
		simpleContextKeyValue,
		func(dst, src simpleContextValueType) simpleContextValueType {
			return src
		})
}

var _ = Describe("SimpleType", func() {

	Describe("SetContext", func() {
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
					simpleSetContext(ctx, simpleContextValue1)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})
		When("value is missing from context", func() {
			It("should panic", func() {
				fn := func() {
					simpleSetContext(ctx, simpleContextValue1)
				}
				Expect(fn).To(PanicWith("value is missing from context"))
			})
		})
		When("value is present in context", func() {
			BeforeEach(func() {
				ctx = simpleNewContext()
				simpleSetContext(ctx, simpleContextValue1)
			})
			It("should set value", func() {
				simpleSetContext(ctx, simpleContextValue1)
				Expect(simpleFromContext(ctx)).To(Equal(simpleContextValue1))
			})
		})
	})

	Describe("FromContext", func() {
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
					_ = simpleFromContext(ctx)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})
		When("value is missing from context", func() {
			It("should panic", func() {
				fn := func() {
					_ = simpleFromContext(ctx)
				}
				Expect(fn).To(PanicWith("value is missing from context"))
			})
		})
		When("value is present in context", func() {
			BeforeEach(func() {
				ctx = simpleNewContext()
			})
			It("should return the value", func() {
				Expect(simpleFromContext(ctx)).To(Equal(simpleContextValue0))
			})
		})
	})

	Describe("ValidateContext", func() {
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
					_ = simpleValidateContext(ctx)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})
		When("value is missing from context", func() {
			It("should return false", func() {
				Expect(simpleValidateContext(ctx)).To(BeFalse())
			})
		})
		When("map is present in context", func() {
			BeforeEach(func() {
				ctx = simpleNewContext()
			})
			It("should return true", func() {
				Expect(simpleValidateContext(ctx)).To(BeTrue())
			})
		})
	})

	Describe("JoinContext", func() {
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
					_ = simpleJoinContext(left, right)
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
					_ = simpleJoinContext(left, right)
				}
				Expect(fn).To(PanicWith("right context is nil"))
			})
		})
		When("value is missing from context", func() {
			It("should panic", func() {
				fn := func() {
					_ = simpleJoinContext(left, right)
				}
				Expect(fn).To(PanicWith("value is missing from context"))
			})
		})
		When("the left context has the value", func() {
			BeforeEach(func() {
				left = simpleNewContext()
			})
			It("should return the left context", func() {
				ctx := simpleJoinContext(left, right)
				Expect(ctx).ToNot(BeNil())
				Expect(simpleValidateContext(ctx)).To(BeTrue())
				Expect(ctx).To(Equal(left))
			})
		})
		When("the right context has the value", func() {
			BeforeEach(func() {
				right = simpleNewContext()
			})
			It("should return a new context", func() {
				ctx := simpleJoinContext(left, right)
				Expect(ctx).ToNot(BeNil())
				Expect(simpleValidateContext(ctx)).To(BeTrue())
			})
		})
		When("both contexts have the value", func() {
			var (
				v0 simpleContextValueType
			)
			BeforeEach(func() {
				left = simpleNewContext()
				right = simpleNewContext()

				_ = simpleFromContext(left)
				v0 = simpleFromContext(right)
			})
			It("should return the left context with key/value pairs from the right", func() {
				ctx := simpleJoinContext(left, right)
				Expect(ctx).ToNot(BeNil())
				Expect(simpleValidateContext(ctx)).To(BeTrue())
				Expect(ctx).To(Equal(left))
				Expect(simpleFromContext(left)).To(Equal(v0))
			})
		})
	})

	Describe("NewContext", func() {
		It("should return a valid context", func() {
			Expect(simpleValidateContext(simpleNewContext())).To(BeTrue())
		})
	})

	Describe("WithContext", func() {
		When("parent is nil", func() {
			It("should panic", func() {
				fn := func() {
					//nolint:staticcheck
					_ = simpleWithContext(nil)
				}
				Expect(fn).To(PanicWith("parent context is nil"))
			})
		})
		When("parent is not nil", func() {
			It("should return a context", func() {
				ctx := simpleWithContext(context.Background())
				Expect(ctx).ToNot(BeNil())
			})
		})
	})

})

type complexContextKeyType uint8

const complexContextKeyValue complexContextKeyType = 0

type complexContextValueType map[uint8]uint8

func complexSetContext(parent context.Context, key, val uint8) {
	ctxgen.SetContext(
		parent,
		complexContextKeyValue,
		func(curVal complexContextValueType) complexContextValueType {
			curVal[key] = val
			return curVal
		})
}

func complexFromContext(ctx context.Context, key uint8) uint8 {
	return ctxgen.FromContext(
		ctx,
		complexContextKeyValue,
		func(val complexContextValueType) uint8 {
			return val[key]
		})
}

func complexWithContext(parent context.Context) context.Context {
	return ctxgen.WithContext(
		parent,
		complexContextKeyValue,
		func() complexContextValueType { return complexContextValueType{} })
}

func complexNewContext() context.Context {
	return ctxgen.NewContext(
		complexContextKeyValue,
		func() complexContextValueType { return complexContextValueType{} })
}

func complexValidateContext(ctx context.Context) bool {
	return ctxgen.ValidateContext[complexContextValueType](ctx, complexContextKeyValue)
}

func complexJoinContext(left, right context.Context) context.Context {
	return ctxgen.JoinContext(
		left,
		right,
		complexContextKeyValue,
		func(dst, src complexContextValueType) complexContextValueType {
			maps.Copy(dst, src)
			return dst
		})
}

var _ = Describe("ComplexType", func() {
	Describe("SetContext", func() {
		var (
			ctx context.Context
			key uint8
			val uint8
		)
		BeforeEach(func() {
			ctx = context.Background()
			key = 0
			val = 1
		})

		When("ctx is nil", func() {
			BeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					complexSetContext(ctx, key, val)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})
		When("value is missing from context", func() {
			It("should panic", func() {
				fn := func() {
					complexSetContext(ctx, key, val)
				}
				Expect(fn).To(PanicWith("value is missing from context"))
			})
		})
		When("value is present in context", func() {
			BeforeEach(func() {
				ctx = complexNewContext()
				complexSetContext(ctx, key, val)
			})
			It("should set value", func() {
				complexSetContext(ctx, key, val)
				Expect(complexFromContext(ctx, key)).To(Equal(val))
			})
		})
	})

	Describe("FromContext", func() {
		var (
			ctx context.Context
			key uint8
			val uint8
		)
		BeforeEach(func() {
			ctx = context.Background()
			key = 0
			val = 0
		})

		When("ctx is nil", func() {
			BeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = complexFromContext(ctx, key)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})
		When("value is missing from context", func() {
			It("should panic", func() {
				fn := func() {
					_ = complexFromContext(ctx, key)
				}
				Expect(fn).To(PanicWith("value is missing from context"))
			})
		})
		When("value is present in context", func() {
			BeforeEach(func() {
				ctx = complexNewContext()
			})
			It("should return the value", func() {
				Expect(complexFromContext(ctx, key)).To(Equal(val))
			})
		})
	})

	Describe("ValidateContext", func() {
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
					_ = complexValidateContext(ctx)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})
		When("value is missing from context", func() {
			It("should return false", func() {
				Expect(complexValidateContext(ctx)).To(BeFalse())
			})
		})
		When("map is present in context", func() {
			BeforeEach(func() {
				ctx = complexNewContext()
			})
			It("should return true", func() {
				Expect(complexValidateContext(ctx)).To(BeTrue())
			})
		})
	})

	Describe("JoinContext", func() {
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
					_ = complexJoinContext(left, right)
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
					_ = complexJoinContext(left, right)
				}
				Expect(fn).To(PanicWith("right context is nil"))
			})
		})
		When("value is missing from context", func() {
			It("should panic", func() {
				fn := func() {
					_ = complexJoinContext(left, right)
				}
				Expect(fn).To(PanicWith("value is missing from context"))
			})
		})
		When("the left context has the value", func() {
			BeforeEach(func() {
				left = complexNewContext()
			})
			It("should return the left context", func() {
				ctx := complexJoinContext(left, right)
				Expect(ctx).ToNot(BeNil())
				Expect(complexValidateContext(ctx)).To(BeTrue())
				Expect(ctx).To(Equal(left))
			})
		})
		When("the right context has the value", func() {
			BeforeEach(func() {
				right = complexNewContext()
			})
			It("should return a new context", func() {
				ctx := complexJoinContext(left, right)
				Expect(ctx).ToNot(BeNil())
				Expect(complexValidateContext(ctx)).To(BeTrue())
			})
		})
		When("both contexts have the value", func() {
			var (
				k0 uint8
				k1 uint8
				k2 uint8
				v0 uint8
				v1 uint8
				v2 uint8
				v3 uint8
				v4 uint8
			)
			BeforeEach(func() {
				k0 = 0
				k1 = 1
				k2 = 2
				v0 = 0
				v1 = 1
				v2 = 2
				v3 = 3
				v4 = 4

				left = complexNewContext()
				right = complexNewContext()

				complexSetContext(left, k0, v0)
				complexSetContext(left, k1, v1)
				complexSetContext(left, k2, v2)
				complexSetContext(right, k0, v3)
				complexSetContext(right, k1, v4)
			})
			It("should return the left context with key/value pairs from the right", func() {
				ctx := complexJoinContext(left, right)
				Expect(ctx).ToNot(BeNil())
				Expect(complexValidateContext(ctx)).To(BeTrue())
				Expect(ctx).To(Equal(left))
				Expect(complexFromContext(left, k0)).To(Equal(v3))
				Expect(complexFromContext(left, k1)).To(Equal(v4))
				Expect(complexFromContext(left, k2)).To(Equal(v2))
			})
		})
	})

	Describe("NewContext", func() {
		It("should return a valid context", func() {
			Expect(complexValidateContext(complexNewContext())).To(BeTrue())
		})
	})

	Describe("WithContext", func() {
		When("parent is nil", func() {
			It("should panic", func() {
				fn := func() {
					//nolint:staticcheck
					_ = complexWithContext(nil)
				}
				Expect(fn).To(PanicWith("parent context is nil"))
			})
		})
		When("parent is not nil", func() {
			It("should return a context", func() {
				ctx := complexWithContext(context.Background())
				Expect(ctx).ToNot(BeNil())
			})
		})
	})
})
