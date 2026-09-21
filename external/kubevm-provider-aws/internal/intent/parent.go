// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package intent

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	awsv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/api/v1alpha1"
)

// AnnotationKey is the back-reference the core requires on a provider object.
//
// Repeated as a literal because the core does not export it -- it is an
// unexported constant at virtualmachine_controller.go:45. Everywhere else this
// provider takes contract strings from the core's package rather than
// retyping them; here there is nothing to take.
//
// The USER writes it, never this provider. It is the provider object's consent
// to be claimed, and a controller that wrote it on the user's behalf would turn
// every one-sided reference into adoption -- the exact thing the contract's
// two-sided linkage exists to prevent. The vSphere provider works the same
// way: nothing in VM Operator writes this key.
const AnnotationKey = "kube-vm.io/virtual-machine"

// ErrNotLinked says the provider object carries no back-reference annotation.
var ErrNotLinked = errors.New("no " + AnnotationKey + " annotation")

// ErrParentMissing says the annotation names a VirtualMachine that does not
// exist yet.
var ErrParentMissing = errors.New("the VirtualMachine it names does not exist")

// ErrNotMutual says the named VirtualMachine does not name this object back.
var ErrNotMutual = errors.New(
	"the VirtualMachine it names does not point back at it")

// ParentOf returns the VirtualMachine this AWSMachine has consented to be
// claimed by, or an error saying why there is none.
//
// A lookup, not a search: the annotation names the parent, and it is fetched
// by name. Then the link is checked from the other side, because consent given
// to a VirtualMachine that never claimed this object is not a link either.
// This is the vSphere provider's rule (virtualmachine_mutator_kubevm.go),
// applied in the controller rather than in an admission webhook.
func ParentOf(ctx context.Context, c client.Reader,
	machine *awsv1a1.AWSMachine) (*kubevmv1a1.VirtualMachine, error) {

	name := machine.GetAnnotations()[AnnotationKey]
	if name == "" {
		return nil, ErrNotLinked
	}

	vm := &kubevmv1a1.VirtualMachine{}
	key := client.ObjectKey{Namespace: machine.Namespace, Name: name}
	if err := c.Get(ctx, key, vm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %q", ErrParentMissing, name)
		}
		return nil, fmt.Errorf("reading VirtualMachine %q: %w", name, err)
	}

	ref := vm.Spec.InfrastructureRef
	if ref.APIGroup != awsv1a1.GroupName || ref.Kind != "AWSMachine" ||
		ref.Name != machine.Name {
		return nil, fmt.Errorf("%w: %q", ErrNotMutual, name)
	}
	return vm, nil
}
