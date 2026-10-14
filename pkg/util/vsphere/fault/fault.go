// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package fault

import (
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// LocalizedMessagesFromFault extracts unique localized and FaultMessage details,
// including nested NoCompatibleHost errors. Messages are trimmed of trailing
// colons and whitespace and returned in first-seen order.
func LocalizedMessagesFromFault(lmf vimtypes.LocalizedMethodFault) []string {
	return LocalizedMessagesFromFaults([]vimtypes.LocalizedMethodFault{lmf})
}

func localizedMessagesFromFault(lmf vimtypes.LocalizedMethodFault) []string {
	var messages []string

	if lmf.LocalizedMessage != "" {
		msg := strings.TrimRight(lmf.LocalizedMessage, ": \n\r\t")
		if msg != "" {
			messages = append(messages, msg)
		}
	}

	if lmf.Fault == nil {
		return messages
	}

	// Include the fault's own messages, which carry the specific reason
	// (e.g. a GenericDrsFault's incompatible candidate zone).
	for _, fm := range lmf.Fault.GetMethodFault().FaultMessage {
		if msg := strings.TrimRight(fm.Message, ": \n\r\t"); msg != "" {
			messages = append(messages, msg)
		}
	}

	// Include localized and FaultMessage details from nested host faults.
	if nch, ok := lmf.Fault.(*vimtypes.NoCompatibleHost); ok {
		for _, nestedLmf := range nch.Error {
			messages = append(messages, localizedMessagesFromFault(nestedLmf)...)
		}
	}

	return messages
}

// LocalizedMessagesFromFaults extracts unique messages across all supplied faults
// and their nested NoCompatibleHost errors, preserving first-seen order after
// trimming trailing colons and whitespace. It returns nil when no messages exist.
func LocalizedMessagesFromFaults(faults []vimtypes.LocalizedMethodFault) []string {
	var allMessages []string //nolint:prealloc // per-fault message count unknown; nil result preserved
	seen := make(map[string]struct{})
	for _, f := range faults {
		for _, message := range localizedMessagesFromFault(f) {
			if _, ok := seen[message]; ok {
				continue
			}
			seen[message] = struct{}{}
			allMessages = append(allMessages, message)
		}
	}
	return allMessages
}
