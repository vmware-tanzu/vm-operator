// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// newExternalNetworkInterfaceObject returns an empty object for the network
// interface CR type of the interface's ExternalRef API group. The VM validation
// webhook enforces that the ExternalRef's Kind is the network interface Kind of
// the group's provider.
func newExternalNetworkInterfaceObject(group string) (ctrlclient.Object, error) {
	switch group {
	case netopv1alpha1.SchemeGroupVersion.Group:
		return &netopv1alpha1.NetworkInterface{}, nil
	case ncpv1alpha1.SchemeGroupVersion.Group:
		return &ncpv1alpha1.VirtualNetworkInterface{}, nil
	case vpcv1alpha1.SchemeGroupVersion.Group:
		return &vpcv1alpha1.SubnetPort{}, nil
	default:
		return nil, fmt.Errorf("unsupported external network interface API group %q", group)
	}
}

// getExternalNetworkInterface gets the pre-created network interface CR
// referenced by the interface's ExternalRef. VM Operator does not create or
// delete this CR. The CR is marked as external and associated with the VM.
func getExternalNetworkInterface(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	client ctrlclient.Client,
	group string,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec) (ctrlclient.Object, error) {

	if !pkgcfg.FromContext(ctx).Features.BringYourOwnNetworkInterface {
		return nil, fmt.Errorf("external network interfaces are not enabled")
	}

	obj, err := newExternalNetworkInterfaceObject(group)
	if err != nil {
		return nil, err
	}

	key := ctrlclient.ObjectKey{Namespace: vm.Namespace, Name: interfaceSpec.ExternalRef.Name}
	if err := client.Get(ctx, key, obj); err != nil {
		return nil, fmt.Errorf("failed to get external network interface %s: %w", key.Name, err)
	}

	if !obj.GetDeletionTimestamp().IsZero() {
		return nil, fmt.Errorf("external network interface %s is being deleted", key.Name)
	}

	if err := associateExternalNetworkInterface(ctx, vm, client, obj, interfaceSpec.Name); err != nil {
		return nil, err
	}

	return obj, nil
}

func getExternalNetworkInterfaceDevice(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	clusterMoRef *vimtypes.ManagedObjectReference,
	group string,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec) (Device, error) {

	obj, err := getExternalNetworkInterface(vmCtx, vmCtx.VM, client, group, interfaceSpec)
	if err != nil {
		return Device{}, err
	}

	switch netIf := obj.(type) {
	case *netopv1alpha1.NetworkInterface:
		return getNetOPNetworkInterfaceDevice(vmCtx, vimClient, netIf, clusterMoRef, interfaceSpec)
	case *ncpv1alpha1.VirtualNetworkInterface:
		return getNCPNetworkInterfaceDevice(vmCtx, vimClient, clusterMoRef, netIf, interfaceSpec)
	case *vpcv1alpha1.SubnetPort:
		return getVPCSubnetPortDevice(vmCtx, vimClient, clusterMoRef, netIf, interfaceSpec)
	default:
		return Device{}, fmt.Errorf("unsupported network interface CR type: %T", obj)
	}
}

func vpcAttachmentRef(vm *vmopv1.VirtualMachine, interfaceName string) string {
	return "virtualmachine/" + vm.Name + "/" + interfaceName
}

