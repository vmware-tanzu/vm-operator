// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha5-virtualmachinegrouppublishrequest,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinegrouppublishrequests,verbs=create,versions=v1alpha5,name=default.mutating.virtualmachinegrouppublishrequest.v1alpha5.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegrouppublishrequests,verbs=get;list;update
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraries,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups,verbs=get

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

func (m mutator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineGroupPublishRequest{}).Name())
}

func (m mutator) Mutate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	// this check is unnecessary but serves as a saft guard
	if ctx.Op != admissionv1.Create {
		return admission.Allowed("")
	}

	return m.mutateCreate(ctx)
}

// mutateCreate sets spec.source to the VM group publish request's name if it is omitted,
// spec.target to a default content library if one exists, and spec.virtualMachines to
// all the VMs under source VM group.
func (m mutator) mutateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	groupPubReq, err := m.vmGroupPublishRequestFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if groupPubReq.Spec.Source == "" {
		groupPubReq.Spec.Source = groupPubReq.Name
	}

	if groupPubReq.Spec.Target == "" {
		defaultCL, err := common.RetrieveDefaultImagePublishContentLibrary(ctx, m.client, groupPubReq.Namespace)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return admission.Errored(http.StatusNotFound, fmt.Errorf("cannot find a default content library with the %q label", common.DefaultImagePublishContentLibraryLabelKey))
			}
			if apierrors.IsBadRequest(err) {
				return admission.Errored(http.StatusBadRequest, err)
			}
			return admission.Errored(http.StatusInternalServerError, err)
		}
		groupPubReq.Spec.Target = defaultCL.Name
	}

	if len(groupPubReq.Spec.VirtualMachines) == 0 {
		visitedGroups := &sets.Set[string]{}
		vmSet, err := vmopv1util.RetrieveVMGroupMembers(ctx, m.client,
			ctrlclient.ObjectKey{Namespace: groupPubReq.Namespace, Name: groupPubReq.Spec.Source}, visitedGroups)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return admission.Errored(http.StatusNotFound, err)
			}
			return admission.Errored(http.StatusInternalServerError, err)
		}
		if vmSet.Len() == 0 {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("%q group does not have any memebers", groupPubReq.Spec.Source))
		}

		// ensure vms are unique and sorted when it is set by the mutator
		groupPubReq.Spec.VirtualMachines = sets.List(vmSet)
	}

	rawGroupPubReq, err := json.Marshal(groupPubReq)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(ctx.RawObj, rawGroupPubReq)
}

// vmGroupPublishRequestFromUnstructured returns the VirtualMachineGroupPublishRequest from the unstructured object.
func (m mutator) vmGroupPublishRequestFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineGroupPublishRequest, error) {
	vmGroupPublishRequest := &vmopv1.VirtualMachineGroupPublishRequest{}
	if err := m.converter.FromUnstructured(obj.UnstructuredContent(), vmGroupPublishRequest); err != nil {
		return nil, err
	}
	return vmGroupPublishRequest, nil
}
