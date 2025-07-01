// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package internal

type powerStateContextKey uint8

const (
	// SoftTimeoutKey is the context key for the time.Duration value that may
	// be stored in the context. If this value is not present, then a default
	// timeout of five minutes is used.
	// Used for testing.
	SoftTimeoutKey powerStateContextKey = iota
)
