// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// TagResourceName returns the derived name of the Tag resource that
// represents the given label key/value pair in the given namespace:
// "tag-" followed by the 16 hex character XXHash64 digest of
// "<namespace>:<key>:<value>".
// Using XXHash64 instead of SHA1 to avoid gosec warnings.
func TagResourceName(namespace, key, value string) string {
	return "tag-" + pkgutil.XXHash64Hex(namespace+":"+TagName(key, value))
}

// TagName returns the tag name for the given label
// key/value pair: "<key>:<value>". The tag category is the namespace.
func TagName(key, value string) string {
	return key + ":" + value
}
