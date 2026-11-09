// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineinframigration

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	mobilityv1a4 "github.com/vmware-tanzu/vm-operator/external/mobility-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// migrationKind is the kind of the VirtualMachineInfraMigration that is
// referred to in the message of every condition mirrored from it.
const migrationKind = "VirtualMachineInfraMigration"

// volumeAttributesClassMismatchReason is the reason of the event that is
// emitted when the VolumeAttributesClass of a migration's home storage policy
// does not match the one in the VirtualMachine's spec.
const volumeAttributesClassMismatchReason = "VolumeAttributesClassMismatch"

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &mobilityv1a4.VirtualMachineInfraMigration{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorder(controllerNameShort)))

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		// A VirtualMachineInfraMigration may be reconciled before its
		// VirtualMachine exists or after the VirtualMachine's status was
		// last written, so the VirtualMachine is watched as well.
		Watches(
			&vmopv1.VirtualMachine{},
			handler.EnqueueRequestsFromMapFunc(
				vmToVirtualMachineInfraMigrationMapper(mgr.GetClient()))).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.GetMaxConcurrentReconciles(controllerNameShort, 0),
			LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

// vmToVirtualMachineInfraMigrationMapper returns a mapper function that maps a
// VirtualMachine to the VirtualMachineInfraMigration with the same namespace
// and name. A VirtualMachine is only mapped if it has a migration.
func vmToVirtualMachineInfraMigrationMapper(
	c client.Client) handler.MapFunc {

	return func(ctx context.Context, o client.Object) []reconcile.Request {
		key := client.ObjectKeyFromObject(o)

		// This is a cache read and avoids reconciling every VirtualMachine
		// event.
		if err := c.Get(
			ctx,
			key,
			&mobilityv1a4.VirtualMachineInfraMigration{}); err != nil {

			return nil
		}

		return []reconcile.Request{{NamespacedName: key}}
	}
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

// Reconciler reconciles a VirtualMachineInfraMigration object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=mobility-operator.vmware.com,resources=virtualmachineinframigrations,verbs=get;list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=volumeattributesclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (_ ctrl.Result, reterr error) {

	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj mobilityv1a4.VirtualMachineInfraMigration
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, r.ReconcileNormal(ctx, &obj)
}

// ReconcileNormal mirrors the conditions of a VirtualMachineInfraMigration onto
// the VirtualMachine with the same namespace and name. Every condition of the
// VirtualMachineInfraMigration that describes the migration's current
// generation is mirrored to the VirtualMachine's status, whatever its status,
// as described by VirtualMachineConditionInfraMigrationPrefix. A condition that
// describes a previous generation is not mirrored, and the VirtualMachine does
// not keep the one it mirrored earlier.
//
// Only when the VirtualMachineInfraMigration's Ready condition is True for the
// migration's current generation is the VolumeAttributesClass that the
// VirtualMachine was migrated to recorded in status.volumeAttributesClassName.
func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	obj *mobilityv1a4.VirtualMachineInfraMigration) error {

	logger := pkglog.FromContextOrDefault(ctx)

	var vm vmopv1.VirtualMachine
	if err := r.Get(ctx, client.ObjectKeyFromObject(obj), &vm); err != nil {
		if apierrors.IsNotFound(err) {
			// The VirtualMachine's creation will trigger another reconcile.
			logger.V(4).Info("No virtual machine for infra migration")
			return nil
		}
		return fmt.Errorf(
			"failed to get virtual machine %s/%s: %w",
			obj.Namespace, obj.Name, err)
	}

	if !vm.DeletionTimestamp.IsZero() {
		return nil
	}

	var vacName string
	// A Ready condition whose observedGeneration is less than the
	// metadata.generation describes the previous spec, so it is not considered
	// until the current spec has been observed.
	if conditions.IsReady(obj, conditions.WithGeneration(obj.Generation)) {
		var err error
		vacName, err = r.getMigratedVAC(ctx, obj, &vm)
		if err != nil {
			return err
		}
	} else {
		logger.V(4).Info("Infra migration is not ready")
	}

	// A condition that is behind the migration's generation describes the
	// previous spec, so mirroring it would make the VirtualMachine briefly
	// report an outcome that is out of date.
	current := conditions.GetObservedAt(obj, obj.Generation)
	if len(current) != len(obj.Status.Conditions) {
		logger.V(4).Info(
			"Not mirroring infra migration conditions that describe a " +
				"previous generation")
	}

	// The conditions that the VirtualMachine reports are owned by this
	// controller, so they are the ones this patch may overwrite or remove.
	owned := mirroredVMIMConditionTypes(&vm)
	for _, c := range current {
		if t := prefixedConditionType(c.Type); !slices.Contains(owned, t) {
			owned = append(owned, t)
		}
	}

	if err := r.patchVirtualMachine(ctx, &vm, owned, func() {
		mirrorConditions(&vm, obj, current)

		if vacName != "" {
			vm.Status.VolumeAttributesClassName = vacName
		}
	}); err != nil {
		return err
	}

	logger.Info("Mirrored infra migration conditions to virtual machine",
		"namespace", vm.Namespace,
		"name", vm.Name)

	return nil
}

