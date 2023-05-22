// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"regexp"
	"strconv"
)

var vmxRe = regexp.MustCompile(`vmx-(\d+)`)

// ParseVirtualHardwareVersion parses the virtual hardware version
// For eg. "vmx-15" returns 15.
func ParseVirtualHardwareVersion(vmxVersion string) int32 {
	// obj matches the full string and the submatch (\d+)
	// and return a []string with values
	obj := vmxRe.FindStringSubmatch(vmxVersion)
	if len(obj) != 2 {
		return 0
	}

	version, err := strconv.ParseInt(obj[1], 10, 32)
	if err != nil {
		return 0
	}

	return int32(version)
}
