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
//
// This assumes vSphere guest OS identifiers are unique under the transform
// above: two distinct IDs differing only in case, or only in characters this
// function collapses to "-", would collide onto the same name. vSphere's
// guest IDs are camelCase-alphanumeric (e.g. "otherLinux64Guest"), so this
// does not happen in practice today. Callers that fan out per-descriptor
// writes keyed by this name (see
// controllers/virtualmachineconfigoptions's reconcileGuestOptions) rely on
// that assumption: a real collision would make those writes race with each
// other every reconcile, since each descriptor would keep overwriting the
// other's Spec.ID.
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
