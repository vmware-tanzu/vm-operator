// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("FormatValue", func() {
	Context("input is a valid label value", func() {
		It("returns the input label", func() {
			inputLabel := "valid-k8s-label-value"
			ret, err := util.FormatValue(inputLabel)
			Expect(err).ToNot(HaveOccurred())
			Expect(ret).To(Equal(inputLabel))
		})
	})

	Context("input is not a valid label", func() {
		It("returns a hashed value", func() {
			inputLabel := strings.Repeat("a", 100)
			actual, err := util.FormatValue(inputLabel)
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Not(Equal(inputLabel)))

			hasher := fnv.New32a()
			_, err = hasher.Write([]byte(inputLabel))
			Expect(err).ToNot(HaveOccurred())
			expected := fmt.Sprintf("hash_%s_z", base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hasher.Sum(nil)))

			Expect(actual).To(Equal(expected))
		})
	})
})

var _ = Describe("MustFormatValue", func() {
	Context("input is not a valid label", func() {
		It("does not panic, returns a hashed value", func() {
			inputLabel := strings.Repeat("a", 100)
			actual := util.MustFormatValue(inputLabel)
			Expect(actual).To(Not(Equal(inputLabel)))

			hasher := fnv.New32a()
			_, err := hasher.Write([]byte(inputLabel))
			Expect(err).ToNot(HaveOccurred())
			expected := fmt.Sprintf("hash_%s_z", base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hasher.Sum(nil)))

			Expect(actual).To(Equal(expected))
		})
	})
})
