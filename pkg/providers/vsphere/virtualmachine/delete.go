// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/paused"
	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
)

func DeleteVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine) error {

	if err := vcVM.Properties(
		vmCtx,
		vcVM.Reference(),
		[]string{
			"config.extraConfig",
			"summary.runtime.connectionState",
		}, &vmCtx.MoVM); err != nil {

		return fmt.Errorf("failed to fetch props when deleting VM: %w", err)
	}

	// Get the VM home dir's datacenter and datastore IDs.
	var (
		vmHomeDatacenterID string
		vmHomeDatastoreID  string
	)
	ec := object.OptionValueList(vmCtx.MoVM.Config.ExtraConfig)
	if v, _ := ec.GetString(pkgconst.VMHomeDatacenterAndDatastoreIDExtraConfigKey); v != "" {
		p := strings.Split(v, ",")
		if len(p) != 2 {
			vmCtx.Logger.Info("Unexpected value in extraConfig",
				"key", pkgconst.VMHomeDatacenterAndDatastoreIDExtraConfigKey,
				"value", v)
		} else {
			vmHomeDatacenterID = p[0]
			vmHomeDatastoreID = p[1]
		}
	}

	// Only process connected VMs or if the connection state is empty.
	if cs := vmCtx.MoVM.Summary.Runtime.ConnectionState; cs != "" && cs !=
		vimtypes.VirtualMachineConnectionStateConnected {

		// Return a NoRequeueError so the VM is not requeued for
		// reconciliation.
		//
		// The watcher service ensures that VMs will be reconciled
		// immediately upon their summary.runtime.connectionState value
		// changing.
		//
		// TODO(akutz) Determine if we should surface some type of condition
		//             that indicates this state.

		return fmt.Errorf("failed to delete vm: %w", pkgerr.NoRequeueError{
			Message: fmt.Sprintf("unsupported connection state: %s", cs),
		})
	}

	// Throw an error to distinguish from successful deletion.
	if paused := paused.ByAdmin(vmCtx.MoVM); paused {
		if vmCtx.VM.Labels == nil {
			vmCtx.VM.Labels = make(map[string]string)
		}
		vmCtx.VM.Labels[vmopv1.PausedVMLabelKey] = "admin"

		return fmt.Errorf("failed to delete vm: %w", pkgerr.NoRequeueError{
			Message: constants.VMPausedByAdminError,
		})
	}

	if _, err := vmutil.SetAndWaitOnPowerState(
		logr.NewContext(vmCtx, vmCtx.Logger),
		vcVM.Client(),
		vmutil.ManagedObjectFromObject(vcVM),
		false,
		vimtypes.VirtualMachinePowerStatePoweredOff,
		vmutil.ParsePowerOpMode(string(vmCtx.VM.Spec.PowerOffMode))); err != nil {

		return fmt.Errorf("powering off vm failed: %w", err)
	}

	t, err := vcVM.Destroy(vmCtx)
	if err != nil {
		return fmt.Errorf("calling destroy VM failed: %w", err)
	}

	if _, err := t.WaitForResult(vmCtx); err != nil {
		return fmt.Errorf("destroy VM task failed: %w", err)
	}

	// If the VM has a vmDir annotation and that path still exists then delete
	// that location as well.
	if vmHomeDatacenterID != "" && vmHomeDatastoreID != "" {
		vmDir := fmt.Sprintf("[%s] %s", vmHomeDatastoreID, vmCtx.VM.UID)
		vmCtx.Logger.Info("Deleting vmDir", "vmDir", vmDir)

		vimClient := vcVM.Client()

		datacenter := object.NewDatacenter(
			vimClient,
			vimtypes.ManagedObjectReference{
				Type:  string(vimtypes.ManagedObjectTypesDatacenter),
				Value: vmHomeDatacenterID,
			})

		fm := object.NewFileManager(vimClient)
		task, err := fm.DeleteDatastoreFile(vmCtx, vmDir, datacenter)
		if err != nil {
			return fmt.Errorf("failed to call delete datastore file: %w", err)
		}

		if err := task.Wait(vmCtx); err != nil {
			if fault.Is(err, &vimtypes.FileNotFound{}) {
				vmCtx.Logger.Info("vmDir is already deleted",
					"vmDir", vmDir)
				return nil
			}
			return fmt.Errorf("failed to delete datastore file: %w", err)
		}

		vmCtx.Logger.V(4).Info("Deleted vmDir", "vmDir", vmDir)
	}

	return nil
}
