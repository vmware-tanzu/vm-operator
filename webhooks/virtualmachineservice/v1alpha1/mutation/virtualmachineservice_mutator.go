// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

const (
	webHookName = "default"
)

// -kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha1-virtualmachineservice,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachineservices,verbs=create;update,versions=v1alpha1,name=default.mutating.virtualmachineservice.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// -kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineservice,verbs=get;list
// -kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineservice/status,verbs=get

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
	vmService, err := m.vmServiceFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var wasMutated bool
	original := vmService
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
	return vmopv1.SchemeGroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineService{}).Name())
}

// vmServiceFromUnstructured returns the VirtualMachineService from the unstructured object.
func (m mutator) vmServiceFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineService, error) {
	vmService := &vmopv1.VirtualMachineService{}
	if err := m.converter.FromUnstructured(obj.UnstructuredContent(), vmService); err != nil {
		return nil, err
	}
	return vmService, nil
}
