// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"crypto/md5"
	"fmt"
)

// GeneratePVCName generates a DNS-safe PVC name by concatenating the name of
// the VM, a hyphen, and the first eight characters of an MD5 hash of the
// diskUUID.
func GeneratePVCName(vmName, diskUUID string) string {
	// Get an MD5 hash of the disk's UUID.
	hash := md5.New()
	_, _ = hash.Write([]byte(diskUUID))
	h := fmt.Sprintf("%x", hash.Sum(nil))

	if len(vmName) > 54 { // DNS name is 63, less 8char for h, less 1char for -.
		vmName = vmName[:54]
	}

	return fmt.Sprintf("%s-%s", vmName, h[:8])
}
