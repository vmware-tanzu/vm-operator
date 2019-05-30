/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"

	"k8s.io/klog"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/sequence"

	"github.com/pkg/errors"

	"github.com/vmware/govmomi/find"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	VsphereVmProviderName string = "vsphere"

	// Annotation Key for vSphere VC Id.
	VmOperatorVcUuidKey = pkg.VmOperatorKey + "/vcuuid"

	// Annotation Key for vSphere MoRef
	VmOperatorMoRefKey = pkg.VmOperatorKey + "/moref"
)

type VSphereVmProvider struct {
	sessions SessionManager
}

var _ vmprovider.VirtualMachineProviderInterface = &VSphereVmProvider{}

func NewVSphereVmProvider(clientset *kubernetes.Clientset) (*VSphereVmProvider, error) {
	vmProvider := &VSphereVmProvider{
		sessions: NewSessionManager(clientset),
	}

	return vmProvider, nil
}

func NewVSphereVmProviderFromConfig(namespace string, config *VSphereVmProviderConfig) (*VSphereVmProvider, error) {
	vmProvider := &VSphereVmProvider{
		sessions: NewSessionManager(nil),
	}

	// Support existing behavior by setting up a Session for whatever namespace we're using. This is
	// used in the integration tests.
	_, err := vmProvider.sessions.NewSession(namespace, config)
	if err != nil {
		return nil, err
	}

	return vmProvider, nil
}

func (vs *VSphereVmProvider) Name() string {
	return VsphereVmProviderName
}

func (vs *VSphereVmProvider) Initialize(stop <-chan struct{}) {
}

