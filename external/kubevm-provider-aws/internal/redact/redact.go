// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package redact strips credential-shaped values out of a platform message
// before it reaches a condition, an event or a log.
//
// Two obligations, and the second is the one that gets forgotten. Nothing
// credential-shaped may pass. And what comes out must still be usable:
// non-empty, still naming the failing operation, still carrying the platform's
// own error code. Redaction that empties a message trades a leak for an
// undebuggable system, and an operator reading nothing but a marker cannot
// tell which IAM action to add.
//
// What this deliberately does NOT touch matters as much as what it removes.
// AROA and AIDA identifiers, ARNs, instance, image and subnet ids share the
// shape of a credential and are not one. A matcher that flags them produces
// findings nobody can act on, and a matcher that cries wolf gets muted --
// after which it catches nothing at all.
package redact

import (
	"regexp"
	"strings"
)

// Marker is what replaces a removed value.
//
// Visible on purpose: a message that was silently shortened reads as though
// the platform said less than it did.
const Marker = "[redacted]"

// accessKeyID matches the two access key id prefixes that are credentials.
//
// AKIA is a long-lived key and ASIA a temporary session key. AROA (a role) and
// AIDA (a user) share the shape exactly and are identifiers rather than
// secrets, so their absence here is a decision, not an oversight.
var accessKeyID = regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)

// secretAssignment matches a secret access key that has been given its name.
//
// Bare 40-character base64 is far too common to match on its own: it would hit
// hashes, checksums and generated ids, which is precisely how a scanner earns
// its way into everybody's ignore list.
var secretAssignment = regexp.MustCompile(
	`(?i)aws_secret_access_key\s*[:=]\s*["']?[A-Za-z0-9/+=]{40}["']?`)

// Redact removes credential-shaped values from a platform message while
// leaving it non-empty and still readable.
func Redact(s string) string {
	out := secretAssignment.ReplaceAllString(s, Marker)
	out = accessKeyID.ReplaceAllString(out, Marker)

	// A message redacted down to nothing is worse than no message at all: it
	// is indistinguishable from a reconcile that never reported anything, and
	// whoever has to debug this has lost the operation and the error code
	// along with the secret.
	if strings.TrimSpace(out) == "" {
		return Marker
	}
	return out
}
