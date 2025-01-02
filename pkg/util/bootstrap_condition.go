// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"strconv"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// GuestInfoBootstrapCondition is the ExtraConfig key at which possible info
// about the bootstrap status may be stored.
const GuestInfoBootstrapCondition = "guestinfo.vmservice.bootstrap.condition"

// GetBootstrapConditionValues returns the bootstrap condition values from a
// VM if the data is present.
func GetBootstrapConditionValues(
	configInfo *vimtypes.VirtualMachineConfigInfo) (bool, string, string, bool) {

	if configInfo == nil {
		return false, "", "", false
	}

	if len(configInfo.ExtraConfig) == 0 {
		return false, "", "", false
	}

	for i := range configInfo.ExtraConfig {
		if ec := configInfo.ExtraConfig[i]; ec != nil {
			if ov := ec.GetOptionValue(); ov != nil {
				if ov.Key == GuestInfoBootstrapCondition {
					if s, ok := ov.Value.(string); ok {
						v := strings.SplitN(s, ",", 3)
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
						default:
							return false, "", "", false
						}
					}
				}
			}
		}
	}

	return false, "", "", false
}
