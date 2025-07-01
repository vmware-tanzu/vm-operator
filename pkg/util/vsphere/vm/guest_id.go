// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// GuestIDProperty is the property name for the guest ID in the config spec.
const GuestIDProperty = "configSpec.guestId"

// IsTaskInfoErrorInvalidGuestID returns true if the provided taskInfo contains
// an error with a fault that indicates the underlying cause is an invalid guest
// ID.
func IsTaskInfoErrorInvalidGuestID(taskInfo *vimtypes.TaskInfo) bool {
	if taskInfo == nil {
		return false
	}
	if taskInfo.Error == nil {
		return false
	}
	if f, ok := taskInfo.Error.Fault.(*vimtypes.InvalidArgument); ok {
		return strings.Contains(f.InvalidProperty, GuestIDProperty)
	}
	return false
}
