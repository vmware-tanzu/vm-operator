// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"fmt"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// TargetID is the controller:bus:slot target ID at which a device is located.
type TargetID struct {
	ControllerType vmopv1.VirtualControllerType
	ControllerBus  int32
	UnitNumber     int32
}

// String returns the stringified target ID, i.e.:
//
//	CONTROLLER_TYPE:CONTROLLER_BUS_NUMBER:UNIT_NUMBER.
func (t TargetID) String() string {
	return fmt.Sprintf("%s:%d:%d",
		t.ControllerType, t.ControllerBus, t.UnitNumber)
}

// HasTargetID is a device connected to a controller and thus has a target ID.
type HasTargetID interface {
	GetControllerType() vmopv1.VirtualControllerType
	GetControllerBusNumber() *int32
	GetUnitNumber() *int32
}

// GetTargetID returns the object's target ID, i.e.:
//
//	CONTROLLER_TYPE:CONTROLLER_BUS_NUMBER:UNIT_NUMBER
//
// If the controller type is empty, controller bus number is unset, or the
// unit number is unset, this function returns an empty string.
func GetTargetID(obj HasTargetID) string {
	var (
		ct = obj.GetControllerType()
		cb = obj.GetControllerBusNumber()
		un = obj.GetUnitNumber()
	)
	if ct != "" && cb != nil && un != nil {
		return fmt.Sprintf("%s:%d:%d", ct, *cb, *un)
	}
	return ""
}

// FindByTargetID returns the object with the matching target ID.
func FindByTargetID[T HasTargetID](
	controllerType vmopv1.VirtualControllerType,
	controllerBusNumber, unitNumber int32,
	hasTargetIDs ...T) *T {

	tid := TargetID{
		ControllerType: controllerType,
		ControllerBus:  controllerBusNumber,
		UnitNumber:     unitNumber,
	}.String()

	for i, v := range hasTargetIDs {
		if GetTargetID(v) == tid {
			return &hasTargetIDs[i]
		}
	}

	return nil
}
