// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha4-virtualmachinegroup,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinegroups,verbs=create;update,versions=v1alpha4,name=default.mutating.virtualmachinegroup.v1alpha4.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups,verbs=get;list;update

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewMutatingWebhook(ctx, mgr, webHookName, NewMutator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create mutation webhook: %w", err)
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewMutator returns the package's Mutator.
func NewMutator(client ctrlclient.Client) builder.Mutator {
	return mutator{
		client:    client,
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type mutator struct {
	client    ctrlclient.Client
	converter runtime.UnstructuredConverter
}

func (m mutator) Mutate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	if ctx.Op == admissionv1.Delete {
		return admission.Allowed("")
	}

	modified, err := m.vmGroupFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var wasMutated bool

	switch ctx.Op {
	case admissionv1.Create:
		ok, err := ProcessPowerState(ctx, modified, nil)
		if err != nil {
			return admission.Denied(err.Error())
		}
		if ok {
			wasMutated = true
		}
	case admissionv1.Update:
		oldVMGroup, err := m.vmGroupFromUnstructured(ctx.OldObj)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		ok, err := ProcessPowerState(ctx, modified, oldVMGroup)
		if err != nil {
			return admission.Denied(err.Error())
		}
		if ok {
			wasMutated = true
		}
	}

	if !wasMutated {
		return admission.Allowed("")
	}

	rawModified, err := json.Marshal(modified)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(ctx.RawObj, rawModified)
}

func (m mutator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineGroup{}).Name())
}

func (m mutator) vmGroupFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineGroup, error) {
	vmGroup := &vmopv1.VirtualMachineGroup{}
	if err := m.converter.FromUnstructured(obj.UnstructuredContent(), vmGroup); err != nil {
		return nil, err
	}
	return vmGroup, nil
}

// ProcessPowerState updates the last updated power state time annotation to the
// current UTC time in RFC3339Nano format for the following cases:
// 1. The group is created with a non-empty spec.powerState value.
// 2. The spec.powerState is updated.
// 3. The spec.nextForcePowerStateSyncTime is set to "now".
// It also removes the apply power state change time annotation if the group is
// being updated directly (not inherited from a parent).
func ProcessPowerState(
	ctx *pkgctx.WebhookRequestContext,
	newVMG, oldVMG *vmopv1.VirtualMachineGroup) (bool, error) {
	// If the old VMGroup is nil, this is a creation request.
	if oldVMG == nil {
		oldVMG = &vmopv1.VirtualMachineGroup{
			Spec: vmopv1.VirtualMachineGroupSpec{
				PowerState:                  "",
				NextForcePowerStateSyncTime: "",
			},
		}
	}

	var (
		wasMutated             bool
		shouldForceSync        bool
		shouldUpdateAnnotation bool
		oldForceSyncVal        = oldVMG.Spec.NextForcePowerStateSyncTime
		newForceSyncVal        = newVMG.Spec.NextForcePowerStateSyncTime
		now                    = time.Now().UTC().Format(time.RFC3339Nano)
	)

	switch {
	case newForceSyncVal == "":
		// Field is either not set or deleted, reset it to the previous value.
		newVMG.Spec.NextForcePowerStateSyncTime = oldForceSyncVal
		wasMutated = oldForceSyncVal != ""
	case strings.EqualFold("now", newForceSyncVal):
		newVMG.Spec.NextForcePowerStateSyncTime = now
		wasMutated = true
		shouldForceSync = true
	case newForceSyncVal != oldForceSyncVal:
		return false, field.Invalid(
			field.NewPath("spec", "nextForcePowerStateSyncTime"),
			newVMG.Spec.NextForcePowerStateSyncTime,
			`may only be set to "now"`)
	}

	shouldUpdateAnnotation = shouldForceSync ||
		newVMG.Spec.PowerState != oldVMG.Spec.PowerState

	if shouldUpdateAnnotation {
		// Set the last updated power state time annotation.
		if newVMG.Annotations == nil {
			newVMG.Annotations = make(map[string]string)
		}
		newVMG.Annotations[constants.LastUpdatedPowerStateTimeAnnotation] = now
		wasMutated = true
	}

	// Check if we should remove the apply-power-state time annotation.
	var (
		oldAnnoTime = oldVMG.Annotations[constants.ApplyPowerStateTimeAnnotation]
		newAnnoTime = newVMG.Annotations[constants.ApplyPowerStateTimeAnnotation]
	)

	if oldAnnoTime != newAnnoTime {
		// VM Group is being updated from its parent group, no op.
		return wasMutated, nil
	}

	if shouldUpdateAnnotation {
		// VM Group's power state is being updated directly, remove the stale
		// annotation to apply the power state immediately.
		if newAnnoTime != "" {
			delete(newVMG.Annotations, constants.ApplyPowerStateTimeAnnotation)
			wasMutated = true
		}
	}

	return wasMutated, nil
}
