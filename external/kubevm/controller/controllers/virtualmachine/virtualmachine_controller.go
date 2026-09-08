// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package virtualmachine reconciles a kube-vm.io VirtualMachine by adopting
// the provider object its spec.infrastructureRef names, mirroring that
// object's status onto fixed contract paths, and deleting it when the
// generic object is deleted.
package virtualmachine

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm/controller/internal/contract"
)

const (
	// finalizerName is set on every generic VirtualMachine this controller
	// adopts a provider object for, so the provider object can be deleted
	// before the generic object is allowed to go away.
	finalizerName = "kube-vm.io/virtualmachine"

	// annotationKey is the annotation a provider object carries to declare
	// which generic VirtualMachine adopted it. Mirrors
	// github.com/vmware-tanzu/vm-operator/pkg/kubevm.AnnotationKey, which
	// this module cannot import without depending on the root module.
	annotationKey = "kube-vm.io/virtual-machine"

	// pollRequeueDelay is the delay used to requeue while waiting on state
	// this controller cannot watch directly: a provider object that does
	// not exist yet or does not yet link back, and a provider object that
	// has not yet reported an address. Mirrors the root module's
	// pkgcfg.Default().PoweredOnVMHasIPRequeueDelay, which this module
	// cannot import.
	pollRequeueDelay = 10 * time.Second

	readyConditionType               = "Ready"
	infrastructureReadyConditionType = "InfrastructureReady"
	upToDateConditionType            = "UpToDate"
)

// +kubebuilder:rbac:groups=kube-vm.io,resources=virtualmachines,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=kube-vm.io,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;update;patch;delete

// AddToManager adds this package's controller to the provided manager.
func AddToManager(mgr manager.Manager) error {
	r := &Reconciler{
		Client: mgr.GetClient(),
		Mapper: mgr.GetRESTMapper(),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&kubevmv1a1.VirtualMachine{}).
		Named("kubevm-virtualmachine").
		Complete(r)
}

// Reconciler reconciles a kube-vm.io VirtualMachine.
type Reconciler struct {
	client.Client
	Mapper meta.RESTMapper
}

// Reconcile adopts the provider object named by spec.infrastructureRef once
// linkage is mutual, mirrors its status onto the fixed contract paths, and
// deletes it when the generic object is deleted.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	vm := &kubevmv1a1.VirtualMachine{}
	if err := r.Get(ctx, req.NamespacedName, vm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !vm.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, vm)
	}

	return r.reconcileNormal(ctx, vm)
}

// providerGVK resolves the provider object's GroupVersionKind from
// spec.infrastructureRef, using the RESTMapper's preferred version for the
// named group/kind. The generic object's InfrastructureRef carries no
// version, since the core does not know a provider's served versions ahead
// of time.
func (r *Reconciler) providerGVK(ref kubevmv1a1.ObjectReference) (schema.GroupVersionKind, error) {
	mapping, err := r.Mapper.RESTMapping(schema.GroupKind{Group: ref.APIGroup, Kind: ref.Kind})
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf(
			"failed to resolve preferred version for %s/%s: %w", ref.APIGroup, ref.Kind, err)
	}
	return mapping.GroupVersionKind, nil
}

