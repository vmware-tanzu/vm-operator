// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package env_test

import (
	"math"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/config/env"
)

var _ = Describe("VarName", func() {
	Describe("String", func() {
		When("VarName is set to an invalid value", func() {
			It("Should panic", func() {
				Expect(func() {
					_ = env.VarName(math.MaxUint8).String()
				}).Should(PanicWith("unknown environment variable"))
			})
		})
	})
})

var _ = Describe("Unsetenv", Ordered /* All tests run in order of appearance */, func() {
	var (
		all []env.VarName
	)

	BeforeAll(func() {
		all = env.All()
	})

	BeforeEach(func() {
		env.Unset()
		for i := range all {
			_, ok := os.LookupEnv(all[i].String())
			Expect(ok).To(BeFalse())
		}
	})

	When("No env vars are set", func() {
		It("Should run without errors", func() {
			env.Unset()
			for i := range all {
				_, ok := os.LookupEnv(all[i].String())
				Expect(ok).To(BeFalse())
			}
		})
	})
	When("All env vars are set", func() {
		BeforeEach(func() {
			for i := range all {
				Expect(os.Setenv(all[i].String(), "fake")).To(Succeed())
			}
			for i := range all {
				Expect(os.Getenv(all[i].String())).To(Equal("fake"))
			}
		})
		It("Should unset the env vars", func() {
			env.Unset()
			for i := range all {
				_, ok := os.LookupEnv(all[i].String())
				Expect(ok).To(BeFalse())
			}
		})
	})
})
