// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha2-virtualmachine,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,verbs=create;update,versions=v1alpha2,name=default.mutating.virtualmachine.v1alpha2.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachine,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachine/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewMutatingWebhook(ctx, mgr, webHookName, NewMutator(mgr.GetClient()))
	if err != nil {
		return errors.Wrapf(err, "failed to create mutation webhook")
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewMutator returns the package's Mutator.
func NewMutator(client client.Client) builder.Mutator {
	return mutator{
		client:    client,
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type mutator struct {
	client    client.Client
	converter runtime.UnstructuredConverter
}

func (m mutator) Mutate(ctx *context.WebhookRequestContext) admission.Response {
	if ctx.Op == admissionv1.Delete {
		return admission.Allowed("")
	}

	vm, err := m.vmFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var wasMutated bool
	original := vm
	modified := original.DeepCopy()

	//nolint:gocritic
	switch ctx.Op {
	case admissionv1.Update:
		oldVM, err := m.vmFromUnstructured(ctx.OldObj)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		if ok, err := SetNextRestartTime(ctx, modified, oldVM); err != nil {
			return admission.Denied(err.Error())
		} else if ok {
			wasMutated = true
		}
	}

	if !wasMutated {
		return admission.Allowed("")
	}

	rawOriginal, err := json.Marshal(original)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	rawModified, err := json.Marshal(modified)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(rawOriginal, rawModified)
}

func (m mutator) For() schema.GroupVersionKind {
	return vmopv1.SchemeGroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachine{}).Name())
}

// vmFromUnstructured returns the VirtualMachine from the unstructured object.
func (m mutator) vmFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachine, error) {
	vm := &vmopv1.VirtualMachine{}
	if err := m.converter.FromUnstructured(obj.UnstructuredContent(), vm); err != nil {
		return nil, err
	}
	return vm, nil
}

// SetNextRestartTime sets spec.nextRestartTime for a VM if the field's
// current value is equal to "now" (case-insensitive).
// Return true if set, otherwise false.
func SetNextRestartTime(
	ctx *context.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine) (bool, error) {

	if newVM.Spec.NextRestartTime == "" {
		newVM.Spec.NextRestartTime = oldVM.Spec.NextRestartTime
		return oldVM.Spec.NextRestartTime != "", nil
	}
	if strings.EqualFold("now", newVM.Spec.NextRestartTime) {
		if oldVM.Spec.PowerState != vmopv1.VirtualMachinePowerStateOn {
			return false, field.Invalid(
				field.NewPath("spec", "nextRestartTime"),
				newVM.Spec.NextRestartTime,
				"can only restart powered on vm")
		}
		newVM.Spec.NextRestartTime = time.Now().UTC().Format(time.RFC3339Nano)
		return true, nil
	}
	if newVM.Spec.NextRestartTime == oldVM.Spec.NextRestartTime {
		return false, nil
	}
	return false, field.Invalid(
		field.NewPath("spec", "nextRestartTime"),
		newVM.Spec.NextRestartTime,
		`may only be set to "now"`)
}