// prefixedConditionType returns the type of the VirtualMachine condition that
// mirrors the VirtualMachineInfraMigration condition with the provided type.
func prefixedConditionType(migrationConditionType string) string {
	return vmopv1.VirtualMachineConditionInfraMigrationPrefix +
		migrationConditionType
}

// mirroredVMIMConditionTypes returns the types of the VirtualMachine's
// conditions that mirror a VirtualMachineInfraMigration condition.
func mirroredVMIMConditionTypes(vm *vmopv1.VirtualMachine) []string {
	var types []string
	for _, c := range vm.Status.Conditions {
		if strings.HasPrefix(
			c.Type,
			vmopv1.VirtualMachineConditionInfraMigrationPrefix) {

			types = append(types, c.Type)
		}
	}
	return types
}

// addSuffixOnMsg returns the
// message of the VirtualMachineInfraMigration's condition followed by a
// reference to the VirtualMachineInfraMigration, in the form
// "<message> @ <kind>/<name>", so that a user reading the VirtualMachine can
// tell which resource the condition was mirrored from. If the condition has no
// message, the message is only the reference.
func addSuffixOnMsg(
	obj *mobilityv1a4.VirtualMachineInfraMigration,
	message string) string {

	ref := fmt.Sprintf("%s/%s", migrationKind, obj.Name)
	if message == "" {
		return ref
	}
	return fmt.Sprintf("%s @ %s", message, ref)
}

// mirrorConditions sets a condition on the VirtualMachine for each of the
// VirtualMachineInfraMigration's current conditions, and removes the conditions
// that mirror a condition that is not among them, either because the
// VirtualMachineInfraMigration no longer reports it or because it describes a
// previous generation.
//
// The status and reason are copied, and so is the message, which is followed by
// a reference to the VirtualMachineInfraMigration. The observedGeneration is
// not, since it refers to the generation of the VirtualMachineInfraMigration
// rather than the VirtualMachine, and the last transition time is only moved
// when one of the copied fields changes.
func mirrorConditions(
	vm *vmopv1.VirtualMachine,
	obj *mobilityv1a4.VirtualMachineInfraMigration,
	current []metav1.Condition) {

	mirrored := make(map[string]struct{}, len(current))

	for _, c := range current {
		t := prefixedConditionType(c.Type)
		mirrored[t] = struct{}{}

		// The reason of a condition must not be empty.
		reason := c.Reason
		if reason == "" {
			reason = string(c.Status)
		}

		conditions.Set(vm, &metav1.Condition{
			Type:    t,
			Status:  c.Status,
			Reason:  reason,
			Message: addSuffixOnMsg(obj, c.Message),
		})
	}

	for _, t := range mirroredVMIMConditionTypes(vm) {
		if _, ok := mirrored[t]; !ok {
			conditions.Delete(vm, t)
		}
	}
}

// getMigratedVAC returns the name of the
// VolumeAttributesClass that the VirtualMachine was migrated to, or an empty
// string if it cannot be determined.
//
// The migration reports the storage policy ID of the VirtualMachine's home
// directory rather than a VolumeAttributesClass name, so the name is looked up
// from the VolumeAttributesClass objects on the cluster that refer to that
// policy. The name is only used if it is also the VolumeAttributesClass in the
// VirtualMachine's spec. Otherwise the migration does not describe what the
// VirtualMachine currently requests, so nothing is recorded and a warning
// event is emitted instead.
func (r *Reconciler) getMigratedVAC(
	ctx context.Context,
	obj *mobilityv1a4.VirtualMachineInfraMigration,
	vm *vmopv1.VirtualMachine) (string, error) {

	ts := obj.Spec.TargetStorage
	if ts == nil || ts.HomeStoragePolicyID == "" {
		// The migration does not change the home storage policy.
		return "", nil
	}

	names, err := kubeutil.GetVACNamesForStoragePolicyID(
		ctx, r.Client, ts.HomeStoragePolicyID)
	if err != nil {
		return "", err
	}

	specName := vm.Spec.VolumeAttributesClassName
	if specName == "" || !slices.Contains(names, specName) {
		r.Recorder.Warnf(
			vm,
			volumeAttributesClassMismatchReason,
			"Infra migration home storage policy %s is referenced by "+
				"volume attributes classes %v, not by "+
				"spec.volumeAttributesClassName %q",
			ts.HomeStoragePolicyID, names, specName)
		return "", nil
	}

	return specName, nil
}

// patchVirtualMachine applies mutate to the VirtualMachine's status and
// patches the result. The patch helper only writes the changed conditions
// against the latest version of the VirtualMachine, retrying on conflict, so
// this does not clobber the conditions that the VirtualMachine controller
// owns. The conditions in owned are the ones this controller owns, so they are
// written even if they changed in the meantime.
func (r *Reconciler) patchVirtualMachine(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	owned []string,
	mutate func()) error {

	patchHelper, err := patch.NewHelper(vm, r.Client)
	if err != nil {
		return fmt.Errorf(
			"failed to init patch helper for virtual machine %s/%s: %w",
			vm.Namespace, vm.Name, err)
	}

	mutate()

	if err := patchHelper.Patch(
		ctx,
		vm,
		patch.WithOwnedConditions{Conditions: owned}); err != nil {

		return fmt.Errorf(
			"failed to patch virtual machine %s/%s: %w",
			vm.Namespace, vm.Name, err)
	}

	return nil
}
