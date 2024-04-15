// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
)

func GetExtraConfigGuestInfo(
	ctx context.Context,
	vm *object.VirtualMachine) (map[string]string, error) {

	var o mo.VirtualMachine
	if err := vm.Properties(ctx, vm.Reference(), []string{"config.extraConfig"}, &o); err != nil {
		return nil, err
	}

	if o.Config == nil {
		return nil, nil
	}

	giMap := make(map[string]string)

	for _, option := range o.Config.ExtraConfig {
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
