// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package library_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/vapi/library"

	clsutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/library"
)

type fakeSyncLibraryItemClient struct {
	getLibItemItem   *library.Item
	getLibItemErr    error
	getLibItemCalls  int32
	syncLibItemErr   error
	syncLibItemCalls int32
}

func (m *fakeSyncLibraryItemClient) GetLibraryItem(
	ctx context.Context,
	id string) (*library.Item, error) {

	_ = atomic.AddInt32(&m.getLibItemCalls, 1)
	return m.getLibItemItem, m.getLibItemErr
}

func (m *fakeSyncLibraryItemClient) SyncLibraryItem(
	ctx context.Context,
	item *library.Item, force bool) error {

	_ = atomic.AddInt32(&m.syncLibItemCalls, 1)
	return m.syncLibItemErr
}

var _ = Describe("SyncLibraryItem", func() {

	var (
		ctx    context.Context
		client *fakeSyncLibraryItemClient
		itemID string
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = &fakeSyncLibraryItemClient{}
		itemID = "fake"
	})

	When("it should panic", func() {
		When("context is nil", func() {
			BeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				Expect(func() {
					_ = clsutil.SyncLibraryItem(ctx, client, itemID)
				}).To(PanicWith("context is nil"))
			})
		})
		When("client is nil", func() {
			BeforeEach(func() {
				client = nil
			})
			It("should panic", func() {
				client = nil
				Expect(func() {
					_ = clsutil.SyncLibraryItem(ctx, client, itemID)
				}).To(PanicWith("client is nil"))
			})
		})
		When("library item ID is empty", func() {
			BeforeEach(func() {
				itemID = ""
			})
			It("should panic", func() {
				Expect(func() {
					_ = clsutil.SyncLibraryItem(ctx, client, itemID)
				}).To(PanicWith("itemID is empty"))
			})
		})
	})

	When("it should not panic", func() {

		var (
			syncErr error
		)

		BeforeEach(func() {
			ctx = logr.NewContext(context.Background(), GinkgoLogr)
		})

		JustBeforeEach(func() {
			syncErr = clsutil.SyncLibraryItem(ctx, client, itemID)
		})

		When("item does not exist", func() {
			BeforeEach(func() {
				client.getLibItemErr = errors.New("404 Not Found")
			})
			It("should 404", func() {
				Expect(syncErr).To(HaveOccurred())
				Expect(syncErr).To(MatchError(fmt.Errorf(
					"error getting library item %s: %w", itemID,
					client.getLibItemErr)))
				Expect(atomic.LoadInt32(&client.getLibItemCalls)).To(Equal(int32(1)))

			})
		})

		When("item does exist", func() {
			BeforeEach(func() {
				client.getLibItemItem = &library.Item{
					ID:   itemID,
					Type: "LOCAL",
				}
			})
			When("the item is local", func() {
				BeforeEach(func() {
					client.getLibItemItem.Type = "LOCAL"
				})
				It("should succeed without calling sync", func() {
					Expect(syncErr).ToNot(HaveOccurred())
					Expect(atomic.LoadInt32(&client.getLibItemCalls)).To(Equal(int32(1)))
					Expect(atomic.LoadInt32(&client.syncLibItemCalls)).To(Equal(int32(0)))
				})
			})
			When("the item is subscribed", func() {
				BeforeEach(func() {
					client.getLibItemItem.Type = "SUBSCRIBED"
				})
				When("there is an issue with syncing", func() {
					BeforeEach(func() {
						client.syncLibItemErr = errors.New("timed out")
					})
					It("should return an error", func() {
						Expect(syncErr).To(HaveOccurred())
						Expect(syncErr).To(MatchError(fmt.Errorf(
							"error syncing library item %s: %w", itemID,
							client.syncLibItemErr)))
						Expect(atomic.LoadInt32(&client.getLibItemCalls)).To(Equal(int32(1)))
						Expect(atomic.LoadInt32(&client.syncLibItemCalls)).To(Equal(int32(1)))
					})
				})
				When("there is no issue with syncing", func() {
					BeforeEach(func() {
						client.getLibItemItem = &library.Item{
							ID: itemID,
						}
					})
					It("should succeed", func() {
						Expect(syncErr).ToNot(HaveOccurred())
						Expect(atomic.LoadInt32(&client.getLibItemCalls)).To(Equal(int32(1)))
						Expect(atomic.LoadInt32(&client.syncLibItemCalls)).To(Equal(int32(1)))
					})
				})
			})
		})
	})
})
