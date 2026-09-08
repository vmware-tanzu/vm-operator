// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/kubevm"
)

// ResolveKubeVMParentOnCreate resolves this VM's configuration from an
// owning kube-vm.io VirtualMachine named by the kube-vm.io/virtual-machine
// annotation.
//
// It returns immediately, doing nothing, unless the KubeVMProvider feature
// gate is on and the annotation is present. When both hold, it Gets the
// named generic object, refuses the request when that object's
// spec.infrastructureRef does not name this VM in this namespace, and
// otherwise fills spec.className, spec.image, spec.storageClass and
// spec.powerState from the generic object, only where the field is
// currently empty, so a value the user set explicitly is never overwritten.
func ResolveKubeVMParentOnCreate(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	if !pkgcfg.FromContext(ctx).Features.KubeVMProvider {
		return false, nil
	}

	genericName, ok := kubevm.GenericObjectName(vm)
	if !ok {
		return false, nil
	}

	generic := &kubevmv1a1.VirtualMachine{}
	if err := client.Get(
		ctx,
		types.NamespacedName{Namespace: vm.Namespace, Name: genericName},
		generic); err != nil {

		if apierrors.IsNotFound(err) {
			return false, fmt.Errorf(
				"generic VirtualMachine %q named by annotation %s not found",
				genericName, kubevm.AnnotationKey)
		}
		return false, fmt.Errorf(
			"failed to get generic VirtualMachine %q: %w", genericName, err)
	}

	ref := generic.Spec.InfrastructureRef
	if ref.APIGroup != vmopv1.GroupName || ref.Kind != "VirtualMachine" || ref.Name != vm.Name {
		return false, fmt.Errorf(
			"generic VirtualMachine %q does not name this VirtualMachine as its infrastructureRef",
			genericName)
	}

	kubevm.ApplyDelegatedFields(vm, generic)

	return true, nil
}
