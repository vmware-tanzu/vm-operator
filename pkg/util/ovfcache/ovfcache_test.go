// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package ovfcache_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/ovf"

	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
)

var _ = Describe("WithContext", func() {
	It("should succeed", func() {
		Expect(ovfcache.WithContext(context.Background())).ToNot(BeNil())
	})

})

var _ = Describe("GetOVFEnvelope", func() {

	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = ovfcache.WithContext(context.Background())
		ovfcache.SetGetter(
			ctx,
			func(ctx context.Context, itemID string) (*ovf.Envelope, error) {
				return &ovf.Envelope{}, nil
			})
	})

	AfterEach(func() {
		ctx = nil
	})

	It("should succeed", func() {
		env, err := ovfcache.GetOVFEnvelope(ctx, "fake", "v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(env).To(Equal(&ovf.Envelope{}))
	})

})
