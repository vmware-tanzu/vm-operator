// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"strings"

	"github.com/vmware/govmomi/vim25/mo"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
)

func IsPausedByAdmin(moVM mo.VirtualMachine) bool {
	for i := range moVM.Config.ExtraConfig {
		if o := moVM.Config.ExtraConfig[i].GetOptionValue(); o != nil {
			if o.Key == vmopv1.PauseVMExtraConfigKey {
				if value, ok := o.Value.(string); ok {
					return strings.ToUpper(value) == constants.ExtraConfigTrue
				}
			}
		}
	}
	return false
}
