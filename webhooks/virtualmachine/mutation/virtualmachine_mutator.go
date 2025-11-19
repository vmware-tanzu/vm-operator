// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
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
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
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

	vmclassKind = "VirtualMachineClass"
)

// +kubebuilder:webhook:path=/default-mutate-vmoperator-vmware-com-v1alpha5-virtualmachine,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,verbs=create;update,versions=v1alpha5,name=default.mutating.virtualmachine.v1alpha5.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
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

// MutateOnCreateFn is a function called to mutate a VM on create.
type MutateOnCreateFn func(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	newVM *vmopv1.VirtualMachine) (wasMutated bool, _ error)

// MutateOnUpdateFn is a function called to mutate a VM on update.
type MutateOnUpdateFn func(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	newVM, oldVM *vmopv1.VirtualMachine) (wasMutated bool, _ error)

var (
	// MutateOnCreateFuncs is used to register functions called when a VM is
	// created. Values must be type MutateOnCreateFn.
	MutateOnCreateFuncs sync.Map

	// MutateOnUpdateFuncs is used to register functions called when a VM is
	// updated. Values must be type MutateOnUpdateFn.
	MutateOnUpdateFuncs sync.Map
)

func init() {
	MutateOnCreateFuncs.Store(
		"create.vmoperator.vmware.com/set-created-at-annotations",
		(MutateOnCreateFn)(SetCreatedAtAnnotations))

	MutateOnCreateFuncs.Store(
		"create.vmoperator.vmware.com/set-default-controllers",
		(MutateOnCreateFn)(SetDefaultControllers))

	MutateOnUpdateFuncs.Store(
		"update.vmoperator.vmware.com/set-default-cdrom-image-kind",
		(MutateOnUpdateFn)(SetDefaultCdromImgKindOnUpdate))

	MutateOnUpdateFuncs.Store(
		"update.vmoperator.vmware.com/mutate-cdrom-controller",
		(MutateOnUpdateFn)(MutateCdromControllerOnUpdate))
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

// ResolveClassAndClassName resolves the spec.class and
// spec.className of a VM.
//   - If spec.className is specified, but spec.class is empty, then
//     spec.class is set to the active instance of the class.
//   - If a VM class instance is specified in spec.class but
//     spec.className is empty, spec.className is set to the class
//     that is the owner of the class instance.
//   - If both spec.class and spec.className both are specified, then we
//     ensure that both point to the same class.
func ResolveClassAndClassName(
	ctx *pkgctx.WebhookRequestContext,
	c ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	className := vm.Spec.ClassName
	classInstance := vm.Spec.Class

	// Return early if both spec.class and spec.className are unset.
	// This can be the case for classless / imported VMs.
	if className == "" && classInstance == nil {
		return false, nil
	}

	// If class instance is set, but the name is empty, it is invalid.
	if classInstance != nil && classInstance.Name == "" {
		return false, fmt.Errorf("empty class instance name")
	}

	// If class and class instance both are set, then pass through to the
	// validation webhook. Nothing to mutate.
	if className != "" && classInstance != nil {
		return false, nil
	}

	if className != "" {
		// If a class is specified, but no instance has been
		// specified, pick the active instance.
		if classInstance == nil {

			// TODO: AKP: This is inefficient. Each instance should
			// have a link to its parent class.
			// Fetch all the instances and filter the one owned by this class
			instanceList := &vmopv1.VirtualMachineClassInstanceList{}
			if err := c.List(ctx,
				instanceList,
				ctrlclient.InNamespace(vm.Namespace),
				ctrlclient.MatchingLabels{vmopv1.VMClassInstanceActiveLabelKey: ""},
			); err != nil {
				return false, err
			}

			// From all the active class instances, filter the ones
			// owned by the class specified in spec.className.
			for _, instance := range instanceList.Items {
				for _, owner := range instance.OwnerReferences {
					if owner.Name == className && owner.Kind == vmclassKind {
						if vm.Spec.Class == nil {
							vm.Spec.Class = &common.LocalObjectRef{}
						}
						vm.Spec.Class.APIVersion = instance.APIVersion
						vm.Spec.Class.Name = instance.Name
						vm.Spec.Class.Kind = instance.Kind

						return true, nil
					}
				}
			}
		}
	} else {
		// If an instance is specified in spec.class, but className is
		// omitted, fetch the VM class pointed by the instance and
		// populate spec.className.
		if vm.Spec.Class != nil {
			classInstance := &vmopv1.VirtualMachineClassInstance{}
			if err := c.Get(
				ctx,
				ctrlclient.ObjectKey{Name: vm.Spec.Class.Name, Namespace: vm.Namespace},
				classInstance); err != nil {
				return false, err
			}

			// Specifying an inactive instance is disallowed.
			if _, ok := classInstance.Labels[vmopv1.VMClassInstanceActiveLabelKey]; !ok {
				return false, fmt.Errorf("must specify a VirtualMachineClassInstance that is active")
			}

			// Set the spec.className to the class that owns the instance.
			for _, ownerClass := range classInstance.OwnerReferences {
				if ownerClass.Kind == vmclassKind {
					vm.Spec.ClassName = ownerClass.Name

					return true, nil
				}
			}
		}
	}

	return false, nil
}

//nolint:gocyclo
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
		if pkgcfg.FromContext(ctx).Features.ImmutableClasses {
			if _, err := ResolveClassAndClassName(ctx, m.client, modified); err != nil {
				return admission.Denied(err.Error())
			}
		}
		if pkgcfg.FromContext(ctx).Features.VMSharedDisks {
			if _, err := SetPVCVolumeDefaults(ctx, m.client, modified); err != nil {
				return admission.Denied(err.Error())
			}
		}

		// Iterate over the externally registered mutate functions.
		var rangeErr error
		MutateOnCreateFuncs.Range(func(key, value any) bool {
			if fn, ok := value.(MutateOnCreateFn); ok {
				if _, err := fn(ctx, m.client, modified); err != nil {
					rangeErr = err
					return false
				}
			}
			return true
		})
		if rangeErr != nil {
			return admission.Denied(rangeErr.Error())
		}

	case admissionv1.Update:
		oldVM, err := m.vmFromUnstructured(ctx.OldObj)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		if pkgcfg.FromContext(ctx).Features.ImmutableClasses {
			if _, err := ResolveClassAndClassName(ctx, m.client, modified); err != nil {
				return admission.Denied(err.Error())
			}
		}

		// When a VM is resized, the last resize instances are set to
		// the class and the instance. The VM controller uses these
		// annotations to figure out if a VM is indeed being resized
		// (and in some cases, the type of resize).
		if ok, err := SetLastResizeAnnotations(ctx, modified, oldVM); err != nil {
			return admission.Denied(err.Error())
		} else if ok {
			wasMutated = true
		}

		if ok, err := SetNextRestartTime(ctx, modified, oldVM); err != nil {
			return admission.Denied(err.Error())
		} else if ok {
			wasMutated = true
		}

		if pkgcfg.FromContext(ctx).Features.MutableNetworks {
			if ok := SetDefaultNetworkOnUpdate(ctx, m.client, modified); ok {
				wasMutated = true
			}
		}

		if pkgcfg.FromContext(ctx).Features.VMGroups {
			if ok := vmopv1util.RemoveStaleGroupOwnerRef(modified, oldVM); ok {
				wasMutated = true
			}
			if ok := CleanupApplyPowerStateChangeTimeAnno(ctx, modified, oldVM); ok {
				wasMutated = true
			}
		}

		if pkgcfg.FromContext(ctx).Features.VMSharedDisks {
			// Add controllers as needed for new volumes.
			if ok, err := AddControllersForVolumes(ctx, m.client, modified); err != nil {
				return admission.Denied(err.Error())
			} else if ok {
				wasMutated = true
			}

			if ok, err := SetPVCVolumeDefaults(ctx, m.client, modified); err != nil {
				return admission.Denied(err.Error())
			} else if ok {
				wasMutated = true
			}
		}

		// Iterate over the externally registered mutate functions.
		var rangeErr error
		MutateOnUpdateFuncs.Range(func(_, value any) bool {
			if fn, ok := value.(MutateOnUpdateFn); ok {
				wm, err := fn(ctx, m.client, modified, oldVM)
				if err != nil {
					rangeErr = err
					return false
				}
				if wm {
					wasMutated = true
				}
			}
			return true
		})
		if rangeErr != nil {
			return admission.Denied(rangeErr.Error())
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

func setDefaultNetworkInterfaceNetwork(
	vm *vmopv1.VirtualMachine,
	ifaceIdx int,
	networkRef common.PartialObjectRef,
) bool {
	ifaceNetwork := vm.Spec.Network.Interfaces[ifaceIdx].Network

	if networkRef.Kind != "" {
		if ifaceNetwork == nil {
			ifaceNetwork = &common.PartialObjectRef{
				TypeMeta: metav1.TypeMeta{
					APIVersion: networkRef.APIVersion,
					Kind:       networkRef.Kind,
				},
			}
			vm.Spec.Network.Interfaces[ifaceIdx].Network = ifaceNetwork
			return true
		}

		if ifaceNetwork.Kind == "" && ifaceNetwork.APIVersion == "" {
			ifaceNetwork.Kind = networkRef.Kind
			ifaceNetwork.APIVersion = networkRef.APIVersion
			return true
		}

	} else {
		// Named network only.
		if ifaceNetwork == nil {
			ifaceNetwork = &common.PartialObjectRef{
				Name: networkRef.Name,
			}
			vm.Spec.Network.Interfaces[ifaceIdx].Network = ifaceNetwork
			return true
		}

		if networkRef.Name != "" && ifaceNetwork.Name == "" {
			ifaceNetwork.Name = networkRef.Name
			return true
		}
	}

	return false
}

func getDefaultNetworkRef(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client) (bool, common.PartialObjectRef) {

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
		return false, common.PartialObjectRef{}
	}

	return true, common.PartialObjectRef{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: apiVersion,
		},
		Name: netName,
	}
}

