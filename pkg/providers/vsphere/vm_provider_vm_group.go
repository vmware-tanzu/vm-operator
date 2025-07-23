// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
)

func (*vSphereVMProvider) PlaceVirtualMachineGroup(
	ctx context.Context,
	group *vmopv1.VirtualMachineGroup,
	groupPlacements []providers.VMGroupPlacement) error {

	_ = ctx
	_ = group
	_ = groupPlacements

	return nil
}
