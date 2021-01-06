// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"encoding/json"
	"net/http"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

const (
	webHookName = "default"
)

// -kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha1-virtualmachineclass,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=create;update,versions=v1alpha1,name=default.mutating.virtualmachineclass.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1beta1,webhookVersions=v1beta1
// -kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclass,verbs=get;list
// -kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclass/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewMutatingWebhook(ctx, mgr, webHookName, NewMutator())
	if err != nil {
		return errors.Wrapf(err, "failed to create mutation webhook")
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewMutator returns the package's Mutator.
func NewMutator() builder.Mutator {
	return mutator{
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type mutator struct {
	converter runtime.UnstructuredConverter
}

//nolint
func (m mutator) Mutate(ctx *context.WebhookRequestContext) admission.Response {
	vmClass, err := m.vmClassFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var wasMutated bool
	original := vmClass
	modified := original.DeepCopy()

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
	return vmopv1.SchemeGroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineClass{}).Name())
}

// vmClassFromUnstructured returns the VirtualMachineClass from the unstructured object.
func (m mutator) vmClassFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineClass, error) {
	vmClass := &vmopv1.VirtualMachineClass{}
	if err := m.converter.FromUnstructured(obj.UnstructuredContent(), vmClass); err != nil {
		return nil, err
	}
	return vmClass, nil
}
