// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"regexp"
	"strings"
)

// nonAlnumHyphenRE matches any character that is not a lowercase letter,
// digit, or hyphen.
var nonAlnumHyphenRE = regexp.MustCompile(`[^a-z0-9-]+`)

// VimGuestOptionsName converts a guest OS identifier into the
// DNS-subdomain-safe name used for the corresponding VirtualMachineGuestOptions
// object's metadata.name.
func VimGuestOptionsName(guestID string) string {
	s := strings.ToLower(guestID)
	s = nonAlnumHyphenRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = s[:63]
		s = strings.TrimRight(s, "-")
	}
	return s
}
