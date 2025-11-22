// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"fmt"
	"strings"

	"github.com/vmware/govmomi/object"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// IssueReporter defines the common interface for hardware validation issues.
// This interface allows polymorphic handling of different issue types.
type IssueReporter interface {
	// HasIssues returns true if there are any issues to report.
	HasIssues() bool
	// Message formats all issues into a human-readable message.
	Message() string
}

// Compile-time checks to ensure issue types implement IssueReporter.
var (
	_ IssueReporter = (*ControllerIssues)(nil)
	_ IssueReporter = (*VolumeIssues)(nil)
	_ IssueReporter = (*CDROMIssues)(nil)
	_ IssueReporter = (*HardwareConfigIssues)(nil)
)

// ControllerIssues stores controller configuration issues found during verification.
// The missing and unexpected controllers are sorted.
type ControllerIssues struct {
	Missing    []pkgutil.ControllerID
	Unexpected []pkgutil.ControllerID
}

// HasIssues returns true if there are any issues to report.
func (c *ControllerIssues) HasIssues() bool {
	return len(c.Missing) > 0 || len(c.Unexpected) > 0
}

// Message formats all issues into a human-readable message.
// Each issue type is on a separate line, and items are formatted as
// comma-separated lists for better readability.
func (c *ControllerIssues) Message() string {
	return formatIssues(
		formatList(c.Missing, "missing controllers"),
		formatList(c.Unexpected, "unexpected controllers"),
	)
}

// VolumeIssues stores volume configuration issues found during verification.
// The missing and unexpected volumes are sorted. The incomplete placement
// volumes are appended in order when looping through the spec list.
type VolumeIssues struct {
	Missing             []pkgutil.DevicePlacement
	Unexpected          []pkgutil.DevicePlacement
	IncompletePlacement []string
}

// HasIssues returns true if there are any issues to report.
func (v *VolumeIssues) HasIssues() bool {
	return len(v.Missing) > 0 ||
		len(v.Unexpected) > 0 ||
		len(v.IncompletePlacement) > 0
}

// Message formats all issues into a human-readable message.
// Each issue type is on a separate line, and items are formatted as
// comma-separated lists for better readability.
func (v *VolumeIssues) Message() string {
	return formatIssues(
		formatList(v.Missing, "missing volumes"),
		formatList(v.Unexpected, "unexpected volumes"),
		formatStrings(v.IncompletePlacement, "volumes with incomplete placement"),
	)
}

// CDROMIssues stores CD-ROM device configuration issues found during verification.
// The missing and unexpected CD-ROM devices are sorted. The incomplete placement
// CD-ROMs and CD-ROM failed resolution are appended in order when looping through
// the spec list.
type CDROMIssues struct {
	Missing             []pkgutil.DevicePlacement
	Unexpected          []pkgutil.DevicePlacement
	IncompletePlacement []string
	FailedResolution    []string
}

// HasIssues returns true if there are any issues to report.
func (c *CDROMIssues) HasIssues() bool {
	return len(c.Missing) > 0 ||
		len(c.Unexpected) > 0 ||
		len(c.IncompletePlacement) > 0 ||
		len(c.FailedResolution) > 0
}

// Message formats all issues into a human-readable message.
// Each issue type is on a separate line, and items are formatted as
// comma-separated lists for better readability.
func (c *CDROMIssues) Message() string {
	return formatIssues(
		formatList(c.Missing, "missing CD-ROM devices"),
		formatList(c.Unexpected, "unexpected CD-ROM devices"),
		formatStrings(c.IncompletePlacement, "CD-ROM devices with incomplete placement"),
		formatStrings(c.FailedResolution, "CD-ROM devices with failed resolution"),
	)
}

// HardwareConfigIssues stores all hardware device configuration issues
// found during verification. This is kept for backward compatibility
// and can be used to aggregate all device-specific issues.
type HardwareConfigIssues struct {
	ControllerIssues ControllerIssues
	VolumeIssues     VolumeIssues
	CDROMIssues      CDROMIssues
}

// HasIssues returns true if there are any issues to report.
func (h *HardwareConfigIssues) HasIssues() bool {
	return h.ControllerIssues.HasIssues() ||
		h.VolumeIssues.HasIssues() ||
		h.CDROMIssues.HasIssues()
}

// Message formats all issues into a concise summary message.
// Following Kubernetes best practices, this provides a concise summary
// rather than duplicating all detailed messages. The message references
// the specific condition types where detailed information is available.
// Example output: "Hardware configuration issues detected. See VirtualMachineHardwareControllersVerified, VirtualMachineHardwareVolumesVerified conditions for details.".
func (h *HardwareConfigIssues) Message() string {
	var conditionTypes []string

	if h.ControllerIssues.HasIssues() {
		conditionTypes = append(conditionTypes, vmopv1.VirtualMachineHardwareControllersVerified)
	}
	if h.VolumeIssues.HasIssues() {
		conditionTypes = append(conditionTypes, vmopv1.VirtualMachineHardwareVolumesVerified)
	}
	if h.CDROMIssues.HasIssues() {
		conditionTypes = append(conditionTypes, vmopv1.VirtualMachineHardwareCDROMVerified)
	}

	if len(conditionTypes) == 0 {
		return ""
	}

	return fmt.Sprintf("Hardware configuration issues detected. See %s conditions for details.",
		strings.Join(conditionTypes, ", "))
}

