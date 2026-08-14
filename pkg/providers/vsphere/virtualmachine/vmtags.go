// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// TagResourceName returns the derived name of the Tag resource that
// represents the given label key/value pair: "tag-" followed by the first
// 17 hex characters of the SHA-1 sum of "<key>:<value>".
func TagResourceName(key, value string) string {
	return "tag-" + pkgutil.SHA1Sum17(VCenterTagName(key, value))
}

// VCenterTagName returns the vCenter tag name for the given label
// key/value pair: "<key>:<value>". The tag category is the namespace.
func VCenterTagName(key, value string) string {
	return key + ":" + value
}
