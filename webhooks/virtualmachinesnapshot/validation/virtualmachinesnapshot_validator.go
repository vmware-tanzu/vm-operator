// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
)

const (
	errUnsupportedVMControllerSharingModeFmt = "controller type %s bus %d is using unsupported sharingMode for snapshot: %s"
	errUnsupportedVMVolumeSharingModeFmt     = "volume %s is using unsupported sharingMode for snapshot: %s"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha6-virtualmachinesnapshot,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinesnapshots,versions=v1alpha6,name=default.validating.virtualmachinesnapshot.v1alpha6.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesnapshots,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesnapshots/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create VirtualMachineSnapshot validation webhook: %w", err)
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewValidator returns the package's Validator.
func NewValidator(client client.Client) builder.Validator {
	return validator{
		client:    client,
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type validator struct {
	client    client.Client
	converter runtime.UnstructuredConverter
}

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineSnapshot{}).Name())
}

// ValidateCreate makes sure the VirtualMachineSnapshot create request is valid.
func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	vmSnapshot, err := v.vmSnapshotFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	vmNameField := field.NewPath("spec", "vmName")

	if vmSnapshot.Spec.VMName == "" {
		fieldErrs = append(fieldErrs, field.Required(vmNameField, "vmName must be provided"))
	} else {
		fieldErrs = append(fieldErrs, vmNameLabelMismatchErr(vmSnapshot)...)
		fieldErrs = append(fieldErrs,
			v.validateVMFields(
				ctx,
				vmSnapshot.Spec.VMName,
				vmSnapshot.Namespace,
				vmNameField)...,
		)
	}

	validationErrs := make([]string, 0)
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) ValidateDelete(*pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

// ValidateUpdate validates if the VirtualMachineSnapshot update is valid
// - Fields other than Description are not allowed to be changed.
func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	vmSnapshot, err := v.vmSnapshotFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVMSnapshot, err := v.vmSnapshotFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(vmSnapshot.Spec.Memory, oldVMSnapshot.Spec.Memory, field.NewPath("spec", "memory"))...)
	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(vmSnapshot.Spec.Quiesce, oldVMSnapshot.Spec.Quiesce, field.NewPath("spec", "quiesce"))...)
	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(vmSnapshot.Spec.VMName, oldVMSnapshot.Spec.VMName, field.NewPath("spec", "vmName"))...)
	fieldErrs = append(fieldErrs, vmNameLabelMismatchErr(vmSnapshot)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

// vmNameLabelMismatchErr rejects a VMNameForSnapshotLabel that disagrees
// with spec.vmName. Returns nil if the label is absent or matches. Since
// spec.vmName is itself immutable, this is sufficient to keep the label
// correct on both create and update without a separate lock-once-set check.
func vmNameLabelMismatchErr(vmSnapshot *vmopv1.VirtualMachineSnapshot) field.ErrorList {
	label, ok := vmSnapshot.Labels[vmopv1.VMNameForSnapshotLabel]
	if !ok || label == vmSnapshot.Spec.VMName {
		return nil
	}

	return field.ErrorList{
		field.Invalid(
			field.NewPath("metadata", "labels").Key(vmopv1.VMNameForSnapshotLabel),
			label,
			fmt.Sprintf("must match spec.vmName %q", vmSnapshot.Spec.VMName)),
	}
}

// validateVMNotVKSNode checks if the referenced VM is a VKS/TKG node and
// returns an error if it is.
func (v validator) validateVMNotVKSNode(
	vm vmopv1.VirtualMachine) error {

	// Check if the VM has CAPI labels indicating it's a VKS/TKG node.
	if kubeutil.HasCAPILabels(vm.Labels) {
		return fmt.Errorf("snapshots are not allowed for VKS/TKG nodes")
	}

	return nil
}

// validateVMControllerSharingMode checks if the referenced VM has controller
// with sharingMode that's not None.
func (v validator) validateVMControllerSharingMode(
	vm vmopv1.VirtualMachine,
	vmNameField *field.Path) field.ErrorList {

	var allErrs field.ErrorList

	if vm.Spec.Hardware != nil {
		for _, c := range vm.Spec.Hardware.SCSIControllers {
			if c.SharingMode != vmopv1.VirtualControllerSharingModeNone {
				allErrs = append(allErrs,
					field.NotSupported(vmNameField,
						fmt.Sprintf(errUnsupportedVMControllerSharingModeFmt,
							"SCSIControllers",
							c.BusNumber,
							c.SharingMode), []string{string(vmopv1.VirtualControllerSharingModeNone)},
					),
				)
			}
		}
	}

	return allErrs
}

// validateVMVolumeSharingMode checks if the referenced VM has a
// dependent-mode volume (Persistent/NonPersistent) with sharingMode
// MultiWriter. vSphere does not support snapshotting a VM with such a
// volume, because the snapshot's redo log can't be shared across
// concurrent writers. Independent-mode MultiWriter volumes (e.g. OracleRAC,
// which is always IndependentPersistent) are excluded from VM snapshots
// entirely, so this restriction does not apply to them.
func (v validator) validateVMVolumeSharingMode(
	vm vmopv1.VirtualMachine,
	vmNameField *field.Path) field.ErrorList {

	var allErrs field.ErrorList

	for _, vol := range vm.Spec.Volumes {
		independentDiskMode := vol.DiskMode == vmopv1.VolumeDiskModeIndependentPersistent ||
			vol.DiskMode == vmopv1.VolumeDiskModeIndependentNonPersistent

		if vol.SharingMode == vmopv1.VolumeSharingModeMultiWriter && !independentDiskMode {
			allErrs = append(allErrs,
				field.NotSupported(vmNameField,
					fmt.Sprintf(errUnsupportedVMVolumeSharingModeFmt,
						vol.Name,
						vol.SharingMode), []string{string(vmopv1.VolumeSharingModeNone)},
				),
			)
		}
	}

	return allErrs
}

func (v validator) validateVMFields(
	ctx *pkgctx.WebhookRequestContext,
	vmName, namespace string,
	vmNameField *field.Path) field.ErrorList {

	// Get the referenced VM
	vm := vmopv1.VirtualMachine{}
	vmKey := client.ObjectKey{
		Name:      vmName,
		Namespace: namespace,
	}

	var fieldErrs field.ErrorList

	if err := v.client.Get(ctx, vmKey, &vm); err != nil {
		if apierrors.IsNotFound(err) {
			// If we can't get the VM, let the snapshot creation proceed.
			// The actual snapshot process will handle missing VMs appropriately.
			ctx.Logger.V(4).Info("VM is not found for controller sharingMode validation, allowing snapshot",
				"vmName", vmName)
			return nil
		}
		fieldErrs = append(fieldErrs, field.Invalid(vmNameField, vmName, err.Error()))
	}

	// Check if the referenced VM is a VKS/TKG node and prevent snapshot.
	if err := v.validateVMNotVKSNode(vm); err != nil {
		fieldErrs = append(fieldErrs, field.Forbidden(vmNameField, err.Error()))
	}

	// Check if the reference VM has controllers with unsupported sharingMode.
	fieldErrs = append(fieldErrs,
		v.validateVMControllerSharingMode(
			vm,
			vmNameField)...,
	)

	// Check if the reference VM has volumes with unsupported sharingMode.
	fieldErrs = append(fieldErrs,
		v.validateVMVolumeSharingMode(
			vm,
			vmNameField)...,
	)

	return fieldErrs
}

// vmSnapshotFromUnstructured returns the VirtualMachineSnapshot from the unstructured object.
func (v validator) vmSnapshotFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineSnapshot, error) {
	vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmSnapshot); err != nil {
		return nil, err
	}
	return vmSnapshot, nil
}