// formatIssues combines multiple message strings, filtering out empty ones.
// Non-empty messages are joined with newlines.
func formatIssues(messages ...string) string {
	var parts []string
	for _, msg := range messages {
		if msg != "" {
			parts = append(parts, msg)
		}
	}
	return strings.Join(parts, "\n")
}

// formatList formats a slice of items that implement fmt.Stringer into a message.
// Returns an empty string if the slice is empty.
func formatList[T fmt.Stringer](items []T, label string) string {
	if len(items) == 0 {
		return ""
	}
	itemStrs := make([]string, 0, len(items))
	for _, item := range items {
		itemStrs = append(itemStrs, item.String())
	}
	return fmt.Sprintf("%s: %s", label, strings.Join(itemStrs, ", "))
}

// formatStrings formats a slice of strings into a message.
// Returns an empty string if the slice is empty.
func formatStrings(items []string, label string) string {
	if len(items) == 0 {
		return ""
	}
	return fmt.Sprintf("%s: %s", label, strings.Join(items, ", "))
}

// reconcileHardwareCondition updates the hardware device configuration conditions
// by verifying that the VM's hardware device configuration matches the desired
// state specified in the spec. It sets individual conditions for controllers,
// volumes, and CD-ROMs, then aggregates them into VirtualMachineHardwareDeviceConfigVerified.
// The aggregated condition provides a concise summary rather than duplicating
// all condition messages. Detailed information is available in the individual
// device-specific conditions.
func reconcileHardwareCondition(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	var (
		issues = HardwareConfigIssues{}
		hwInfo = pkgutil.BuildHardwareInfo(vmCtx.MoVM)
	)

	checkControllers(vmCtx.VM, hwInfo, &issues.ControllerIssues)
	checkVolumes(vmCtx.VM, hwInfo, &issues.VolumeIssues)
	checkCDROMDevices(vmCtx, k8sClient, hwInfo, &issues.CDROMIssues)

	if issues.HasIssues() {
		conditions.MarkFalse(
			vmCtx.VM,
			vmopv1.VirtualMachineHardwareDeviceConfigVerified,
			vmopv1.VirtualMachineHardwareDeviceConfigMismatchReason,
			"%s",
			issues.Message())
		return nil
	}

	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineHardwareDeviceConfigVerified)
	return nil
}

// checkControllers verifies that all controllers specified in the VM spec are
// attached and no extra controllers exist. It sets the
// VirtualMachineHardwareControllersVerified condition.
func checkControllers(
	vm *vmopv1.VirtualMachine,
	hwInfo pkgutil.HardwareInfo,
	issues *ControllerIssues) {

	var (
		expected = sets.New[pkgutil.ControllerID]()
		actual   = hwInfo.Controllers
	)

	if hw := vm.Spec.Hardware; hw != nil {
		for _, ctrl := range hw.IDEControllers {
			expected.Insert(pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeIDE,
				BusNumber:      ctrl.BusNumber,
			})
		}
		for _, ctrl := range hw.NVMEControllers {
			expected.Insert(pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeNVME,
				BusNumber:      ctrl.BusNumber,
			})
		}
		for _, ctrl := range hw.SATAControllers {
			expected.Insert(pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeSATA,
				BusNumber:      ctrl.BusNumber,
			})
		}
		for _, ctrl := range hw.SCSIControllers {
			expected.Insert(pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeSCSI,
				BusNumber:      ctrl.BusNumber,
			})
		}
	}

	issues.Missing, issues.Unexpected = pkgutil.DiffSets(expected, actual)

	if issues.HasIssues() {
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineHardwareControllersVerified,
			vmopv1.VirtualMachineHardwareControllersMismatchReason,
			"%s",
			issues.Message())
		return
	}

	conditions.MarkTrue(vm, vmopv1.VirtualMachineHardwareControllersVerified)
}

