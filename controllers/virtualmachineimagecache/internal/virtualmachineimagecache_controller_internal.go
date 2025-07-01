// // © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package internal

type contextKey uint8

const (
	// NewCacheStorageURIsClientContextKey is used for testing.
	NewCacheStorageURIsClientContextKey contextKey = iota

	// NewContentLibraryProviderContextKey is used for testing.
	NewContentLibraryProviderContextKey
)
