// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// ErrorMessageFromTaskInfo extracts a comprehensive error message from a TaskInfo.
// It combines the localized message with all fault messages to provide complete
// error details. This is useful when vSphere returns a generic localized message
// (e.g., ending with a colon) followed by detailed fault messages.
//
// Example:
//   - LocalizedMessage: "Customization of guest OS is not supported: "
//   - FaultMessage: "Tools version 7.4.3 installed in the GuestOS is not supported."
//   - Result: "Customization of guest OS is not supported: Tools version 7.4.3 installed in the GuestOS is not supported."
func ErrorMessageFromTaskInfo(taskInfo *vimtypes.TaskInfo) string {
	if taskInfo == nil || taskInfo.Error == nil {
		return ""
	}

	taskErr := taskInfo.Error
	fault := taskErr.Fault.GetMethodFault()
	if fault == nil || len(fault.FaultMessage) == 0 {
		return taskErr.LocalizedMessage
	}

	// Build a comprehensive error message from all fault messages.
	var faultMsgs []string
	for _, fm := range fault.FaultMessage {
		if fm.Message != "" {
			faultMsgs = append(faultMsgs, fm.Message)
		}
	}

	if len(faultMsgs) == 0 {
		return taskErr.LocalizedMessage
	}

	// Combine the localized message (if present) with the fault messages.
	localizedMsg := taskErr.LocalizedMessage
	if localizedMsg != "" {
		// Remove trailing colons and whitespace from localized message
		// as vSphere often returns messages like "Operation failed due to: ".
		localizedMsg = strings.TrimRight(localizedMsg, ": \n\r\t")
		return fmt.Sprintf("%s: %s", localizedMsg, strings.Join(faultMsgs, "; "))
	}

	return strings.Join(faultMsgs, "; ")
}
