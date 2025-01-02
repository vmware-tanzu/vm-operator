// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
)

// SliceToString returns a comma-separated string for the provided slice.
// Any "," characters in the elements themselves are escaped with the "\"
// character.
// Empty elements or elements that are empty after all leading/trailing white-
// space is trimmed are ignored.
func SliceToString(s []string) string {
	var slice []string
	for i := range s {
		if s := strings.TrimSpace(s[i]); s != "" {
			slice = append(slice, strings.ReplaceAll(s, ",", `\,`))
		}
	}
	if len(slice) == 0 {
		return ""
	}
	return strings.Join(slice, ",")
}

func StringToSet(s string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, s := range StringToSlice(s) {
		set[s] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func StringToSlice(s string) []string {
	if s == "" {
		return nil
	}
	// Replace all instances of a comma
	s = strings.ReplaceAll(s, `\,`, "\x00")
	var slice []string
	for _, s := range strings.Split(strings.TrimSpace(s), ",") {
		if s := strings.TrimSpace(s); s != "" {
			slice = append(slice, strings.ReplaceAll(s, "\x00", ","))
		}
	}
	if len(slice) == 0 {
		return nil
	}
	return slice
}
