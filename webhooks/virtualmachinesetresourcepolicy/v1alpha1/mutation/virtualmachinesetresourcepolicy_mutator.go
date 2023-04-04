// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"encoding/json"
	"net/http"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

const (
	webHookName = "default"
)

// -kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha1-virtualmachinesetresourcepolicy,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies,verbs=create;update,versions=v1alpha1,name=default.mutating.virtualmachinesetresourcepolicy.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// -kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicy,verbs=get;list
// -kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicy/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewMutatingWebhook(ctx, mgr, webHookName, NewMutator(nil))
	if err != nil {
		return errors.Wrapf(err, "failed to create mutation webhook")
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewMutator returns the package's Mutator.
func NewMutator(_ client.Client) builder.Mutator {
	return mutator{
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type mutator struct {
	converter runtime.UnstructuredConverter
}

func (m mutator) Mutate(ctx *context.WebhookRequestContext) admission.Response {
	vmRP, err := m.vmRPFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var wasMutated bool
	original := vmRP
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
	return vmopv1.SchemeGroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineSetResourcePolicy{}).Name())
}

// vmRPFromUnstructured returns the VirtualMachineSetResourcePolicy from the unstructured object.
func (m mutator) vmRPFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineSetResourcePolicy, error) {
	vmRP := &vmopv1.VirtualMachineSetResourcePolicy{}
	if err := m.converter.FromUnstructured(obj.UnstructuredContent(), vmRP); err != nil {
		return nil, err
	}
	return vmRP, nil
}
