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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-mutate-vmoperator-vmware-com-v1alpha5-virtualmachinesnapshot,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinesnapshots,versions=v1alpha5,name=default.mutating.virtualmachinesnapshot.v1alpha5.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1

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
	if ctx.Op != admissionv1.Create {
		return admission.Allowed("")
	}

	modified, err := m.vmSnapshotFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Always set the VM name label on create
	if wasMutated := SetVMNameLabel(modified); !wasMutated {
		return admission.Allowed("")
	}

	rawModified, err := json.Marshal(modified)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(ctx.RawObj, rawModified)
}

func (m mutator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineSnapshot{}).Name())
}

// vmSnapshotFromUnstructured returns the VirtualMachineSnapshot from the unstructured object.
func (m mutator) vmSnapshotFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineSnapshot, error) {
	vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
	if err := m.converter.FromUnstructured(obj.UnstructuredContent(), vmSnapshot); err != nil {
		return nil, err
	}
	return vmSnapshot, nil
}

// SetVMNameLabel sets the VM name label on the snapshot if it has a vmRef.
// Returns true if the snapshot was mutated, false otherwise.
func SetVMNameLabel(vmSnapshot *vmopv1.VirtualMachineSnapshot) bool {
	// Only set the label if there's a vmName.
	if vmSnapshot.Spec.VMName == "" {
		return false
	}

	// Add the label if it does not exist.
	if _, exists := vmSnapshot.Labels[vmopv1.VMNameForSnapshotLabel]; !exists {
		vmName := vmSnapshot.Spec.VMName
		metav1.SetMetaDataLabel(&vmSnapshot.ObjectMeta, vmopv1.VMNameForSnapshotLabel, vmName)

		return true
	}

	return false
}
