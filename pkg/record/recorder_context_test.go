// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package record_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apirecord "k8s.io/client-go/tools/record"

	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

var _ = Describe("WithContext", func() {
	var (
		left    context.Context
		leftVal record.Recorder
	)
	BeforeEach(func() {
		left = context.Background()
		leftVal = record.New(apirecord.NewFakeRecorder(0))
	})
	When("parent is nil", func() {
		BeforeEach(func() {
			left = nil
		})
		It("should panic", func() {
			fn := func() {
				_ = record.WithContext(left, leftVal)
			}
			Expect(fn).To(PanicWith("parent context is nil"))
		})
	})
	When("parent is not nil", func() {
		When("value is nil", func() {
			BeforeEach(func() {
				leftVal = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = record.WithContext(left, leftVal)
				}
				Expect(fn).To(PanicWith("recorder is nil"))
			})
		})
		When("value is not nil", func() {
			It("should return a new context", func() {
				ctx := record.WithContext(left, leftVal)
				Expect(ctx).ToNot(BeNil())
				Expect(record.FromContext(ctx)).To(Equal(leftVal))
			})
		})
	})
})
