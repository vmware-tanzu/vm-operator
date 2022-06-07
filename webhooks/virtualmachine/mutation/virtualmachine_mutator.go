// Copyright (c) 2019-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
)

const (
	webHookName = "default"

	NSXTYPE   = "NSXT"
	VDSTYPE   = "VSPHERE_NETWORK"
	NAMEDTYPE = "NAMED"
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

//nolint
func (m mutator) Mutate(ctx *context.WebhookRequestContext) admission.Response {
	vm, err := m.vmFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var wasMutated bool
	original := vm
	modified := original.DeepCopy()

	// Check and Add default NetworkInterface only for a newly created VM.
	if modified.CreationTimestamp.IsZero() {
		if AddDefaultNetworkInterface(ctx, m.client, modified) {
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

	networkProviderType := os.Getenv(lib.NetworkProviderType)
	networkType := ""
	networkName := ""
	switch networkProviderType {
	case NSXTYPE:
		networkType = network.NsxtNetworkType
	case VDSTYPE:
		networkType = network.VdsNetworkType
	case NAMEDTYPE:
		// gce2e/local setup only
		// Use named network provider
		name, _ := getVSphereProviderConfigMap(ctx, client)
		if name == "" {
			networkName = "VM Network"
		} else {
			networkName = name
		}
	default:
		return false
	}
	defaultNif := vmopv1.VirtualMachineNetworkInterface{
		NetworkType: networkType,
		NetworkName: networkName,
	}
	vm.Spec.NetworkInterfaces = []vmopv1.VirtualMachineNetworkInterface{defaultNif}
	return true
}

// Only used in gce2e tests.
func getVSphereProviderConfigMap(ctx *context.WebhookRequestContext, c client.Client) (string, error) {
	configMapKey := client.ObjectKey{Name: config.ProviderConfigMapName, Namespace: ctx.Namespace}
	configMap := corev1.ConfigMap{}
	if err := c.Get(ctx, configMapKey, &configMap); err != nil {
		return "", err
	}

	data := configMap.Data
	return data["Network"], nil
}