// AddDefaultNetworkInterface adds default network interface to a VM if the NoNetwork annotation is not set
// and no NetworkInterface is specified.
// Return true if default NetworkInterface is added, otherwise return false.
func AddDefaultNetworkInterface(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) bool {

	// Continue to support this ad-hoc v1a1 annotation. I don't think need or want to have this annotation
	// in v1a2: Disabled mostly already covers it. We could map between the two for version conversion, but
	// they do mean slightly different things, and kind of complicated to know what to do like if the annotation
	// is removed.
	if _, ok := vm.Annotations[v1alpha1.NoDefaultNicAnnotation]; ok {
		if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 {
			return false
		}
	}

	if vm.Spec.Network != nil && vm.Spec.Network.Disabled {
		return false
	}

	ok, networkRef := getDefaultNetworkRef(ctx, client)
	if !ok {
		return false
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
			if ok := setDefaultNetworkInterfaceNetwork(vm, i, networkRef); ok {
				updated = true
			}
		}
	}

	return updated
}

func SetDefaultNetworkOnUpdate(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) bool {

	if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 || vm.Spec.Network.Disabled {
		return false
	}

	ok, networkRef := getDefaultNetworkRef(ctx, client)
	if !ok {
		return false
	}

	var updated bool
	for i := range vm.Spec.Network.Interfaces {
		if ok := setDefaultNetworkInterfaceNetwork(vm, i, networkRef); ok {
			updated = true
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

// SetDefaultBiosUUID sets a default bios uuid for a new VM if one was not
// provided. If CloudInit is the Bootstrap method, CloudInit InstanceID
// is also set to BiosUUID.
// Return true if a default bios uuid was set, otherwise false.
func SetDefaultBiosUUID(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

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

func SetCreatedAtAnnotations(
	ctx *pkgctx.WebhookRequestContext,
	_ ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (_ bool, _ error) {
	// If this is the first time the VM has been created, then record the
	// build version and storage schema version into the VM's annotations.
	// This enables future work to know at what version a VM was "created."
	if vm.Annotations == nil {
		vm.Annotations = map[string]string{}
	}
	vm.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = pkgcfg.FromContext(ctx).BuildVersion
	vm.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = vmopv1.GroupVersion.Version
	return true, nil
}

// SetDefaultControllers sets the default device controllers for a VM.
func SetDefaultControllers(
	_ *pkgctx.WebhookRequestContext,
	_ ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (_ bool, _ error) {
	if vm.Spec.Hardware == nil {
		vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
	}

	if len(vm.Spec.Hardware.IDEControllers) == 0 {
		for i := int32(0); i < vmopv1.VirtualControllerTypeIDE.MaxCount(); i++ {
			vm.Spec.Hardware.IDEControllers = append(
				vm.Spec.Hardware.IDEControllers,
				vmopv1.IDEControllerSpec{BusNumber: i},
			)
		}
		return true, nil
	}

	return false, nil
}

// SetLastResizeAnnotations sets the last resize annotation as needed when the class name changes.
func SetLastResizeAnnotations(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) (bool, error) {

	f := pkgcfg.FromContext(ctx).Features
	if !f.VMResize && !f.VMResizeCPUMemory {
		return false, nil
	}

	if vm.Spec.ClassName == oldVM.Spec.ClassName {
		if !f.ImmutableClasses {
			return false, nil
		}

		if vm.Spec.Class != nil && oldVM.Spec.Class != nil &&
			vm.Spec.Class.Name == oldVM.Spec.Class.Name {
			return false, nil
		}
	}

	modified := false
	if _, _, _, ok := vmopv1util.GetLastResizedAnnotation(*vm); !ok {
		// This is an existing VM - since it does not have the last-resized annotation - that is
		// now being resized. Save off the prior class name so we know the class has changed and
		// we'll perform a resize.
		if err := vmopv1util.SetLastResizedAnnotationClassName(vm, oldVM.Spec.ClassName); err != nil {
			return false, err
		}

		modified = true
	}

	if f.ImmutableClasses {
		if _, _, _, ok := vmopv1util.GetLastResizedInstanceAnnotation(*vm); !ok {
			instanceName := ""
			if oldVM.Spec.Class != nil {
				instanceName = oldVM.Spec.Class.Name
			}
			if err := vmopv1util.SetLastResizedAnnotationClassInstanceName(vm, instanceName); err != nil {
				return false, err
			}
		}
	}

	return modified, nil
}

func SetDefaultCdromImgKindOnCreate(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) {

	if vm.Spec.Hardware == nil {
		return
	}

	for i, c := range vm.Spec.Hardware.Cdrom {
		if c.Image.Kind == "" {
			vm.Spec.Hardware.Cdrom[i].Image.Kind = vmiKind
		}
	}
}

func SetDefaultCdromImgKindOnUpdate(
	ctx *pkgctx.WebhookRequestContext,
	_ ctrlclient.Client,
	vm, oldVM *vmopv1.VirtualMachine) (bool, error) {

	var (
		mutated          bool
		imgNameToOldKind map[string]string
		oldHW            = oldVM.Spec.Hardware
		newHW            = vm.Spec.Hardware
	)

	if newHW == nil {
		return false, nil
	}

	if oldHW != nil {
		imgNameToOldKind = make(map[string]string, len(oldHW.Cdrom))
		for _, c := range oldHW.Cdrom {
			imgNameToOldKind[c.Image.Name] = c.Image.Kind
		}
	}

	for i, c := range newHW.Cdrom {
		// Repopulate the image kind only if it was previously set to default.
		// This ensures an error is returned if the image kind was reset from
		// a different value other than the default VirtualMachineImage kind.
		if c.Image.Kind == "" && imgNameToOldKind[c.Image.Name] == vmiKind {
			newHW.Cdrom[i].Image.Kind = vmiKind
			mutated = true
		}
	}

	return mutated, nil
}

func SetImageNameFromCdrom(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) {

	// Return early if the VM image name is set or no CD-ROMs are specified.
	if vm.Spec.ImageName != "" || vm.Spec.Hardware == nil || len(vm.Spec.Hardware.Cdrom) == 0 {
		return
	}

	var cdromImageName string
	for _, cdrom := range vm.Spec.Hardware.Cdrom {
		// Set the image name to the first connected CD-ROM image name.
		if cdrom.Connected != nil && *cdrom.Connected {
			cdromImageName = cdrom.Image.Name
			break
		}
	}

	// If no connected CD-ROM is found, set it to the first CD-ROM image name.
	if cdromImageName == "" {
		cdromImageName = vm.Spec.Hardware.Cdrom[0].Image.Name
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

func CleanupApplyPowerStateChangeTimeAnno(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) bool {

	var (
		oldAnnoTime = oldVM.Annotations[constants.ApplyPowerStateTimeAnnotation]
		newAnnoTime = vm.Annotations[constants.ApplyPowerStateTimeAnnotation]
	)

	if oldAnnoTime != newAnnoTime {
		// VM is updated with a new apply-power-state time from parent, no op.
		return false
	}

	if oldVM.Spec.PowerState != vm.Spec.PowerState {
		// VM's power state is updated directly without a new apply-power-state
		// time set from a parent group, remove the stale annotation to apply
		// the power state immediately.
		if newAnnoTime != "" {
			delete(vm.Annotations, constants.ApplyPowerStateTimeAnnotation)
			return true
		}
	}

	return false
}
