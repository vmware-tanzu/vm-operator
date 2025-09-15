// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"strings"

	"github.com/vmware/govmomi/object"
)

func GetExtraConfigGuestInfo(
	ctx context.Context,
	vm *object.VirtualMachine) (map[string]string, error) {
	extraConfig, err := GetExtraConfigFromObject(ctx, vm)
	if err != nil {
		return nil, err
	}

	giMap := make(map[string]string)

	for _, option := range extraConfig {
		if val := option.GetOptionValue(); val != nil {
			if strings.HasPrefix(val.Key, "guestinfo.") {
				if str, ok := val.Value.(string); ok {
					giMap[val.Key] = str
				}
			}
		}
	}

	return giMap, nil
}
