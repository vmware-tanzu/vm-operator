// Copyright (c) 2019-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	goctx "context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/nsx.vmware.com/v1alpha1"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

const (
	webHookName          = "default"
	defaultInterfaceName = "eth0"
	defaultNamedNetwork  = "VM Network"
)

// +kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha3-virtualmachine,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,verbs=create;update,versions=v1alpha3,name=default.mutating.virtualmachine.v1alpha3.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get
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

	modified, err := m.vmFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var wasMutated bool

	switch ctx.Op {
	case admissionv1.Create:
		// SetCreatedAtAnnotations always mutates the VM on create.
		wasMutated = true
		SetCreatedAtAnnotations(ctx, modified)
		AddDefaultNetworkInterface(ctx, m.client, modified)
		SetDefaultPowerState(ctx, m.client, modified)
		if _, err := ResolveImageNameOnCreate(ctx, m.client, modified); err != nil {
			return admission.Denied(err.Error())
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

	rawModified, err := json.Marshal(modified)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(ctx.RawObj, rawModified)
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

// AddDefaultNetworkInterface adds default network interface to a VM if the NoNetwork annotation is not set
// and no NetworkInterface is specified.
// Return true if default NetworkInterface is added, otherwise return false.
func AddDefaultNetworkInterface(ctx *context.WebhookRequestContext, client client.Client, vm *vmopv1.VirtualMachine) bool {
	// Continue to support this ad-hoc v1a1 annotation. I don't think need or want to have this annotation
	// in v1a2: Disabled mostly already covers it. We could map between the two for version conversion, but
	// they do mean slightly different things, and kind of complicated to know what to do like if the annotation
	// is removed.
	if _, ok := vm.Annotations[v1alpha1.NoDefaultNicAnnotation]; ok {
		return false
	}

	if vm.Spec.Network != nil && vm.Spec.Network.Disabled {
		return false
	}

	kind, apiVersion, netName := "", "", ""
	switch pkgconfig.FromContext(ctx).NetworkProviderType {
	case pkgconfig.NetworkProviderTypeNSXT:
		kind = "VirtualNetwork"
		apiVersion = ncpv1alpha1.SchemeGroupVersion.String()
	case pkgconfig.NetworkProviderTypeVDS:
		kind = "Network"
		apiVersion = netopv1alpha1.SchemeGroupVersion.String()
	case pkgconfig.NetworkProviderTypeVPC:
		kind = "SubnetSet"
		apiVersion = vpcv1alpha1.SchemeGroupVersion.String()
	case pkgconfig.NetworkProviderTypeNamed:
		netName, _ = getProviderConfigMap(ctx, client)
		if netName == "" {
			netName = defaultNamedNetwork
		}
	default:
		return false
	}

	networkRef := common.PartialObjectRef{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: apiVersion,
		},
		Name: netName,
	}

	if vm.Spec.Network == nil {
		vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
	}

	var updated bool
	if len(vm.Spec.Network.Interfaces) == 0 {
		vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
			{
				Name:    defaultInterfaceName,
				Network: &networkRef,
			},
		}
		updated = true
	} else {
		for i := range vm.Spec.Network.Interfaces {
			ifaceNetwork := vm.Spec.Network.Interfaces[i].Network

			if networkRef.Kind != "" && ifaceNetwork == nil {
				ifaceNetwork = &common.PartialObjectRef{
					TypeMeta: metav1.TypeMeta{
						APIVersion: networkRef.APIVersion,
						Kind:       networkRef.Kind,
					},
				}
				vm.Spec.Network.Interfaces[i].Network = ifaceNetwork
				updated = true

			} else if networkRef.Kind != "" && ifaceNetwork.Kind == "" && ifaceNetwork.APIVersion == "" {
				ifaceNetwork.Kind = networkRef.Kind
				ifaceNetwork.APIVersion = networkRef.APIVersion
				updated = true
			}

			// Named network only.
			if networkRef.Kind != "" && ifaceNetwork == nil {
				ifaceNetwork = &common.PartialObjectRef{
					Name: networkRef.Name,
				}
				vm.Spec.Network.Interfaces[i].Network = ifaceNetwork
			} else if networkRef.Name != "" && ifaceNetwork.Name == "" {
				ifaceNetwork.Name = networkRef.Name
				updated = true
			}
		}
	}

	return updated
}

// getProviderConfigMap is used in e2e tests.
func getProviderConfigMap(ctx *context.WebhookRequestContext, c client.Client) (string, error) {
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

// SetDefaultPowerState sets the default power state for a new VM.
// Return true if the default power state was set, otherwise false.
func SetDefaultPowerState(
	ctx *context.WebhookRequestContext,
	client client.Client,
	vm *vmopv1.VirtualMachine) bool {

	if vm.Spec.PowerState == "" {
		vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
		return true
	}
	return false
}

const (
	vmiKind            = "VirtualMachineImage"
	cvmiKind           = "Cluster" + vmiKind
	imgNotFoundFormat  = "no VM image exists for %q in namespace or cluster scope"
	imgNameNotMatchRef = "must refer to the same resource as spec.image"
)

// ResolveImageNameOnCreate ensures vm.spec.image is set to a non-empty value if
// vm.spec.imageName is also non-empty.
func ResolveImageNameOnCreate(
	ctx *context.WebhookRequestContext,
	c client.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	// Return early if the VM image name is empty.
	imgName := vm.Spec.ImageName
	if imgName == "" {
		return false, nil
	}

	img, err := vmopv1util.ResolveImageName(ctx, c, vm.Namespace, imgName)
	if err != nil {
		return false, err
	}

	var (
		resolvedKind string
		resolvedName string
	)

	switch timg := img.(type) {
	case *vmopv1.VirtualMachineImage:
		resolvedKind = vmiKind
		resolvedName = timg.Name
	case *vmopv1.ClusterVirtualMachineImage:
		resolvedKind = cvmiKind
		resolvedName = timg.Name
	}

	// If no image spec was provided then update it.
	if vm.Spec.Image == nil {
		vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
			Kind: resolvedKind,
			Name: resolvedName,
		}
		return true, nil
	}

	// Verify that the resolved image name matches the value in spec.image.
	if resolvedKind != vm.Spec.Image.Kind || resolvedName != vm.Spec.Image.Name {
		return false, field.Invalid(field.NewPath("spec", "imageName"), vm.Spec.ImageName, imgNameNotMatchRef)
	}

	return false, nil
}

func SetCreatedAtAnnotations(ctx goctx.Context, vm *vmopv1.VirtualMachine) {
	// If this is the first time the VM has been create, then record the
	// build version and storage schema version into the VM's annotations.
	// This enables future work to know at what version a VM was "created."
	if vm.Annotations == nil {
		vm.Annotations = map[string]string{}
	}
	vm.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = pkgconfig.FromContext(ctx).BuildVersion
	vm.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = vmopv1.SchemeGroupVersion.Version
}
