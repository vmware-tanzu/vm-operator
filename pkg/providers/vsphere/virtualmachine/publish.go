// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"fmt"

	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/vcenter"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

const (
	sourceVirtualMachineType = "VirtualMachine"

	itemDescriptionFormat = "virtualmachinepublishrequest.vmoperator.vmware.com: %s\n"
)

func CreateOVF(
	vmCtx pkgctx.VirtualMachineContext,
	client *rest.Client,
	vmPubReq *vmopv1.VirtualMachinePublishRequest,
	cl *imgregv1a1.ContentLibrary,
	actID string) (string, error) {

	// Use VM Operator specific description so that we can link published items
	// to the vmPub if anything unexpected happened.
	descriptionPrefix := fmt.Sprintf(itemDescriptionFormat, string(vmPubReq.UID))
	createSpec := vcenter.CreateSpec{
		Name:        vmPubReq.Status.TargetRef.Item.Name,
		Description: descriptionPrefix + vmPubReq.Status.TargetRef.Item.Description,
	}

	source := vcenter.ResourceID{
		Type:  sourceVirtualMachineType,
		Value: vmCtx.VM.Status.UniqueID,
	}

	target := vcenter.LibraryTarget{
		LibraryID: string(cl.Spec.UUID),
	}

	ovf := vcenter.OVF{
		Spec:   createSpec,
		Source: source,
		Target: target,
	}

	vmCtx.Logger.Info("Creating OVF from VM", "spec", ovf, "actId", actID)

	// Use vmpublish uid as the act id passed down to the content library service, so that we can track
	// the task status by the act id.
	return vcenter.NewManager(client).CreateOVF(
		pkgutil.WithVAPIActivationID(vmCtx, client, actID),
		ovf)
}
