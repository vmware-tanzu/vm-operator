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

	xxhash "github.com/cespare/xxhash/v2"
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
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
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
		Logger:  ctrl.Log.WithName("VirtualMachineClass").WithValues("name", req.Name),
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
	if pkgcfg.FromContext(vmClassCtx).Features.ImmutableClasses {
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

// reconcileInstance creates a VirtualMachineClassInstance of the given VirtualMachineClass.
// The Instance is named VMClass Name + VMClass Hash and is:
// - Annotated with the VMClass Hash
// - Labeled with VMClass Name
// - Labeled with VMClass State "active"
// - Owner ref set to the VMClass
// All other Instances of this VM class are labeled "inactive".
func (r *Reconciler) reconcileInstance(ctx *pkgctx.VirtualMachineClassContext) error {
	hash, err := vmClassHash(ctx, ctx.VMClass)
	if err != nil {
		return err
	}

	vmClassInstance := &vmopv1.VirtualMachineClassInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ctx.VMClass.Namespace,
			Name:      ctx.VMClass.Name + "-" + hash,
		},
	}

	_, err = ctrlutil.CreateOrPatch(ctx, r.Client, vmClassInstance, func() error {
		if err := ctrlutil.SetControllerReference(
			ctx.VMClass, vmClassInstance, r.Scheme()); err != nil {

			return err
		}

		vmClassInstance.Spec.VirtualMachineClassSpec = ctx.VMClass.Spec

		vmClassInstance.Annotations = map[string]string{
			pkgconst.VirtualMachineClassHashAnnotationKey: hash,
		}

		vmClassInstance.Labels = map[string]string{
			pkgconst.VirtualMachineClassNameLabelKey:  ctx.VMClass.Name,
			pkgconst.VirtualMachineClassStateLabelKey: pkgconst.VirtualMachineClassStateActive,
		}

		return nil
	})
	if err != nil {
		return err
	}

	var list vmopv1.VirtualMachineClassInstanceList

	err = r.Client.List(ctx, &list,
		client.InNamespace(ctx.VMClass.Namespace),
		client.MatchingLabels(vmClassInstance.Labels))
	if err != nil {
		return err
	}

	for i := range list.Items {
		instance := &list.Items[i]

		if instance.Name == vmClassInstance.Name {
			continue
		}

		patchHelper, err := patch.NewHelper(instance, r.Client)
		if err != nil {
			return fmt.Errorf("failed to init patch helper for %s: %w", instance.Name, err)
		}

		instance.Labels[pkgconst.VirtualMachineClassStateLabelKey] = pkgconst.VirtualMachineClassStateInactive

		if err := patchHelper.Patch(ctx, instance); err != nil {
			ctx.Logger.Error(err, "patch failed")
			return err
		}
	}

	return nil
}
