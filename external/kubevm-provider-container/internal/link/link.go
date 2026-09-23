// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package link resolves the two-sided adoption check every KubeVM provider
// must perform on its own, independently of the core: the core only sets an
// owner reference AFTER it has confirmed a mutual link, and it does not watch
// this provider's GVK, so this object's own controller can run many reconciles
// before that owner reference ever lands. Trusting an absent owner reference
// to mean "not adopted yet" would make a freshly-linked object sit idle for up
// to the core's sync period; checking the link directly here does not.
package link

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	containerv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/api/v1alpha1"
)

// ErrNotLinked means the object carries no back-reference annotation at all.
var ErrNotLinked = errors.New("no back-reference annotation")

// ErrParentMissing means the annotation names a VirtualMachine that does not
// exist (yet, or ever).
var ErrParentMissing = errors.New("named VirtualMachine does not exist")

// ErrNotMutual means the named VirtualMachine exists but does not name this
// object back in spec.infrastructureRef.
var ErrNotMutual = errors.New("VirtualMachine does not name this object")

// ParentOf returns the VirtualMachine this object is mutually linked to, or
// one of the sentinel errors above.
func ParentOf(ctx context.Context, c client.Client, obj client.Object) (
	*kubevmv1a1.VirtualMachine, error) {

	name := obj.GetAnnotations()[containerv1a1.AnnotationKey]
	if name == "" {
		return nil, ErrNotLinked
	}

	vm := &kubevmv1a1.VirtualMachine{}
	key := client.ObjectKey{Namespace: obj.GetNamespace(), Name: name}
	if err := c.Get(ctx, key, vm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrParentMissing
		}
		return nil, fmt.Errorf("getting VirtualMachine %q: %w", name, err)
	}

	ref := vm.Spec.InfrastructureRef
	if ref.APIGroup != containerv1a1.GroupName || ref.Kind != "ContainerMachine" ||
		ref.Name != obj.GetName() {
		return nil, ErrNotMutual
	}
	return vm, nil
}
