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
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha2-virtualmachine,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,verbs=create;update,versions=v1alpha2,name=default.mutating.virtualmachine.v1alpha2.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachine,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachine/status,verbs=get
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=clustervirtualmachineimages,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=clustervirtualmachineimages/status,verbs=get;list;watch

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	// Index the VirtualMachineImage and ClusterVirtualMachineImage objects by
	// status.name field to allow efficient querying in ResolveImageName().
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&vmopv1.VirtualMachineImage{},
		"status.name",
		func(rawObj client.Object) []string {
			vmi := rawObj.(*vmopv1.VirtualMachineImage)
			return []string{vmi.Status.Name}
		}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&vmopv1.ClusterVirtualMachineImage{},
		"status.name",
		func(rawObj client.Object) []string {
			cvmi := rawObj.(*vmopv1.ClusterVirtualMachineImage)
			return []string{cvmi.Status.Name}
		}); err != nil {
		return err
	}

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

	switch ctx.Op {
	case admissionv1.Create:
		if mutated, err := ResolveImageName(ctx, m.client, modified); err != nil {
			return admission.Denied(err.Error())
		} else if mutated {
			wasMutated = true
		}
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

// ResolveImageName mutates the vm.spec.imageName if it's not set to a vmi name
// and there is a single namespace or cluster scope image with that status name.
func ResolveImageName(
	ctx *context.WebhookRequestContext,
	c client.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {
	// Return early if the VM image name is empty or already set to a vmi name.
	imageName := vm.Spec.ImageName
	if imageName == "" || !lib.IsWCPVMImageRegistryEnabled() ||
		strings.HasPrefix(imageName, "vmi-") {
		return false, nil
	}

	var determinedImageName string
	// Check if a single namespace scope image exists by the status name.
	vmiList := &vmopv1.VirtualMachineImageList{}
	if err := c.List(ctx, vmiList, client.InNamespace(vm.Namespace),
		client.MatchingFields{
			"status.name": imageName,
		},
	); err != nil {
		return false, err
	}
	switch len(vmiList.Items) {
	case 0:
		break
	case 1:
		determinedImageName = vmiList.Items[0].Name
	default:
		return false, errors.Errorf("multiple VM images exist for %q in namespace scope", imageName)
	}

	// Check if a single cluster scope image exists by the status name.
	cvmiList := &vmopv1.ClusterVirtualMachineImageList{}
	if err := c.List(ctx, cvmiList, client.MatchingFields{
		"status.name": imageName,
	}); err != nil {
		return false, err
	}
	switch len(cvmiList.Items) {
	case 0:
		break
	case 1:
		if determinedImageName != "" {
			return false, errors.Errorf("multiple VM images exist for %q in namespace and cluster scope", imageName)
		}
		determinedImageName = cvmiList.Items[0].Name
	default:
		return false, errors.Errorf("multiple VM images exist for %q in cluster scope", imageName)
	}

	if determinedImageName == "" {
		return false, errors.Errorf("no VM image exists for %q in namespace or cluster scope", imageName)
	}

	vm.Spec.ImageName = determinedImageName
	return true, nil
}
