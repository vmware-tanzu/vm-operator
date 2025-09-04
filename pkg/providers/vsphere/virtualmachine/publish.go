// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/vcenter"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	imgregv1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha2"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
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
		Flags:       []string{"EXTRA_CONFIG"}, // Preserve ExtraConfig
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

// CloneVM clones a source virtual machine, as a VM template, into an inventory-backed
// content library as defined by the VirtualMachinePublishRequest object.
func CloneVM(
	vmCtx pkgctx.VirtualMachineContext,
	vimClient *vim25.Client,
	vmPubReq *vmopv1.VirtualMachinePublishRequest,
	cl *imgregv1.ContentLibrary,
	storagePolicyID, actID string) (string, error) {

	targetName := vmPubReq.Status.TargetRef.Item.Name

	folderRef := vimtypes.ManagedObjectReference{
		Type:  string(vimtypes.ManagedObjectTypesFolder),
		Value: cl.Spec.ID,
	}

	cloneSpec := vimtypes.VirtualMachineCloneSpec{
		Location: vimtypes.VirtualMachineRelocateSpec{
			Folder: &folderRef,
			// Causes a linked clone to be unlinked and consolidated
			DiskMoveType: string(vimtypes.VirtualMachineRelocateDiskMoveOptionsMoveAllDiskBackingsAndDisallowSharing),
			Profile: []vimtypes.BaseVirtualMachineProfileSpec{
				&vimtypes.VirtualMachineDefinedProfileSpec{ProfileId: storagePolicyID},
			},
		},
		Template: true,
		Config: &vimtypes.VirtualMachineConfigSpec{
			Annotation: vmPubReq.Status.TargetRef.Item.Description,
			ExtraConfig: []vimtypes.BaseOptionValue{
				&vimtypes.OptionValue{
					Key:   vmopv1.VirtualMachinePublishRequestUUIDLabelKey,
					Value: string(vmPubReq.UID),
				},
			},
		},
	}

	vmCtx.Logger.Info("Publishing VM as template",
		"actId", actID,
		"targetName", targetName,
		"cloneSpec", cloneSpec,
		"cloneSource", vmCtx.VM.Status.UniqueID,
		"cloneTarget", cl.Spec.ID)

	vm := object.NewVirtualMachine(
		vimClient,
		vimtypes.ManagedObjectReference{
			Type:  string(vimtypes.ManagedObjectTypesVirtualMachine),
			Value: vmCtx.VM.Status.UniqueID,
		})

	cloneTask, err := vm.Clone(
		vmCtx,
		object.NewFolder(vimClient, folderRef),
		targetName,
		cloneSpec)

	if err != nil {
		return "", fmt.Errorf("failed to call clone api: %w", err)
	}

	cloneTaskInfo, err := cloneTask.WaitForResult(vmCtx)
	if err != nil {
		return "", fmt.Errorf("failed to clone VM to template: %w", err)
	}
	vmCtx.Logger.V(4).Info("cloned VM", "taskInfo", cloneTaskInfo)

	cloneMoRef := cloneTaskInfo.Result.(vimtypes.ManagedObjectReference)

	return cloneMoRef.Value, nil
}
