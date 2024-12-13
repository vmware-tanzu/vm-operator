// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package internal_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/ovf"

	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache/internal"
)

const fakeString = "fake"

var _ = Describe("WithContext", func() {
	const (
		maxItems            = 3
		expireAfter         = 3 * time.Second
		expireCheckInterval = 1 * time.Second
	)

	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = internal.WithContext(
			context.Background(),
			maxItems,
			expireAfter,
			expireCheckInterval)
	})

	AfterEach(func() {
		ctx = nil
	})

	It("should succeed", func() {
		Expect(ctx).ToNot(BeNil())
	})

	It("Should delete lock for expired item", func() {
		Expect(ctx).ToNot(BeNil())

		const itemID = fakeString

		r := internal.Put(ctx, itemID, internal.VersionedOVFEnvelope{})
		Expect(r).To(Equal(pkgutil.CachePutResultCreate))

		curItemLock := internal.GetLock(ctx, itemID)
		Expect(curItemLock).ToNot(BeNil())

		Eventually(func() bool {
			// A new lock is returned if the item is not found, thus the lock
			// should be different after the item is expired and deleted from
			// the pool.
			return internal.GetLock(ctx, itemID) != curItemLock
		}, 5*time.Second, 1*time.Second).Should(BeTrue())
	})
})

var _ = Describe("JoinContext", func() {

	const (
		maxItems            = 3
		expireAfter         = 3 * time.Second
		expireCheckInterval = 1 * time.Second
	)

	var (
		left  context.Context
		right context.Context
	)

	BeforeEach(func() {
		left = internal.WithContext(
			context.Background(),
			maxItems,
			expireAfter,
			expireCheckInterval)

		right = internal.WithContext(
			context.Background(),
			maxItems,
			expireAfter,
			expireCheckInterval)
	})

	AfterEach(func() {
		left, right = nil, nil
	})

	When("left has the cache", func() {
		BeforeEach(func() {
			right = context.Background()
		})
		It("should have the cache", func() {
			Expect(internal.Cache(internal.JoinContext(left, right))).To(Equal(internal.Cache(left)))
		})
	})

	When("right has the cache", func() {
		BeforeEach(func() {
			left = context.Background()
		})
		It("should have the cache", func() {
			Expect(internal.Cache(internal.JoinContext(left, right))).To(Equal(internal.Cache(right)))
		})
	})

	When("both have the cache", func() {
		It("should have the right cache", func() {
			Expect(internal.Cache(internal.JoinContext(left, right))).To(Equal(internal.Cache(right)))
		})
	})
})

var _ = Describe("GetOVFEnvelope", func() {

	const (
		maxItems            = 100
		expireAfter         = 30 * time.Minute
		expireCheckInterval = 5 * time.Minute
	)

	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = internal.WithContext(
			context.Background(),
			maxItems,
			expireAfter,
			expireCheckInterval)
	})

	AfterEach(func() {
		ctx = nil
	})

	When("there is no getter", func() {

		It("should return an error", func() {
			env, err := internal.GetOVFEnvelope(ctx, fakeString, "v1")
			Expect(err).To(MatchError(internal.ErrNoGetter))
			Expect(env).To(BeNil())
		})
	})

	When("there is a getter that returns an error", func() {
		BeforeEach(func() {
			internal.SetGetter(
				ctx,
				func(ctx context.Context, itemID string) (*ovf.Envelope, error) {
					return &ovf.Envelope{}, errors.New(fakeString)
				})
		})
		It("should return the error", func() {
			env, err := internal.GetOVFEnvelope(ctx, fakeString, "v1")
			Expect(err).To(MatchError(fakeString))
			Expect(env).To(BeNil())

		})
	})

	When("there is a getter that returns an item", func() {
		BeforeEach(func() {
			internal.SetGetter(
				ctx,
				func(ctx context.Context, itemID string) (*ovf.Envelope, error) {
					return &ovf.Envelope{
						References: []ovf.File{},
					}, nil
				})
		})
		It("should return env", func() {
			env, err := internal.GetOVFEnvelope(ctx, fakeString, "v1")
			Expect(err).ToNot(HaveOccurred())
			Expect(env).To(Equal(&ovf.Envelope{
				References: []ovf.File{},
			}))

			// Change what the getter returns.
			internal.SetGetter(
				ctx,
				func(ctx context.Context, itemID string) (*ovf.Envelope, error) {
					return &ovf.Envelope{
						Network: &ovf.NetworkSection{},
					}, nil
				})

			// Assert the original, cached item is still returned.
			env, err = internal.GetOVFEnvelope(ctx, fakeString, "v1")
			Expect(err).ToNot(HaveOccurred())
			Expect(env).To(Equal(&ovf.Envelope{
				References: []ovf.File{},
			}))
		})
	})

})
