// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineclass

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/go-logr/logr"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineClass{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			LogConstructor:          pkgutil.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder) *Reconciler {
	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}

// Reconciler reconciles a VirtualMachineClass object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclassinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclassinstances/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	vmClass := &vmopv1.VirtualMachineClass{}
	if err := r.Get(ctx, req.NamespacedName, vmClass); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmClassCtx := &pkgctx.VirtualMachineClassContext{
		Context: ctx,
		Logger:  pkgutil.FromContextOrDefault(ctx),
		VMClass: vmClass,
	}

	patchHelper, err := patch.NewHelper(vmClass, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", vmClassCtx, err)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, vmClass); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmClassCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vmClass.DeletionTimestamp.IsZero() {
		// Noop.
		return ctrl.Result{}, nil
	}

	if err := r.ReconcileNormal(vmClassCtx); err != nil {
		vmClassCtx.Logger.Error(err, "Failed to reconcile VirtualMachineClass")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(vmClassCtx *pkgctx.VirtualMachineClassContext) error {
	// Reconcile VM class instances if the Supervisor supports it.
	if pkgcfg.FromContext(vmClassCtx).Features.ImmutableClasses {
		vmClassCtx.Logger.Info("Reconciling VirtualMachineClassInstances for VM class")
		if err := r.reconcileInstance(vmClassCtx); err != nil {
			return err
		}
	}

	return nil
}

// TODO: copied from pkg/providers/vsphere/vmlifecycle/bootstrap.go.
func getVimTypeHash(obj vimtypes.AnyType) (string, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal vim type to json: %w", err)
	}
	h := xxhash.New()
	if _, err := h.Write(data); err != nil {
		return "", fmt.Errorf("failed to write vim type to hash: %w", err)
	}
	out := h.Sum(nil)
	return fmt.Sprintf("%x", out), nil
}

// vmClassHash returns a hash of the VM Class Reserved + VirtualMachineConfigSpec.
func vmClassHash(ctx *pkgctx.VirtualMachineClassContext, class *vmopv1.VirtualMachineClass) (string, error) {
	if len(class.Spec.ConfigSpec) == 0 {
		// wcpsvc is expected to always set configSpec when creating a VM class.
		return "", fmt.Errorf("%s configSpec is nil", class.Name)
	}

	spec, err := vsphere.GetVMClassConfigSpec(ctx, class.Spec.ConfigSpec)
	if err != nil {
		return "", err
	}

	data := struct {
		Reserved bool                              `json:"reserved"`
		Spec     vimtypes.VirtualMachineConfigSpec `json:"spec"`
	}{
		class.Spec.ReservedProfileID != "",
		spec,
	}

	return getVimTypeHash(data)
}

// reconcileInstance creates a VirtualMachineClassInstance for the
// given VirtualMachineClass. The name of the instance object contains
// the a hash of ConfigSpec (and other relevant fields) to uniquely
// identify a configuration. The hash is generated using a 64-bit
// xxHash algorithm. The instance also contains:
//   - An annotation containing the same hash
//   - A label with the corresponding VM class name to allow for
//     efficient server side queries to list instances for a given VM
//     class.
//   - A label to indicate if an instance is active. An instance is
//     marked inactive if the configuration of the VM class is modified,
//     or the class is removed from the namespace or deleted.  There
//     can only be one active instance of a VM class. So, all other
//     instances are marked inactive.
//   - Owner reference to the owning class.
func (r *Reconciler) reconcileInstance(ctx *pkgctx.VirtualMachineClassContext) error {
	hash, err := vmClassHash(ctx, ctx.VMClass)
	if err != nil {
		ctx.Logger.Error(err, "Failed to calculate the hash value required to create VM class instance")
		return err
	}

	vmClassInstance := &vmopv1.VirtualMachineClassInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ctx.VMClass.Namespace,
			Name:      ctx.VMClass.Name + "-" + hash,
		},
	}

	// Note that there's a window where users will be able to specify
	// a stale VM class instance to create a VM if the call comes in
	// after we create a new instance but before we mark the existing
	// active one as "inactive". The alternative is to flip the order
	// but that will lead to rejection of VM creations even though
	// there's a valid class associated with the namespace. So we
	// choose former.
	_, err = ctrlutil.CreateOrPatch(ctx, r.Client, vmClassInstance, func() error {
		if err := ctrlutil.SetControllerReference(
			ctx.VMClass, vmClassInstance, r.Scheme()); err != nil {

			return err
		}

		vmClassInstance.Spec.VirtualMachineClassSpec = ctx.VMClass.Spec

		// Set the hash as an annotation on the instance.
		if vmClassInstance.Annotations == nil {
			vmClassInstance.Annotations = make(map[string]string)
		}
		vmClassInstance.Annotations[pkgconst.VirtualMachineClassHashAnnotationKey] = hash

		// Set a label to mark the instance as active.
		if vmClassInstance.Labels == nil {
			vmClassInstance.Labels = make(map[string]string)
		}
		vmClassInstance.Labels[vmopv1.VMClassInstanceActiveLabelKey] = ""

		return nil
	})
	if err != nil {
		return err
	}

	var list vmopv1.VirtualMachineClassInstanceList

	if err := r.Client.List(ctx, &list,
		client.InNamespace(ctx.VMClass.Namespace)); err != nil {

		ctx.Logger.Error(err, "Error listing instances for VM class")
		return err
	}

	for _, instance := range list.Items {
		// Only process instances owned by this VM class
		owned, err := ctrlutil.HasOwnerReference(instance.OwnerReferences, ctx.VMClass, r.Scheme())
		if err != nil {
			return fmt.Errorf("failed to check if VM class instance is owned by the class. err: %w", err)
		}

		if !owned {
			ctx.Logger.Info("Skipping this instance since it is not owned by VM class")
			continue
		}

		// Create a deepcopy of the object for patching.
		patch := client.MergeFrom(instance.DeepCopy())

		if val, ok := instance.Annotations[pkgconst.VirtualMachineClassHashAnnotationKey]; ok && val == hash {
			// Hash value on the VM class instance matches with the
			// hash we just calculated. Mark this class as active.
			if _, ok := instance.Labels[vmopv1.VMClassInstanceActiveLabelKey]; !ok {
				ctx.Logger.Info("Marking instance as active")
				if instance.Labels == nil {
					instance.Labels = make(map[string]string)
				}
				instance.Labels[vmopv1.VMClassInstanceActiveLabelKey] = ""
			}
		} else {
			// Mark the instance as inactive since its hash does not
			// match the hash of the current version of VM class.
			ctx.Logger.Info("Marking instance as inactive")
			delete(instance.Labels, vmopv1.VMClassInstanceActiveLabelKey)
			// If this was the last label, set labels to nil so the
			// field gets removed from the object.
		}

		if err := r.Client.Patch(ctx, &instance, patch); err != nil {
			ctx.Logger.Error(err, "patch failed")
			return err
		}
	}

	return nil
}