// checkVolumes verifies that all volumes in the spec are attached and
// no extra disks are attached that aren't in the spec. It sets the
// VirtualMachineHardwareVolumesVerified condition.
func checkVolumes(
	vm *vmopv1.VirtualMachine,
	hwInfo pkgutil.HardwareInfo,
	issues *VolumeIssues) {

	var (
		expected = sets.New[pkgutil.DevicePlacement]()
		actual   = sets.New[pkgutil.DevicePlacement]()
	)

	for _, vol := range vm.Spec.Volumes {
		if vol.ControllerType == "" ||
			vol.ControllerBusNumber == nil ||
			vol.UnitNumber == nil {
			issues.IncompletePlacement = append(issues.IncompletePlacement, vol.Name)
			continue
		}

		expected.Insert(pkgutil.DevicePlacement{
			Key:                 vol.Name,
			ControllerType:      vol.ControllerType,
			ControllerBusNumber: *vol.ControllerBusNumber,
			UnitNumber:          *vol.UnitNumber,
		})
	}

	// Build mapping from disk UUID to volume name for attached PVC volumes.
	// vm.Status.Volumes is reconciled by the volume batch controller, which
	// runs as a separate reconciler from this one. This creates a potential
	// race condition where vm.Status.Volumes may not be up-to-date when this
	// check runs. We rely on eventual consistency here: the volume batch
	// reconciler will eventually update vm.Status.Volumes, and this check
	// will be correct on subsequent reconciliation cycles. The volume batch
	// reconciler will eventually be moved into the same reconciler to
	// eliminate this race condition.
	volNameByDiskUUID := make(map[string]string)
	for _, volStatus := range vm.Status.Volumes {
		// Ignore Attached status here since we will be checking again the
		// hardware status directly.
		if volStatus.DiskUUID != "" {
			volNameByDiskUUID[volStatus.DiskUUID] = volStatus.Name
		}
	}

	// Build actual disks from hardware info.
	// hwInfo.Disks uses disk UUID as the key (from BuildHardwareInfo).
	// We need to map disk UUID to volume name to match against expected placements.
	// The placementKey in DevicePlacement should be the volume name (from spec),
	// not the disk UUID.
	for _, diskPlacement := range hwInfo.Disks.UnsortedList() {
		// Look up volume name by disk UUID
		// diskPlacement.Key is the disk UUID (from BuildHardwareInfo)
		if volName, found := volNameByDiskUUID[diskPlacement.Key]; found {
			actual.Insert(pkgutil.DevicePlacement{
				Key:                 volName, // Use volume name as the key
				ControllerType:      diskPlacement.ControllerType,
				ControllerBusNumber: diskPlacement.ControllerBusNumber,
				UnitNumber:          diskPlacement.UnitNumber,
			})
		}
	}

	issues.Missing, issues.Unexpected = pkgutil.DiffSets(expected, actual)

	if issues.HasIssues() {
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineHardwareVolumesVerified,
			vmopv1.VirtualMachineHardwareVolumesMismatchReason,
			"%s",
			issues.Message())
		return
	}

	conditions.MarkTrue(vm, vmopv1.VirtualMachineHardwareVolumesVerified)
}

// checkCDROMDevices verifies that all CD-ROM devices in the spec have
// matching virtual devices with the same backing file name, and no extra
// CD-ROM devices exist that aren't in the spec. It sets the
// VirtualMachineHardwareCDROMVerified condition.
func checkCDROMDevices(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	hwInfo pkgutil.HardwareInfo,
	issues *CDROMIssues) {

	var (
		vm       = vmCtx.VM
		expected = sets.New[pkgutil.DevicePlacement]()
		actual   = hwInfo.CDROMs
	)

	if vm.Spec.Hardware != nil {
		for _, cdromSpec := range vm.Spec.Hardware.Cdrom {
			if cdromSpec.ControllerType == "" ||
				cdromSpec.ControllerBusNumber == nil ||
				cdromSpec.UnitNumber == nil {
				issues.IncompletePlacement = append(issues.IncompletePlacement, cdromSpec.Name)
				continue
			}

			backingFileName, err := virtualmachine.GetBackingFileNameByImageRef(
				vmCtx, k8sClient, cdromSpec.Image, vm.Namespace, false, nil)
			if err != nil {
				vmCtx.Logger.Error(err, "failed to resolve backing file name for CD-ROM device",
					"cdromName", cdromSpec.Name, "image", cdromSpec.Image)
				issues.FailedResolution = append(issues.FailedResolution, cdromSpec.Name)
				continue
			}

			expected.Insert(pkgutil.DevicePlacement{
				Key:                 backingFileName,
				ControllerType:      cdromSpec.ControllerType,
				ControllerBusNumber: *cdromSpec.ControllerBusNumber,
				UnitNumber:          *cdromSpec.UnitNumber,
			})
		}
	}

	issues.Missing, issues.Unexpected = pkgutil.DiffSets(expected, actual)

	if issues.HasIssues() {
		conditions.MarkFalse(
			vmCtx.VM,
			vmopv1.VirtualMachineHardwareCDROMVerified,
			vmopv1.VirtualMachineHardwareCDROMMismatchReason,
			"%s",
			issues.Message())
		return
	}

	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineHardwareCDROMVerified)
}
