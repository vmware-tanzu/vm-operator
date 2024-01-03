// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
)

var _ = Describe("Context", func() {

	Describe("NewContext", func() {
		It("Should return a new context with an empty config", func() {
			ctx := pkgconfig.NewContext()
			Expect(ctx).ToNot(BeNil())
			config := pkgconfig.FromContext(ctx)
			Expect(config).To(Equal(pkgconfig.Config{}))
		})
	})

	Describe("NewContextWithDefaultConfig", func() {
		It("Should return a new context with a default config", func() {
			ctx := pkgconfig.NewContextWithDefaultConfig()
			Expect(ctx).ToNot(BeNil())
			config := pkgconfig.FromContext(ctx)
			Expect(config).To(Equal(pkgconfig.Default()))
		})
	})

	Describe("FromContext", func() {
		When("Context is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgconfig.FromContext(nil) //nolint:staticcheck
				}).Should(PanicWith("context is nil"))
			})
		})
		When("Context missing the config", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgconfig.FromContext(context.Background())
				}).Should(PanicWith("config is missing from context"))
			})
		})
	})

	Describe("JoinContext", func() {
		When("Parent context is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgconfig.JoinContext(nil, pkgconfig.NewContext()) //nolint:staticcheck
				}).Should(PanicWith("parent context is nil"))
			})
		})
		When("Context with config is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgconfig.JoinContext(context.Background(), nil)
				}).Should(PanicWith("contextWithConfig is nil"))
			})
		})
		When("Context with config is missing the config", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgconfig.JoinContext(context.Background(), context.Background())
				}).Should(PanicWith("config is missing from context"))
			})
		})
		When("All arguments are valid", func() {
			It("Should return a new context with the config", func() {
				out := pkgconfig.JoinContext(
					context.Background(),
					pkgconfig.WithConfig(pkgconfig.Config{
						ContentAPIWait: 100 * time.Hour,
					}))
				Expect(out).ToNot(BeNil())
				Expect(pkgconfig.FromContext(out).ContentAPIWait).To(Equal(100 * time.Hour))
			})
		})
	})

	Describe("WithContext", func() {
		When("Parent context is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = pkgconfig.WithContext(nil, pkgconfig.Config{}) //nolint:staticcheck
				}).Should(PanicWith("parent context is nil"))
			})
		})
	})

	Describe("SetContext", func() {
		When("Context is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					pkgconfig.SetContext(nil, func(*pkgconfig.Config) {}) //nolint:staticcheck
				}).Should(PanicWith("context is nil"))
			})
		})
		When("SetFn is nil", func() {
			It("Should panic", func() {
				Expect(func() {
					pkgconfig.SetContext(pkgconfig.NewContext(), nil)
				}).Should(PanicWith("setFn is nil"))
			})
		})
		When("Context is missing the config", func() {
			It("Should panic", func() {
				Expect(func() {
					pkgconfig.SetContext(context.Background(), func(*pkgconfig.Config) {})
				}).Should(PanicWith("config is missing from context"))
			})
		})
		When("All arguments are valid", func() {
			It("Should update the config in the context", func() {
				ctx := pkgconfig.WithConfig(pkgconfig.Config{
					ContainerNode:  true,
					ContentAPIWait: 0,
				})
				pkgconfig.SetContext(ctx, func(config *pkgconfig.Config) {
					config.ContainerNode = false
					config.ContentAPIWait = 100 * time.Hour
				})
				Expect(pkgconfig.FromContext(ctx).ContainerNode).To(BeFalse())
				Expect(pkgconfig.FromContext(ctx).ContentAPIWait).To(Equal(100 * time.Hour))
			})
		})
	})
})
