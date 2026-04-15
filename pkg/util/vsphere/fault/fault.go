package fault

import (
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// LocalizedMessagesFromFault extracts all localized messages from a LocalizedMethodFault,
// recursively traversing nested faults.
//
// This function handles the special case of NoCompatibleHost hierarchies by:
//   - Extracting the top-level LocalizedMessage only
//   - Recursively unwrapping Error field chains (e.g., NoCompatibleHost.Error)
//
// Returns an empty slice if both the LocalizedMessage and Fault are nil/empty.
func LocalizedMessagesFromFault(lmf vimtypes.LocalizedMethodFault) []string {
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

	// Only extract messages from the top level NoCompatibleHost fault.
	// Placement APIs return faults from single host per ResourcePool, we won't end up with too many errors.
	if nch, ok := lmf.Fault.(*vimtypes.NoCompatibleHost); ok {
		for _, nestedLmf := range nch.Error {
			messages = append(messages, LocalizedMessagesFromFault(nestedLmf)...)
		}
	}

	return messages
}

// LocalizedMessagesFromFaults extracts all localized messages from a slice of LocalizedMethodFault.
// Returns an empty slice if the input faults slice is empty.
func LocalizedMessagesFromFaults(faults []vimtypes.LocalizedMethodFault) []string {
	var allMessages []string
	for _, f := range faults {
		allMessages = append(allMessages, LocalizedMessagesFromFault(f)...)
	}
	return allMessages
}
