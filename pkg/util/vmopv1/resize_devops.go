// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// OverrideResizeConfigSpec applies any set fields in the VM Spec to the ConfigSpec.
func OverrideResizeConfigSpec(
	_ context.Context,
	vm vmopv1.VirtualMachine,
	cs *vimtypes.VirtualMachineConfigSpec) error {

	if adv := vm.Spec.Advanced; adv != nil {
		ptr.Overwrite(&cs.ChangeTrackingEnabled, adv.ChangeBlockTracking)
	}

	return nil
}
