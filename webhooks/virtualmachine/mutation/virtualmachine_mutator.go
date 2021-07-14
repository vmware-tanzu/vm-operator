// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	goctx "context"
	"encoding/json"
	"net/http"
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha1-virtualmachine,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,verbs=create;update,versions=v1alpha1,name=default.mutating.virtualmachine.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1beta1,webhookVersions=v1beta1,webhookVersions=v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachine,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachine/status,verbs=get
// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=availabilityzones,verbs=get;list;watch
// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=availabilityzones/status,verbs=get;list;watch

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

var (
	zoneIndex int
	zoneMu    sync.Mutex
)

//nolint
func (m mutator) Mutate(ctx *context.WebhookRequestContext) admission.Response {
	vm, err := m.vmFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var wasMutated bool
	original := vm
	modified := original.DeepCopy()

	mutatedAZ, err := m.mutateAvailabilityZone(ctx.Context, original, modified)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if mutatedAZ {
		wasMutated = true
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

func (m mutator) mutateAvailabilityZone(
	ctx goctx.Context,
	vmOld, vmNew *vmopv1.VirtualMachine) (bool, error) {

	// Do not overwrite the topology key if one exists. It does not matter if
	// the zone is invalid -- that is checked in the validating webhook.
	if vmNew.Labels[topology.KubernetesTopologyZoneLabelKey] != "" {
		return false, nil
	}

	zones, err := topology.GetAvailabilityZones(ctx, m.client)
	if err != nil {
		return false, err
	}

	if len(zones) == 0 {
		return false, topology.ErrNoAvailabilityZones
	}

	// Get the next available zone name.
	var zoneName string
	func() {
		zoneMu.Lock()
		defer zoneMu.Unlock()

		if zoneIndex >= len(zones) {
			zoneIndex = 0
		}

		zoneName = zones[zoneIndex].Name
		zoneIndex++
	}()

	// If the only zone is the default zone and the FSS is not enabled,
	// do not mutate the VM.
	if !lib.IsWcpFaultDomainsFSSEnabled() {
		if zoneName == topology.DefaultAvailabilityZoneName {
			return false, nil
		}
	}

	if vmNew.Labels == nil {
		vmNew.Labels = map[string]string{}
	}

	vmNew.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
	return true, nil
}
