// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"crypto/sha1" //nolint:gosec // used for creating safe names
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/cespare/xxhash/v2"
)

// SHA1Sum17 returns the first 17 characters of the base64-encoded, SHA1
// sum created from the provided string.
func SHA1Sum17(s string) string {
	h := sha1.New() //nolint:gosec // used for creating safe names
	_, _ = io.WriteString(h, s)
	return hex.EncodeToString(h.Sum(nil))[:17]
}

// XXHash64Hex returns the full 64-bit xxhash.Sum64 digest of the provided
// string, formatted as 16 hex characters.
func XXHash64Hex(s string) string {
	return fmt.Sprintf("%016x", xxhash.Sum64String(s))
}

// VMIName returns the VMI name for a given library item ID.
func VMIName(itemID string) string {
	return "vmi-" + SHA1Sum17(strings.ReplaceAll(itemID, "-", ""))
}