// getProvider Gets the provider object named by spec.infrastructureRef as
// unstructured, importing no provider types. It never creates the object.
func (r *Reconciler) getProvider(
	ctx context.Context,
	vm *kubevmv1a1.VirtualMachine) (*unstructured.Unstructured, error) {
	gvk, err := r.providerGVK(vm.Spec.InfrastructureRef)
	if err != nil {
		return nil, err
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	if err := r.Get(ctx, types.NamespacedName{
		Namespace: vm.Namespace,
		Name:      vm.Spec.InfrastructureRef.Name,
	}, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// isMutuallyLinked reports whether the provider object's annotation names
// this generic object back. The generic object's own spec.infrastructureRef
// naming the provider object is a given, since that is how the provider
// object was found.
func isMutuallyLinked(vm *kubevmv1a1.VirtualMachine, provider *unstructured.Unstructured) bool {
	return provider.GetAnnotations()[annotationKey] == vm.Name
}

// conflictingOwner returns a non-nil error when the provider object already
// carries a controller owner reference naming a generic VirtualMachine
// other than the given one.
func conflictingOwner(vm *kubevmv1a1.VirtualMachine, provider *unstructured.Unstructured) error {
	for _, ref := range provider.GetOwnerReferences() {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		if ref.APIVersion != kubevmv1a1.GroupVersion.String() || ref.Kind != "VirtualMachine" {
			continue
		}
		if ref.Name != vm.Name {
			return fmt.Errorf(
				"provider object %s/%s is already controlled by generic VirtualMachine %q",
				provider.GetNamespace(), provider.GetName(), ref.Name)
		}
	}
	return nil
}

func (r *Reconciler) reconcileNormal(
	ctx context.Context,
	vm *kubevmv1a1.VirtualMachine) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(vm, finalizerName) {
		base := vm.DeepCopy()
		controllerutil.AddFinalizer(vm, finalizerName)
		if err := r.Patch(ctx, vm, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	provider, err := r.getProvider(ctx, vm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// The provider object may not exist yet: nothing besides this
			// generic object's own changes wakes this reconciler, since a
			// provider's concrete GVK is not known ahead of time and so
			// cannot be statically watched. Requeue until it shows up.
			return ctrl.Result{RequeueAfter: pollRequeueDelay},
				r.setNotAdopted(ctx, vm, "provider object not found")
		}
		return ctrl.Result{}, fmt.Errorf("failed to get provider object: %w", err)
	}

	if !isMutuallyLinked(vm, provider) {
		return ctrl.Result{RequeueAfter: pollRequeueDelay}, r.setNotAdopted(ctx, vm,
			"provider object does not name this generic VirtualMachine back")
	}

	if err := conflictingOwner(vm, provider); err != nil {
		return ctrl.Result{RequeueAfter: pollRequeueDelay}, r.setNotAdopted(ctx, vm, err.Error())
	}

	if err := r.ensureOwnerReference(ctx, vm, provider); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set owner reference: %w", err)
	}

	return r.reconcileStatus(ctx, vm, provider)
}

// ensureOwnerReference sets a controller owner reference on the provider
// object naming the generic object, patched with an optimistic lock and
// skipped when the owner reference list is already correct. Owner
// references are a shared list — a plain merge patch would replace it
// wholesale with no conflict detection against a concurrent writer.
func (r *Reconciler) ensureOwnerReference(
	ctx context.Context,
	vm *kubevmv1a1.VirtualMachine,
	provider *unstructured.Unstructured) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		base := provider.DeepCopy()

		isController := true
		blockOwnerDeletion := true
		ownerRef := metav1.OwnerReference{
			APIVersion:         kubevmv1a1.GroupVersion.String(),
			Kind:               "VirtualMachine",
			Name:               vm.Name,
			UID:                vm.UID,
			Controller:         &isController,
			BlockOwnerDeletion: &blockOwnerDeletion,
		}

		refs := provider.GetOwnerReferences()
		found := false
		for i := range refs {
			if refs[i].UID == ownerRef.UID {
				refs[i] = ownerRef
				found = true
				break
			}
		}
		if !found {
			refs = append(refs, ownerRef)
		}
		provider.SetOwnerReferences(refs)

		if apiequality.Semantic.DeepEqual(base.GetOwnerReferences(), provider.GetOwnerReferences()) {
			return nil
		}

		return r.Patch(ctx, provider, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
	})
}

// setNotAdopted reports that this generic object has no adopted
// infrastructure and waits; it never recreates or writes the provider
// object.
func (r *Reconciler) setNotAdopted(
	ctx context.Context,
	vm *kubevmv1a1.VirtualMachine,
	message string) error {
	base := vm.DeepCopy()

	vm.Status.Ready = false
	setCondition(&vm.Status.Conditions, vm.Generation, readyConditionType,
		metav1.ConditionFalse, "NotAdopted", message)
	setCondition(&vm.Status.Conditions, vm.Generation, infrastructureReadyConditionType,
		metav1.ConditionFalse, "NotAdopted", message)

	if apiequality.Semantic.DeepEqual(base.Status, vm.Status) {
		return nil
	}

	return r.Status().Patch(ctx, vm, client.MergeFrom(base))
}

