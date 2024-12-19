// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package library

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/vapi/library"

	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// SyncLibraryItemClient implements the client methods used by the
// SyncLibraryItem method.
type SyncLibraryItemClient interface {
	GetLibraryItem(ctx context.Context, id string) (*library.Item, error)
	SyncLibraryItem(ctx context.Context, item *library.Item, force bool) error
}

// SyncLibraryItem issues a sync call to the provided library item.
func SyncLibraryItem(
	ctx context.Context,
	client SyncLibraryItemClient,
	itemID string) error {

	if pkgutil.IsNil(ctx) {
		panic("context is nil")
	}
	if pkgutil.IsNil(client) {
		panic("client is nil")
	}
	if itemID == "" {
		panic("itemID is empty")
	}

	logger := logr.FromContextOrDiscard(ctx)

	// A file from a library item that belongs to a subscribed library may not
	// be fully available. Sync the file to ensure it is present.
	logger.Info("Syncing content library item", "libraryItemID", itemID)
	libItem, err := client.GetLibraryItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf(
			"error getting library item %s: %w", itemID, err)
	}
	if libItem.Type == "LOCAL" {
		return nil
	}
	if err := client.SyncLibraryItem(ctx, libItem, true); err != nil {
		return fmt.Errorf(
			"error syncing library item %s: %w", itemID, err)
	}
	return nil
}
