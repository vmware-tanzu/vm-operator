// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"strconv"
	"strings"
)

// GuestInfoBootstrapCondition is the ExtraConfig key at which possible info
// about the bootstrap status may be stored.
const GuestInfoBootstrapCondition = "guestinfo.vmservice.bootstrap.condition"

// GetBootstrapConditionValues returns the bootstrap condition values from a
// VM if the data is present.
func GetBootstrapConditionValues(
	extraConfig map[string]string) (bool, string, string, bool) {

	val, ok := extraConfig[GuestInfoBootstrapCondition]
	if !ok {
		return false, "", "", false
	}

	v := strings.SplitN(val, ",", 3)
	status, _ := strconv.ParseBool(strings.TrimSpace(v[0]))
	switch len(v) {
	case 1:
		return status, "", "", true
	case 2:
		return status, strings.TrimSpace(v[1]), "", true
	case 3:
		return status,
			strings.TrimSpace(v[1]),
			strings.TrimSpace(v[2]),
			true
	}

	return false, "", "", false
}