func (r *Reconciler) reconcileStatus(
	ctx context.Context,
	vm *kubevmv1a1.VirtualMachine,
	provider *unstructured.Unstructured) (ctrl.Result, error) {
	base := vm.DeepCopy()

	status, err := contract.ReadStatus(provider)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to read provider status: %w", err)
	}

	vm.Status.PowerState = kubevmv1a1.PowerState(status.PowerState)
	vm.Status.ProviderID = status.ProviderID
	vm.Status.ProviderMetadata = status.ProviderMetadata
	vm.Status.ObservedGeneration = vm.Generation

	vm.Status.Addresses = nil
	for _, a := range status.Addresses {
		vm.Status.Addresses = append(vm.Status.Addresses, kubevmv1a1.VirtualMachineAddress{
			Interface: a.Interface,
			Type:      kubevmv1a1.VirtualMachineAddressType(a.Type),
			Address:   a.Address,
		})
	}

	ready := status.Ready != nil && *status.Ready == corev1.ConditionTrue
	vm.Status.Ready = ready

	if status.Ready != nil {
		conditionStatus := metav1.ConditionFalse
		if ready {
			conditionStatus = metav1.ConditionTrue
		}
		setCondition(&vm.Status.Conditions, vm.Generation, readyConditionType,
			conditionStatus, orDefault(status.ReadyReason, "Reported"), status.ReadyMessage)
		setCondition(&vm.Status.Conditions, vm.Generation, infrastructureReadyConditionType,
			conditionStatus, orDefault(status.ReadyReason, "Reported"), status.ReadyMessage)
	} else {
		setCondition(&vm.Status.Conditions, vm.Generation, readyConditionType,
			metav1.ConditionUnknown, "Waiting", "provider object has not reported readiness")
		setCondition(&vm.Status.Conditions, vm.Generation, infrastructureReadyConditionType,
			metav1.ConditionUnknown, "Waiting", "provider object has not reported readiness")
	}

	if status.UpToDate != nil {
		conditionStatus := metav1.ConditionFalse
		if *status.UpToDate == corev1.ConditionTrue {
			conditionStatus = metav1.ConditionTrue
		}
		setCondition(&vm.Status.Conditions, vm.Generation, upToDateConditionType,
			conditionStatus, orDefault(status.UpToDateReason, "Reported"), status.UpToDateMessage)
	}

	if !apiequality.Semantic.DeepEqual(base.Status, vm.Status) {
		if err := r.Status().Patch(ctx, vm, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch status: %w", err)
		}
	}

	if !ready && len(vm.Status.Addresses) == 0 {
		return ctrl.Result{RequeueAfter: pollRequeueDelay}, nil
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileDelete(
	ctx context.Context,
	vm *kubevmv1a1.VirtualMachine) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(vm, finalizerName) {
		return ctrl.Result{}, nil
	}

	provider, err := r.getProvider(ctx, vm)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get provider object: %w", err)
		}
		// The provider object is gone; the generic object can go too.
		base := vm.DeepCopy()
		controllerutil.RemoveFinalizer(vm, finalizerName)
		if err := r.Patch(ctx, vm, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if err := r.Delete(ctx, provider); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to delete provider object: %w", err)
	}

	// Requeue and wait for the provider object to actually be gone before
	// dropping the finalizer, so the generic object does not disappear
	// ahead of the backend VM.
	return ctrl.Result{RequeueAfter: time.Second}, nil
}

func setCondition(
	conditions *[]metav1.Condition,
	generation int64,
	condType string,
	status metav1.ConditionStatus,
	reason, message string) {
	newCond := metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: generation,
		Reason:             reason,
		Message:            message,
	}

	for i := range *conditions {
		if (*conditions)[i].Type == condType {
			existing := (*conditions)[i]
			newCond.LastTransitionTime = existing.LastTransitionTime
			if existing.Status != status {
				newCond.LastTransitionTime = metav1.Now()
			}
			(*conditions)[i] = newCond
			return
		}
	}

	newCond.LastTransitionTime = metav1.Now()
	*conditions = append(*conditions, newCond)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
