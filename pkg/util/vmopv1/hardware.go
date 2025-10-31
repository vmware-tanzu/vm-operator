// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

// ControllerSpec is an interface describing a controller specification.
type ControllerSpec interface {
	MaxSlots() int32
}

// ControllerHasAvailableSlots checks if a controller has available slots for
// new devices.
func ControllerHasAvailableSlots(
	controller ControllerSpec,
	busNumber int32,
	statusDeviceCounts, volumeAssignments map[int32]int32,
) bool {
	// Get current device count from status
	currentDevices := statusDeviceCounts[busNumber]

	// Get number of volumes already assigned to this controller in this mutation
	assignedVolumes := volumeAssignments[busNumber]

	// Check if adding one more volume would exceed capacity
	return (currentDevices + assignedVolumes + 1) <= controller.MaxSlots()
}
