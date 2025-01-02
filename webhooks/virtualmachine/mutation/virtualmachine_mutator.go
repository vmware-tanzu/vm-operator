// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

const (
	webHookName            = "default"
	defaultInterfaceName   = "eth0"
	defaultNamedNetwork    = "VM Network"
	defaultCdromNamePrefix = "cdrom"
)

// +kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha3-virtualmachine,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,verbs=create;update,versions=v1alpha3,name=default.mutating.virtualmachine.v1alpha3.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineimages/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=clustervirtualmachineimages,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=clustervirtualmachineimages/status,verbs=get;list;watch

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	// Index the VirtualMachineImage and ClusterVirtualMachineImage objects by
	// status.name field to allow efficient querying in ResolveImageName().
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&vmopv1.VirtualMachineImage{},
		"status.name",
		func(rawObj ctrlclient.Object) []string {
			vmi := rawObj.(*vmopv1.VirtualMachineImage)
			return []string{vmi.Status.Name}
		}); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&vmopv1.ClusterVirtualMachineImage{},
		"status.name",
		func(rawObj ctrlclient.Object) []string {
			cvmi := rawObj.(*vmopv1.ClusterVirtualMachineImage)
			return []string{cvmi.Status.Name}
		}); err != nil {
		return err
	}

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
		SetDefaultCdromImgKindOnCreate(ctx, modified)
		SetImageNameFromCdrom(ctx, modified)
		if _, err := SetDefaultInstanceUUID(ctx, m.client, modified); err != nil {
			return admission.Denied(err.Error())
		}
		if _, err := SetDefaultBiosUUID(ctx, m.client, modified); err != nil {
			return admission.Denied(err.Error())
		}
		if _, err := ResolveImageNameOnCreate(ctx, m.client, modified); err != nil {
			return admission.Denied(err.Error())
		}
		if pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
			if _, err := SetDefaultEncryptionClass(ctx, m.client, modified); err != nil {
				return admission.Denied(err.Error())
			}
		}
	case admissionv1.Update:
		oldVM, err := m.vmFromUnstructured(ctx.OldObj)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		if ok, err := SetLastResizeAnnotation(ctx, modified, oldVM); err != nil {
			return admission.Denied(err.Error())
		} else if ok {
			wasMutated = true
		}

		if ok, err := SetNextRestartTime(ctx, modified, oldVM); err != nil {
			return admission.Denied(err.Error())
		} else if ok {
			wasMutated = true
		}

		if ok := SetDefaultCdromImgKindOnUpdate(ctx, modified, oldVM); ok {
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
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachine{}).Name())
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
	ctx *pkgctx.WebhookRequestContext,
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
func AddDefaultNetworkInterface(ctx *pkgctx.WebhookRequestContext, client ctrlclient.Client, vm *vmopv1.VirtualMachine) bool {
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
	switch pkgcfg.FromContext(ctx).NetworkProviderType {
	case pkgcfg.NetworkProviderTypeNSXT:
		kind = "VirtualNetwork"
		apiVersion = ncpv1alpha1.SchemeGroupVersion.String()
	case pkgcfg.NetworkProviderTypeVDS:
		kind = "Network"
		apiVersion = netopv1alpha1.SchemeGroupVersion.String()
	case pkgcfg.NetworkProviderTypeVPC:
		kind = "SubnetSet"
		apiVersion = vpcv1alpha1.SchemeGroupVersion.String()
	case pkgcfg.NetworkProviderTypeNamed:
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
func getProviderConfigMap(ctx *pkgctx.WebhookRequestContext, c ctrlclient.Client) (string, error) {
	var obj corev1.ConfigMap
	if err := c.Get(
		ctx,
		ctrlclient.ObjectKey{
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
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) bool {

	if vm.Spec.PowerState == "" {
		vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
		return true
	}
	return false
}

// SetDefaultInstanceUUID sets a default instance uuid for a new VM.
// Return true if a default instance uuid was set, otherwise false.
func SetDefaultInstanceUUID(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	// DevOps users are not allowed to set spec.instanceUUID when creating a VM.
	if !ctx.IsPrivilegedAccount && vm.Spec.InstanceUUID != "" {
		return false, field.Forbidden(
			field.NewPath("spec", "instanceUUID"),
			"only privileged users may set this field")
	}

	if vm.Spec.InstanceUUID == "" {
		// Default to a Random (Version 4) UUID.
		// This is the same UUID flavor/version used by Kubernetes and preferred
		// by vSphere.
		vm.Spec.InstanceUUID = uuid.NewString()
		return true, nil
	}

	return false, nil
}

// SetDefaultBiosUUID sets a default bios uuid for a new VM.
// If CloudInit is the Bootstrap method, CloudInit InstanceID is also set to
// BiosUUID.
// Return true if a default bios uuid was set, otherwise false.
func SetDefaultBiosUUID(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	// DevOps users are not allowed to set spec.biosUUID when creating a VM.
	if !ctx.IsPrivilegedAccount && vm.Spec.BiosUUID != "" {
		return false, field.Forbidden(
			field.NewPath("spec", "biosUUID"),
			"only privileged users may set this field")
	}

	var wasMutated bool

	if vm.Spec.BiosUUID == "" {
		// Default to a Random (Version 4) UUID.
		// This is the same UUID flavor/version used by Kubernetes and preferred
		// by vSphere.
		vm.Spec.BiosUUID = uuid.NewString()
		wasMutated = true
	}

	if bs := vm.Spec.Bootstrap; bs != nil {
		if ci := bs.CloudInit; ci != nil {
			if ci.InstanceID == "" {
				// If the Cloud-Init bootstrap provider was selected and the
				// value for spec.bootstrap.cloudInit.instanceID was omitted,
				// then default it to the value of spec.biosUUID.
				ci.InstanceID = vm.Spec.BiosUUID
				wasMutated = true
			}
		}
	}

	return wasMutated, nil
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
	ctx *pkgctx.WebhookRequestContext,
	c ctrlclient.Client,
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

func SetCreatedAtAnnotations(ctx context.Context, vm *vmopv1.VirtualMachine) {
	// If this is the first time the VM has been created, then record the
	// build version and storage schema version into the VM's annotations.
	// This enables future work to know at what version a VM was "created."
	if vm.Annotations == nil {
		vm.Annotations = map[string]string{}
	}
	vm.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = pkgcfg.FromContext(ctx).BuildVersion
	vm.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = vmopv1.GroupVersion.Version
}

// SetLastResizeAnnotation sets the last resize annotation as needed when the class name changes.
func SetLastResizeAnnotation(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) (bool, error) {

	if f := pkgcfg.FromContext(ctx).Features; !f.VMResize && !f.VMResizeCPUMemory {
		return false, nil
	}

	if vm.Spec.ClassName == oldVM.Spec.ClassName {
		return false, nil
	}

	if _, _, _, ok := vmopv1util.GetLastResizedAnnotation(*vm); !ok {
		// This is an existing VM - since it does not have the last-resized annotation - that is
		// now being resized. Save off the prior class name so we know the class has changed and
		// we'll perform a resize.
		if err := vmopv1util.SetLastResizedAnnotationClassName(vm, oldVM.Spec.ClassName); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

func SetDefaultCdromImgKindOnCreate(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) {

	for i, c := range vm.Spec.Cdrom {
		if c.Image.Kind == "" {
			vm.Spec.Cdrom[i].Image.Kind = vmiKind
		}
	}
}

func SetDefaultCdromImgKindOnUpdate(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) bool {

	var (
		mutated          bool
		imgNameToOldKind = make(map[string]string, len(oldVM.Spec.Cdrom))
	)

	for _, c := range oldVM.Spec.Cdrom {
		imgNameToOldKind[c.Image.Name] = c.Image.Kind
	}

	for i, c := range vm.Spec.Cdrom {
		// Repopulate the image kind only if it was previously set to default.
		// This ensures an error is returned if the image kind was reset from
		// a different value other than the default VirtualMachineImage kind.
		if c.Image.Kind == "" && imgNameToOldKind[c.Image.Name] == vmiKind {
			vm.Spec.Cdrom[i].Image.Kind = vmiKind
			mutated = true
		}
	}

	return mutated
}

func SetImageNameFromCdrom(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) {

	// Return early if the VM image name is set or no CD-ROMs are specified.
	if vm.Spec.ImageName != "" || len(vm.Spec.Cdrom) == 0 {
		return
	}

	var cdromImageName string
	for _, cdrom := range vm.Spec.Cdrom {
		// Set the image name to the first connected CD-ROM image name.
		if cdrom.Connected != nil && *cdrom.Connected {
			cdromImageName = cdrom.Image.Name
			break
		}
	}

	// If no connected CD-ROM is found, set it to the first CD-ROM image name.
	if cdromImageName == "" {
		cdromImageName = vm.Spec.Cdrom[0].Image.Name
	}

	vm.Spec.ImageName = cdromImageName
}

// SetDefaultEncryptionClass assigns spec.crypto.encryptionClassName to the
// namespace's default EncryptionClass when creating a VM if spec.crypto is
// nil or spec.crypto.encryptionClassName is empty.
func SetDefaultEncryptionClass(
	ctx *pkgctx.WebhookRequestContext,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	// Return early if the VM already specifies an EncryptionClass.
	if c := vm.Spec.Crypto; c != nil && c.EncryptionClassName != "" {
		return false, nil
	}

	// Return early if the VM does not specify a StorageClass.
	if vm.Spec.StorageClass == "" {
		return false, nil
	}

	// Check if the VM's storage class supports encryption.
	if ok, _, err := kubeutil.IsEncryptedStorageClass(
		ctx,
		k8sClient,
		vm.Spec.StorageClass); err != nil {

		return false, err
	} else if !ok {

		// The VM Class does not support encryption, so we do not need to set
		// a default EncryptionClass for the VM.
		return false, nil
	}

	// Get the default encryption class for this namespace.
	defaultEncClass, err := kubeutil.GetDefaultEncryptionClassForNamespace(
		ctx,
		k8sClient,
		vm.Namespace)
	if err != nil {
		if errors.Is(err, kubeutil.ErrNoDefaultEncryptionClass) {
			return false, nil
		}
		return false, err
	}

	// Update the VM with the namespace's default EncryptionClass name.
	if vm.Spec.Crypto == nil {
		vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{}
	}
	vm.Spec.Crypto.EncryptionClassName = defaultEncClass.Name

	return true, nil

}