// associateExternalNetworkInterface marks an external network interface CR as
// such and associates it with the VM's interface by setting the VM association
// labels. For VPC, the attachment annotation required by NSX is also set on the
// SubnetPort.
func associateExternalNetworkInterface(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	client ctrlclient.Client,
	obj ctrlclient.Object,
	interfaceName string) error {

	if vmName := obj.GetLabels()[pkgconst.NetworkInterfaceVMNameLabel]; vmName != "" && vmName != vm.Name {
		return fmt.Errorf("external network interface %s is associated with VM %s", obj.GetName(), vmName)
	}

	// A CR owned by this VM, for example after an interface is switched from
	// managed to external, keeps that ownership. A CR owned by a different VM
	// is rejected since it is deleted along with that VM.
	if owner := otherVMOwner(vm, obj); owner != "" {
		return fmt.Errorf("external network interface %s is owned by VM %s", obj.GetName(), owner)
	}

	subnetPort, _ := obj.(*vpcv1alpha1.SubnetPort)

	var attachmentRef string
	if subnetPort != nil {
		attachmentRef = vpcAttachmentRef(vm, interfaceName)
		if ref := subnetPort.Annotations[pkgconst.VPCAttachmentRef]; ref != "" &&
			!strings.HasPrefix(ref, vpcAttachmentRef(vm, "")) {
			return fmt.Errorf("external SubnetPort %s is attached to %s", subnetPort.Name, ref)
		}
	}

	labels, annotations := obj.GetLabels(), obj.GetAnnotations()

	if labels[pkgconst.NetworkInterfaceVMNameLabel] == vm.Name &&
		labels[VMInterfaceNameLabel] == interfaceName &&
		IsExternalNetworkInterface(obj) &&
		(subnetPort == nil || annotations[pkgconst.VPCAttachmentRef] == attachmentRef) {
		return nil
	}

	base := obj.DeepCopyObject().(ctrlclient.Object) //nolint:forcetypeassert
	if labels == nil {
		labels = map[string]string{}
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	labels[pkgconst.NetworkInterfaceVMNameLabel] = vm.Name
	labels[VMInterfaceNameLabel] = interfaceName
	annotations[pkgconst.ExternalNetworkInterfaceAnnotation] = "true"
	if subnetPort != nil {
		annotations[pkgconst.VPCAttachmentRef] = attachmentRef
	}
	obj.SetLabels(labels)
	obj.SetAnnotations(annotations)

	// Use an optimistic lock so that if another VM concurrently associates the
	// same CR, one of the patches fails with a conflict instead of silently
	// overwriting the other. The retry then observes the other VM's label.
	if err := client.Patch(
		ctx,
		obj,
		ctrlclient.MergeFromWithOptions(base, ctrlclient.MergeFromWithOptimisticLock{})); err != nil {

		return fmt.Errorf("failed to associate external network interface %s: %w", obj.GetName(), err)
	}

	return nil
}

// otherVMOwner returns the name of a VirtualMachine other than vm that owns
// obj, or an empty string if there is none.
func otherVMOwner(vm *vmopv1.VirtualMachine, obj ctrlclient.Object) string {
	for _, ref := range obj.GetOwnerReferences() {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			continue
		}
		if gv.Group == vmopv1.GroupVersion.Group &&
			ref.Kind == "VirtualMachine" &&
			ref.UID != vm.UID {

			return ref.Name
		}
	}
	return ""
}

// IsExternalNetworkInterface returns true if the network interface CR is marked
// as an external network interface. Those are never deleted by VM Operator,
// regardless of their owner references.
func IsExternalNetworkInterface(obj ctrlclient.Object) bool {
	_, ok := obj.GetAnnotations()[pkgconst.ExternalNetworkInterfaceAnnotation]
	return ok
}

// ReleaseExternalNetworkInterface removes the VM association metadata from an
// external network interface CR. It is a no-op if there is nothing to remove.
func ReleaseExternalNetworkInterface(
	ctx context.Context,
	client ctrlclient.Client,
	obj ctrlclient.Object) error {

	labelKeys := []string{pkgconst.NetworkInterfaceVMNameLabel, VMInterfaceNameLabel}
	annotationKeys := []string{pkgconst.VPCAttachmentRef, pkgconst.ExternalNetworkInterfaceAnnotation}

	labels, annotations := obj.GetLabels(), obj.GetAnnotations()

	var found bool
	for _, k := range labelKeys {
		if _, ok := labels[k]; ok {
			found = true
		}
	}
	for _, k := range annotationKeys {
		if _, ok := annotations[k]; ok {
			found = true
		}
	}
	if !found {
		return nil
	}

	base := obj.DeepCopyObject().(ctrlclient.Object) //nolint:forcetypeassert
	for _, k := range labelKeys {
		delete(labels, k)
	}
	for _, k := range annotationKeys {
		delete(annotations, k)
	}

	// Use an optimistic lock so a release based on a stale read cannot strip
	// markers that another VM has since set.
	if err := client.Patch(
		ctx,
		obj,
		ctrlclient.MergeFromWithOptions(base, ctrlclient.MergeFromWithOptimisticLock{})); err != nil {

		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to release external network interface %s: %w", obj.GetName(), err)
	}

	return nil
}

// ReleaseAllExternalNetworkInterfaces removes the VM association metadata from
// all external network interface CRs associated with the VM. This is used once
// the VM has been deleted. It is a no-op when the BringYourOwnNetworkInterface
// capability is disabled.
func ReleaseAllExternalNetworkInterfaces(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	client ctrlclient.Client) error {

	if !pkgcfg.FromContext(ctx).Features.BringYourOwnNetworkInterface {
		return nil
	}

	objs, err := listExternalNetworkInterfaces(ctx, client, vm)
	if err != nil {
		return fmt.Errorf("failed to list external network interfaces: %w", err)
	}

	var errs []error
	for _, obj := range objs {
		if err := ReleaseExternalNetworkInterface(ctx, client, obj); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
