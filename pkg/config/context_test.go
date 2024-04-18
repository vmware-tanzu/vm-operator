// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

var _ = Describe("Context", func() {

	Describe("NewContext", func() {
		It("Should return a new context with an empty config", func() {
			ctx := pkgcfg.NewContext()
			Expect(ctx).ToNot(BeNil())
			config := pkgcfg.FromContext(ctx)
			Expect(config).To(Equal(pkgcfg.Config{}))
		})
	})

	Describe("NewContextWithDefaultConfig", func() {
		It("Should return a new context with a default config", func() {
			ctx := pkgcfg.NewContextWithDefaultConfig()
			Expect(ctx).ToNot(BeNil())
			config := pkgcfg.FromContext(ctx)
			Expect(config).To(Equal(pkgcfg.Default()))
		})
	})

	Describe("FromContext", func() {
		When("Context is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgcfg.FromContext(nil) //nolint:staticcheck
				}).Should(PanicWith("context is nil"))
			})
		})
		When("Context missing the config", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgcfg.FromContext(context.Background())
				}).Should(PanicWith("config is missing from context"))
			})
		})
	})

	Describe("JoinContext", func() {
		When("Parent context is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgcfg.JoinContext(nil, pkgcfg.NewContext()) //nolint:staticcheck
				}).Should(PanicWith("parent context is nil"))
			})
		})
		When("Context with config is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgcfg.JoinContext(context.Background(), nil)
				}).Should(PanicWith("contextWithConfig is nil"))
			})
		})
		When("Context with config is missing the config", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgcfg.JoinContext(context.Background(), context.Background())
				}).Should(PanicWith("config is missing from context"))
			})
		})
		When("All arguments are valid", func() {
			It("Should return a new context with the config", func() {
				out := pkgcfg.JoinContext(
					context.Background(),
					pkgcfg.WithConfig(pkgcfg.Config{
						ContentAPIWait: 100 * time.Hour,
					}))
				Expect(out).ToNot(BeNil())
				Expect(pkgcfg.FromContext(out).ContentAPIWait).To(Equal(100 * time.Hour))
			})
		})
	})

	Describe("WithContext", func() {
		When("Parent context is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgcfg.WithContext(nil, pkgcfg.Config{}) //nolint:staticcheck
				}).Should(PanicWith("parent context is nil"))
			})
		})
	})

	Describe("SetContext", func() {
		When("Context is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					pkgcfg.SetContext(nil, func(*pkgcfg.Config) {}) //nolint:staticcheck
				}).Should(PanicWith("context is nil"))
			})
		})
		When("SetFn is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					pkgcfg.SetContext(pkgcfg.NewContext(), nil)
				}).Should(PanicWith("setFn is nil"))
			})
		})
		When("Context is missing the config", func() {
			It("Should panic", func() {
				Expect(func() {
					pkgcfg.SetContext(context.Background(), func(*pkgcfg.Config) {})
				}).Should(PanicWith("config is missing from context"))
			})
		})
		When("All arguments are valid", func() {
			It("Should update the config in the context", func() {
				ctx := pkgcfg.WithConfig(pkgcfg.Config{
					ContainerNode:  true,
					ContentAPIWait: 0,
				})
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.ContainerNode = false
					config.ContentAPIWait = 100 * time.Hour
				})
				Expect(pkgcfg.FromContext(ctx).ContainerNode).To(BeFalse())
				Expect(pkgcfg.FromContext(ctx).ContentAPIWait).To(Equal(100 * time.Hour))
			})
		})
	})
})
