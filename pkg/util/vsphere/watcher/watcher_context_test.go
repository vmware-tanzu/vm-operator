// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package watcher_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
)

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
				_ = watcher.JoinContext(left, right)
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
				_ = watcher.JoinContext(left, right)
			}
			Expect(fn).To(PanicWith("right context is nil"))
		})
	})
	When("value is missing from context", func() {
		It("should panic", func() {
			fn := func() {
				_ = watcher.JoinContext(left, right)
			}
			Expect(fn).To(PanicWith("value is missing from context"))
		})
	})
	When("the left context has a channels object", func() {
		BeforeEach(func() {
			left = watcher.NewContext()
		})
		It("should return the left context", func() {
			ctx := watcher.JoinContext(left, right)
			Expect(ctx).ToNot(BeNil())
		})
	})
	When("the right context has a channels object", func() {
		BeforeEach(func() {
			right = watcher.NewContext()
		})
		It("should return a new context", func() {
			ctx := watcher.JoinContext(left, right)
			Expect(ctx).ToNot(BeNil())
			Expect(watcher.ValidateContext(ctx)).To(BeTrue())
		})
	})
	When("both contexts have a channels object", func() {
		BeforeEach(func() {
			left = watcher.NewContext()
			right = watcher.NewContext()
		})
		It("should return a new context", func() {
			ctx := watcher.JoinContext(left, right)
			Expect(ctx).ToNot(BeNil())
			Expect(watcher.ValidateContext(ctx)).To(BeTrue())
		})
	})
})

var _ = Describe("WithContext", func() {
	When("parent is nil", func() {
		It("should panic", func() {
			fn := func() {
				//nolint:staticcheck
				_ = watcher.WithContext(nil)
			}
			Expect(fn).To(PanicWith("parent context is nil"))
		})
	})
	When("parent is not nil", func() {
		It("should return a context", func() {
			ctx := watcher.NewContext()
			Expect(ctx).ToNot(BeNil())
		})
	})
})

var _ = Describe("Add", func() {
	When("there is no watcher", func() {
		It("should fail", func() {
			Expect(watcher.Add(
				watcher.WithContext(pkgcfg.NewContext()),
				vimtypes.ManagedObjectReference{},
				"fake",
			)).To(MatchError(watcher.ErrNoWatcher))
		})
	})
	When("async signal is disabled", func() {
		It("should fail", func() {
			Expect(watcher.Add(
				watcher.WithContext(pkgcfg.WithConfig(
					pkgcfg.Config{AsyncSignalDisabled: true})),
				vimtypes.ManagedObjectReference{},
				"fake",
			)).To(MatchError(watcher.ErrAsyncSignalDisabled))
		})
	})

})

var _ = Describe("Remove", func() {
	When("there is no watcher", func() {
		It("should fail", func() {
			Expect(watcher.Remove(
				watcher.WithContext(pkgcfg.NewContext()),
				vimtypes.ManagedObjectReference{},
				"fake",
			)).To(MatchError(watcher.ErrNoWatcher))
		})
	})
	When("async signal is disabled", func() {
		It("should fail", func() {
			Expect(watcher.Remove(
				watcher.WithContext(pkgcfg.WithConfig(
					pkgcfg.Config{AsyncSignalDisabled: true})),
				vimtypes.ManagedObjectReference{},
				"fake",
			)).To(MatchError(watcher.ErrAsyncSignalDisabled))
		})
	})
})
