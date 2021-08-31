// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package lib

import (
	"os"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MaxAllowedConcurrentVMsOnProvider", func() {
	Context("when the MAX_CREATE_VMS_ON_PROVIDER env is set", func() {
		AfterEach(func() {
			Expect(os.Unsetenv(MaxCreateVMsOnProviderEnv)).To(Succeed())
		})

		Context("with a valid env value", func() {
			It("returns the value from the env", func() {
				expectedVal := 50
				Expect(os.Setenv(MaxCreateVMsOnProviderEnv, strconv.Itoa(expectedVal))).To(Succeed())

				Expect(MaxConcurrentCreateVMsOnProvider()).To(Equal(expectedVal))
			})
		})

		Context("with an invalid env value", func() {
			It("returns the default value", func() {
				invalidVal := "-42x"
				Expect(os.Setenv(MaxCreateVMsOnProviderEnv, invalidVal)).To(Succeed())

				Expect(MaxConcurrentCreateVMsOnProvider()).To(Equal(DefaultMaxCreateVMsOnProvider))
			})
		})
	})

	Context("when the MAX_CREATE_VMS_ON_PROVIDER env is not set", func() {
		It("returns the default value", func() {
			Expect(MaxConcurrentCreateVMsOnProvider()).To(Equal(DefaultMaxCreateVMsOnProvider))
		})
	})
})
