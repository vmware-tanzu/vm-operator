// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// OverwriteResizeConfigSpec applies any set fields in the VM Spec to the ConfigSpec.
func OverwriteResizeConfigSpec(
	_ context.Context,
	vm vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) error {

	if adv := vm.Spec.Advanced; adv != nil {
		ptr.OverwriteWithUser(&cs.ChangeTrackingEnabled, adv.ChangeBlockTracking, ci.ChangeTrackingEnabled)
	}

	return nil
}