func (vs *VSphereVmProvider) ListVirtualMachineImages(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error) {
	klog.Infof("Listing VirtualMachineImages in namespace %q", namespace)

	ses, err := vs.sessions.GetSession(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// TODO(bryanv) Need an actual path here?
	resVms, err := ses.ListVirtualMachines(ctx, "*")
	if err != nil {
		return nil, transformVmImageError("", err)
	}

	var images []*v1alpha1.VirtualMachineImage
	for _, resVm := range resVms {
		images = append(images, resVmToVirtualMachineImage(ctx, namespace, resVm))
	}

	if ses.contentlib != nil {
		//List images from Content Library
		imagesFromCL, err := ses.ListVirtualMachineImagesFromCL(ctx, namespace)
		if err != nil {
			return nil, err
		}

		images = append(images, imagesFromCL...)
	}

	return images, nil
}

func (vs *VSphereVmProvider) GetVirtualMachineImage(ctx context.Context, namespace, name string) (*v1alpha1.VirtualMachineImage, error) {
	vmName := fmt.Sprintf("%v/%v", namespace, name)

	klog.Infof("Getting image for VirtualMachine %v", vmName)

	ses, err := vs.sessions.GetSession(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// Find items in Library if Content Lib has been initialized
	if ses.contentlib != nil {
		image, err := ses.GetVirtualMachineImageFromCL(ctx, name, namespace)
		if err != nil {
			return nil, err
		}

		// If image is found return image or continue
		if image != nil {
			return image, nil
		}
	}

	resVm, err := ses.GetVirtualMachine(ctx, name)
	if err != nil {
		return nil, transformVmImageError(vmName, err)
	}

	return resVmToVirtualMachineImage(ctx, namespace, resVm), nil
}

func (vs *VSphereVmProvider) ListVirtualMachines(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachine, error) {
	return nil, nil
}

func (vs *VSphereVmProvider) DoesVirtualMachineExist(ctx context.Context, namespace, name string) (bool, error) {
	ses, err := vs.sessions.GetSession(ctx, namespace)
	if err != nil {
		return false, err
	}

	if _, err = ses.GetVirtualMachine(ctx, name); err != nil {
		switch err.(type) {
		case *find.NotFoundError, *find.DefaultNotFoundError:
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}

func (vs *VSphereVmProvider) addProviderAnnotations(objectMeta *v1.ObjectMeta, vmRes *res.VirtualMachine) {
	annotations := objectMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[pkg.VmOperatorVmProviderKey] = VsphereVmProviderName
	annotations[VmOperatorMoRefKey] = vmRes.ReferenceValue()

	objectMeta.SetAnnotations(annotations)
}

func (vs *VSphereVmProvider) CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine,
	vmClass v1alpha1.VirtualMachineClass, vmMetadata vmprovider.VirtualMachineMetadata) error {

	vmName := vm.NamespacedName()
	klog.Infof("Creating VirtualMachine %v", vmName)

	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return err
	}

	// Determine if this is a clone or create from scratch.
	// The later is really only useful for dummy VMs at the moment.
	var resVm *res.VirtualMachine
	if vm.Spec.ImageName == "" {
		resVm, err = ses.CreateVirtualMachine(ctx, vm, vmClass, vmMetadata)
	} else {
		resVm, err = ses.CloneVirtualMachine(ctx, vm, vmClass, vmMetadata)
	}

	if err != nil {
		klog.Errorf("Create/Clone VirtualMachine %v failed: %v", vmName, err)
		return transformVmError(vmName, err)
	}

	err = vs.mergeVmStatus(ctx, vm, resVm)
	if err != nil {
		return transformVmError(vmName, err)
	}

	vs.addProviderAnnotations(&vm.ObjectMeta, resVm)

	return nil
}

func (vs *VSphereVmProvider) updateVm(ctx context.Context, vm *v1alpha1.VirtualMachine, resVm *res.VirtualMachine) error {
	err := vs.updatePowerState(ctx, vm, resVm)
	if err != nil {
		return err
	}

	return nil
}

func (vs *VSphereVmProvider) updatePowerState(ctx context.Context, vm *v1alpha1.VirtualMachine, resVm *res.VirtualMachine) error {
	// Default to on.
	powerState := v1alpha1.VirtualMachinePoweredOn
	if vm.Spec.PowerState != "" {
		powerState = vm.Spec.PowerState
	}

	if err := resVm.SetPowerState(ctx, powerState); err != nil {
		return errors.Wrapf(err, "failed to set power state to %v", powerState)
	}

	return nil
}

// UpdateVirtualMachine updates the VM status, power state, phase etc
func (vs *VSphereVmProvider) UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error {
	vmName := vm.NamespacedName()
	klog.Infof("Updating VirtualMachine %v", vmName)

	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return err
	}

	resVm, err := ses.GetVirtualMachine(ctx, vm.Name)
	if err != nil {
		return transformVmError(vmName, err)
	}

	err = vs.updateVm(ctx, vm, resVm)
	if err != nil {
		return transformVmError(vmName, err)
	}

	err = vs.mergeVmStatus(ctx, vm, resVm)
	if err != nil {
		return transformVmError(vmName, err)
	}

	return nil
}

func (vs *VSphereVmProvider) DeleteVirtualMachine(ctx context.Context, vmToDelete *v1alpha1.VirtualMachine) error {
	vmName := vmToDelete.NamespacedName()
	klog.Infof("Deleting VirtualMachine %v", vmName)

	ses, err := vs.sessions.GetSession(ctx, vmToDelete.Namespace)
	if err != nil {
		return err
	}

	resVm, err := ses.GetVirtualMachine(ctx, vmToDelete.Name)
	if err != nil {
		return transformVmError(vmName, err)
	}

	deleteSequence := sequence.NewVirtualMachineDeleteSequence(vmToDelete, resVm)
	err = deleteSequence.Execute(ctx)
	if err != nil {
		klog.Errorf("Delete VirtualMachine %v sequence failed: %v", vmName, err)
		return err
	}

	return nil
}

// mergeVmStatus merges the v1alpha1 VM's status with resource VM's status
func (vs *VSphereVmProvider) mergeVmStatus(ctx context.Context, vm *v1alpha1.VirtualMachine, resVm *res.VirtualMachine) error {
	vmStatus, err := resVm.GetStatus(ctx)
	if err != nil {
		return errors.Wrapf(err, "unable to get VirtualMachine status")
	}

	vmStatus.Phase = vm.Status.Phase
	vmStatus.DeepCopyInto(&vm.Status)

	return nil
}

func resVmToVirtualMachineImage(ctx context.Context, namespace string, resVm *res.VirtualMachine) *v1alpha1.VirtualMachineImage {
	powerState, uuid, reference := resVm.ImageFields(ctx)

	return &v1alpha1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{
			Name:      resVm.Name,
			Namespace: namespace,
		},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       uuid,
			InternalId: reference,
			PowerState: powerState,
		},
		Spec: v1alpha1.VirtualMachineImageSpec{
			Type:            "VM",
			ImageSourceType: "Inventory",
		},
	}
}

func libItemToVirtualMachineImage(item *library.Item, namespace string) *v1alpha1.VirtualMachineImage {

	return &v1alpha1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{
			Name:      item.Name,
			Namespace: namespace,
		},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       item.ID,
			InternalId: item.Name,
		},
		Spec: v1alpha1.VirtualMachineImageSpec{
			Type:            item.Type,
			ImageSourceType: "Content Library",
		},
	}

}

// Transform Govmomi error to Kubernetes error
// TODO: Fill out with VIM fault types
func transformError(resourceType string, resource string, err error) error {
	switch err.(type) {
	case *find.NotFoundError, *find.DefaultNotFoundError:
		return k8serror.NewNotFound(vmoperator.Resource(resourceType), resource)
	case *find.MultipleFoundError, *find.DefaultMultipleFoundError:
		// Transform?
		return err
	default:
		return err
	}
}

func transformVmError(resource string, err error) error {
	return transformError(vmoperator.InternalVirtualMachine.GetKind(), resource, err)
}

func transformVmImageError(resource string, err error) error {
	return transformError(vmoperator.InternalVirtualMachineImage.GetKind(), resource, err)
}
