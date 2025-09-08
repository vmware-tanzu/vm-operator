// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"fmt"
	"maps"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// getPolicyEvaluationResults evaluates the vsphere.policy.vmware.com resources
// that apply to the provided VM and returns the results or
// PolicyEvaluationNotReadyError when the evaluation is not yet complete.
func getPolicyEvaluationResults(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec vimtypes.VirtualMachineConfigSpec) ([]vspherepolv1.PolicyEvaluationResult, error) {

	logger := pkglog.FromContextOrDefault(ctx).
		WithName("getPolicyEvaluationResults")

	// Ensure the image criteria is considered.
	var (
		imageName   string
		imageLabels map[string]string
	)
	if vm.Spec.Image != nil {
		img, err := vmopv1util.GetImage(
			ctx, k8sClient, *vm.Spec.Image, vm.Namespace)
		if err != nil {
			if moVM.Config != nil && apierrors.IsNotFound(err) {
				// Do not return an exception if this is an existing VM whose
				// image no longer exists.
				logger.V(4).Info("Cannot find image for existing VM")
			} else {
				return nil, fmt.Errorf(
					"failed to get VM %[1]s/%[2]s image %[1]s/%[3]s: %[4]w",
					vm.Namespace,
					vm.Name,
					vm.Spec.Image.Name,
					err)
			}
		} else {
			imageName = img.Name
			if len(img.Labels) > 0 {
				imageLabels = maps.Clone(img.Labels)
			}
		}
	}

	// Ensure the workload criteria is considered.
	var (
		guestID        string
		guestFamily    vspherepolv1.GuestFamilyType
		workloadLabels map[string]string
	)
	if len(vm.Labels) > 0 {
		workloadLabels = maps.Clone(vm.Labels)
	}

	switch {
	case moVM.Config != nil:
		//
		// Existing VM.
		//
		guestID = moVM.Guest.GuestId
		guestFamily = vspherepolv1.FromVimGuestFamily(moVM.Guest.GuestFamily)
	case moVM.Config == nil:
		//
		// New VM.
		//
		guestID = configSpec.GuestId
	}

	// If the guestFamily could not be detected, then discover it from the
	// guestID.
	if guestFamily == "" && guestID != "" {
		vimGuestID := vimtypes.VirtualMachineGuestOsIdentifier(guestID)
		vimGuestFamily := vimGuestID.ToFamily()
		guestFamily = vspherepolv1.FromVimGuestFamily(string(vimGuestFamily))
	}

	// Ensure the explicit policies are considered.
	var (
		explicitPolicies []vspherepolv1.LocalObjectRef
	)
	if vmp := vm.Spec.Policies; len(vmp) > 0 {
		explicitPolicies = make([]vspherepolv1.LocalObjectRef, len(vmp))
		for i := range vmp {
			explicitPolicies[i].APIVersion = vmp[i].APIVersion
			explicitPolicies[i].Kind = vmp[i].Kind
			explicitPolicies[i].Name = vmp[i].Name
		}
	}

	logger.Info("Got PolicyEvaluation criteria info",
		"guestID", guestID,
		"guestFamily", guestFamily,
		"imageName", imageName,
		"imageLabels", imageLabels,
		"workloadLabels", workloadLabels)

	obj := vspherepolv1.PolicyEvaluation{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: vm.Namespace,
			Name:      "vm-" + vm.Name,
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, k8sClient, &obj, func() error {

		// Ensure the PolicyEvaluation object is owned by the VM.
		if err := controllerutil.SetControllerReference(
			vm,
			&obj,
			k8sClient.Scheme()); err != nil {

			return fmt.Errorf(
				"failed to mark VM %[1]s/%[2]s as owner "+
					"of PolicyEvaluation %[1]s/%[3]s: %[4]w",
				vm.Namespace,
				vm.Name,
				obj.Name,
				err)
		}

		// Ensure explicit policies are evaluated.
		if vmp := vm.Spec.Policies; len(vmp) == 0 {
			obj.Spec.Policies = nil
		} else {
			obj.Spec.Policies = explicitPolicies
		}

		// Ensure the image criteria is considered.
		if imageName == "" && len(imageLabels) == 0 {
			obj.Spec.Image = nil
		} else {
			obj.Spec.Image = &vspherepolv1.PolicyEvaluationImageSpec{
				Name:   imageName,
				Labels: imageLabels,
			}
		}

		// Ensure the workload criteria is considered.
		if guestID == "" && guestFamily == "" && len(workloadLabels) == 0 {
			obj.Spec.Workload = nil
		} else {
			obj.Spec.Workload = &vspherepolv1.PolicyEvaluationWorkloadSpec{
				Labels: workloadLabels,
				Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
					GuestID:     guestID,
					GuestFamily: guestFamily,
				},
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf(
			"failed to create or patch VM %[1]s/%[2]s "+
				"PolicyEvaluation %[1]s/%[3]s: %[4]w",
			vm.Namespace,
			vm.Name,
			obj.Name,
			err)
	}

	waiting := func(reason any) error {
		logger.Info("Waiting on PolicyEvaluation object",
			"reason", reason,
			"generation", obj.Generation,
			"observedGeneration", obj.Status.ObservedGeneration,
			"spec", obj.Spec)

		// The object is still being processed.
		return fmt.Errorf(
			"VM %[1]s/%[2]s "+
				"PolicyEvaluation %[1]s/vm-%[2]s still being evaluated: %[3]w",
			vm.Namespace,
			vm.Name,
			ErrPolicyNotReady)

	}

	switch result {
	case controllerutil.OperationResultCreated,
		controllerutil.OperationResultUpdated:

		return nil, waiting(result)

	case controllerutil.OperationResultNone:

		if obj.Generation != obj.Status.ObservedGeneration {
			return nil, waiting("generation")
		}

		return obj.Status.Policies, nil
	}

	return nil, fmt.Errorf(
		"invalid result %[1]s updating to get VM %[2]s/%[3]s "+
			"PolicyEvaluation %[2]s/vm-%[3]s: %[4]w",
		result,
		vm.Namespace,
		vm.Name,
		err)
}

// getTagsFromPolicyEvaluationResults returns the list of vSphere tag UUIDs
// from one of more PolicyEvaluationResult objects.
func getTagsFromPolicyEvaluationResults(
	results ...vspherepolv1.PolicyEvaluationResult) []string {

	active := sets.Set[string]{}

	for _, r := range results {
		for _, t := range r.Tags {
			active.Insert(t)
		}
	}

	return active.UnsortedList()
}
