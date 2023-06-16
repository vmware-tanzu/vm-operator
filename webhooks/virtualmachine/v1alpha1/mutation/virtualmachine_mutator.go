// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
)

const (
	webHookName         = "default"
	defaultNamedNetwork = "VM Network"
)

// +kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha1-virtualmachine,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,verbs=create;update,versions=v1alpha1,name=default.mutating.virtualmachine.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
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

	switch ctx.Op {
	case admissionv1.Create:
		if AddDefaultNetworkInterface(ctx, m.client, modified) {
			wasMutated = true
		}
		if SetDefaultPowerState(ctx, m.client, modified) {
			wasMutated = true
		}
	case admissionv1.Update:
		oldVM, err := m.vmFromUnstructured(ctx.OldObj)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		if ok, err := SetNextRestartTime(ctx, m.client, modified, oldVM); err != nil {
			return admission.Denied(err.Error())
		} else if ok {
			wasMutated = true
		}
		// Prevent someone from setting the Spec.VmMetadata.SecretName
		// field to an empty string if the field is already set to a
		// non-empty string.
		if omd := original.Spec.VmMetadata; omd != nil && omd.SecretName == "" {
			vmOnDisk, err := m.vmFromUnstructured(ctx.OldObj)
			if err != nil {
				return admission.Errored(http.StatusInternalServerError, err)
			}
			if cmd := vmOnDisk.Spec.VmMetadata; cmd != nil {
				if sn := cmd.SecretName; sn != "" {
					modified.Spec.VmMetadata.SecretName = sn
					wasMutated = true
				}
			}
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

// SetDefaultPowerState sets the default power state for a new VM.
// Return true if the default power state was set, otherwise false.
func SetDefaultPowerState(
	ctx *context.WebhookRequestContext,
	client client.Client,
	vm *vmopv1.VirtualMachine) bool {

	if vm.Spec.PowerState == "" {
		vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOn
		return true
	}
	return false
}

// SetNextRestartTime sets spec.nextRestartTime for a VM if the field's
// current value is equal to "now" (case-insensitive).
// Return true if set, otherwise false.
func SetNextRestartTime(
	ctx *context.WebhookRequestContext,
	client client.Client,
	newVM, oldVM *vmopv1.VirtualMachine) (bool, error) {

	if newVM.Spec.NextRestartTime == "" {
		newVM.Spec.NextRestartTime = oldVM.Spec.NextRestartTime
		return oldVM.Spec.NextRestartTime != "", nil
	}
	if strings.EqualFold("now", newVM.Spec.NextRestartTime) {
		if oldVM.Spec.PowerState != vmopv1.VirtualMachinePoweredOn {
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

// AddDefaultNetworkInterface adds default network interface to a VM if the NoNetwork annotation is not set
// and no NetworkInterface is specified.
// Return true if default NetworkInterface is added, otherwise return false.
func AddDefaultNetworkInterface(ctx *context.WebhookRequestContext, client client.Client, vm *vmopv1.VirtualMachine) bool {
	if _, ok := vm.Annotations[vmopv1.NoDefaultNicAnnotation]; ok {
		return false
	}

	if len(vm.Spec.NetworkInterfaces) != 0 {
		return false
	}

	netName, netType := "", ""
	switch os.Getenv(lib.NetworkProviderType) {
	case lib.NetworkProviderTypeNSXT:
		netType = network.NsxtNetworkType
	case lib.NetworkProviderTypeVDS:
		netType = network.VdsNetworkType
	case lib.NetworkProviderTypeNamed:
		if netName, _ = getProviderConfigMap(ctx, client); netName == "" {
			netName = defaultNamedNetwork
		}
	default:
		return false
	}
	vm.Spec.NetworkInterfaces = []vmopv1.VirtualMachineNetworkInterface{
		{
			NetworkName: netName,
			NetworkType: netType,
		},
	}
	return true
}

// getProviderConfigMap is used in e2e tests.
func getProviderConfigMap(
	ctx *context.WebhookRequestContext, c client.Client) (string, error) {

	var obj corev1.ConfigMap
	if err := c.Get(
		ctx,
		client.ObjectKey{
			Name:      config.ProviderConfigMapName,
			Namespace: ctx.Namespace,
		},
		&obj); err != nil {
		return "", err
	}
	return obj.Data["Network"], nil
}
