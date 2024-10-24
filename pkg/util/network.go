// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"net"
	"regexp"
)

// hostNameFormat matches strings that:
//
//   - do not exceed 63 characters
//   - contain only alpha-numeric characters, dashes, or symbol-point unicode
//     (ex. emoji)
//   - does not begin or end with a dash
const hostNameFormat = `^([a-zA-Z0-9\p{S}\p{L}]{1,2}|(?:[a-zA-Z0-9\p{S}\p{L}][a-zA-Z0-9-\p{S}\p{L}]{0,61}[a-zA-Z0-9\p{S}\p{L}]))$`

var hostNameRx = regexp.MustCompile(hostNameFormat)

// IsValidHostName returns true if the provided value adheres to the format of
// an RFC-1123 DNS label, which may be:
//
//   - an IP4 address
//   - an IP6 address
//   - a string of alpha-numeric characters that:
//   - must not exceed 63 characters in length
//   - may begin with a digit
//   - may contain dashes except as the first or last character
//   - must not contain underscores
//   - may contain symbol-point unicode, (ex. emoji)
//
// The IsValidHostName and IsValidDomainName functions are defined as part of
// this project because the similar functions from the Kubernetes API machinery
// and OpenAPI projects were different enough to not be compatible with the
// type of required validation.
//
//   - The IsFullyQualifiedDomainName function from
//     https://github.com/kubernetes/apimachinery/blob/d5c9711b77ee5a0dde0fef41c9ca86a67f5ddb4e/pkg/util/validation/validation.go#L92-L115:
//     -- requires a minimum of two segments
//     -- does not permit IP4 or IP6 addresses
//     -- does not permit upper-case characters
//     -- does not permit symbol-point unicode
//
//   - The IsDNS1123Label function from
//     https://github.com/kubernetes/apimachinery/blob/d5c9711b77ee5a0dde0fef41c9ca86a67f5ddb4e/pkg/util/validation/validation.go#L185-L202:
//     -- does not permit IP4 or IP6 addresses
//     -- does not permit upper-case characters
//     -- does not permit symbol-point unicode
//
//   - The IsHostName function from
//     https://github.com/go-openapi/strfmt/blob/f065ed8a46434a6407610166cd707c170cf1e483/default.go#L92-L112:
//     -- does not permit IP4 or IP6 addresses
//     -- does not distinguish between a host name and an FQDN
//
// Since none of the above, upstream functions could be re-used as-is, instead
// the regular expression HostnamePattern from
// https://github.com/go-openapi/strfmt/blob/f065ed8a46434a6407610166cd707c170cf1e483/default.go#L33-L60C21
// was used as the basis for this file's hostNameFormat and fqdnFormat regular
// expressions. Additionally, the level of testing in network_test.go for both
// the IsValidHostName and IsValidDomainName functions far exceeds the testing
// for the aforementioned, upstream functions.
func IsValidHostName(s string) bool {
	return net.ParseIP(s) != nil || hostNameRx.MatchString(s)
}

// fqdnFormat matches strings that are fully-qualified domain names.
const fqdnFormat = `^((?:(?:(?:[a-zA-Z0-9\p{S}\p{L}]{1,2})|(?:[a-zA-Z0-9\p{S}\p{L}][-a-zA-Z0-9\p{S}\p{L}]{0,61}[a-zA-Z0-9\p{S}\p{L}]))\.){0,}(?:(?:[a-zA-Z0-9]{2})|(?:[a-zA-Z0-9][-a-zA-Z0-9]{0,61}[a-zA-Z0-9])))$`

var fqdnNameRx = regexp.MustCompile(fqdnFormat)

// IsValidDomainName returns true if the provided value adheres to the format
// for DNS names specified in RFC-1034, Section 3.5:
//
//   - must not exceed 255 characters in length when combined with the host name
//   - individual segments must be 63 characters or less.
//   - the top-level domain( ex. ".com"), is at least two letters with no
//     special characters.
//   - underscores are not allowed.
//   - dashes are permitted, but not at the start or end of the value.
//   - long, top-level domain names (ex. ".london") are permitted.
//   - symbol unicode points, such as emoji, are disallowed in the top-level
//     domain.
//
// Please refer to the comments for the IsValidHostName function for information
// on why both functions were defined as part of this project instead of using
// existing functions from the Kubernetes API machinery project or OpenAPI's
// validation utility functions.
func IsValidDomainName(s string) bool {
	// Given a host name must be at least one character long followed by a "."
	// character, the domain name only has 253 characters of available space.
	return len(s) <= 253 && fqdnNameRx.MatchString(s)
}

func Dedupe(slice []string) []string {
	// Create a map to track unique elements
	seen := make(map[string]bool)
	result := []string{}

	// Iterate over the slice
	for _, str := range slice {
		// Check if the element has been seen before
		if !seen[str] {
			seen[str] = true
			result = append(result, str)
		}
	}

	return result
}
