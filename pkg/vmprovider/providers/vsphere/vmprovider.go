/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"fmt"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/iface"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/sequence"

	"github.com/pkg/errors"

	"github.com/golang/glog"
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

var _ iface.VirtualMachineProviderInterface = &VSphereVmProvider{}

func NewVSphereVmProvider(clientset *kubernetes.Clientset) (*VSphereVmProvider, error) {
	vmProvider := &VSphereVmProvider{
		sessions: NewSessionManager(clientset),
	}

	return vmProvider, nil
}

func NewVSphereVmProviderFromConfig(namespace string, config *VSphereVmProviderConfig, credentials *VSphereVmProviderCredentials) (*VSphereVmProvider, error) {
	vmProvider := &VSphereVmProvider{
		sessions: NewSessionManager(nil),
	}

	// Support existing behavior by setting up a Session for whatever namespace we're using.
	_, err := vmProvider.sessions.NewSession(namespace, config, credentials)
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
	glog.Infof("Listing VirtualMachineImages in namespace %q", namespace)

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

	return images, nil
}

func (vs *VSphereVmProvider) GetVirtualMachineImage(ctx context.Context, namespace, name string) (*v1alpha1.VirtualMachineImage, error) {
	vmName := fmt.Sprintf("%v/%v", namespace, name)

	glog.Infof("Getting image for VirtualMachine %v", vmName)

	ses, err := vs.sessions.GetSession(ctx, namespace)
	if err != nil {
		return nil, err
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

func (vs *VSphereVmProvider) GetVirtualMachine(ctx context.Context, namespace, name string) (*v1alpha1.VirtualMachine, error) {
	vmName := fmt.Sprintf("%v/%v", namespace, name)

	glog.Infof("Getting VirtualMachine %v", vmName)

	ses, err := vs.sessions.GetSession(ctx, namespace)
	if err != nil {
		return nil, err
	}

	resVm, err := ses.GetVirtualMachine(ctx, name)
	if err != nil {
		return nil, transformVmError(vmName, err)
	}

	vm, err := vs.mergeVmStatus(ctx, &v1alpha1.VirtualMachine{}, resVm)
	if err != nil {
		return nil, errors.Wrapf(err, "error in creating the VM: %v", name)
	}

	return vm, nil
}

func (vs *VSphereVmProvider) addProviderAnnotations(objectMeta *v1.ObjectMeta, vmRes *res.VirtualMachine) {
	annotations := objectMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[pkg.VmOperatorVmProviderKey] = VsphereVmProviderName
	//annotations[VmOperatorVcUuidKey] = vs.Config.VcPNID
	annotations[VmOperatorMoRefKey] = vmRes.ReferenceValue()

	objectMeta.SetAnnotations(annotations)
}

func (vs *VSphereVmProvider) CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmClass *v1alpha1.VirtualMachineClass, metadata map[string]string) (*v1alpha1.VirtualMachine, error) {
	vmName := vm.NamespacedName()

	glog.Infof("Creating VirtualMachine %v", vmName)

	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return nil, err
	}

	// Determine if this is a clone of an existing image or creation from scratch.
	// The later is really only useful for dummy VMs at the moment.
	var resVm *res.VirtualMachine
	if vm.Spec.ImageName == "" {
		resVm, err = ses.CreateVirtualMachine(ctx, vm, vmClass)
	} else {
		resVm, err = ses.CloneVirtualMachine(ctx, vm, vmClass, metadata)
	}

	if err != nil {
		glog.Errorf("Create VirtualMachine %v failed: %v", vmName, err)
		return nil, transformVmError(vmName, err)
	}

	vm, err = vs.mergeVmStatus(ctx, vm, resVm)
	if err != nil {
		return nil, transformVmError(vmName, err)
	}

	vs.addProviderAnnotations(&vm.ObjectMeta, resVm)

	return vm, nil
}

func (vs *VSphereVmProvider) updateResourceSettings(ctx context.Context, vm *v1alpha1.VirtualMachine, resVm *res.VirtualMachine) error {

	cpu, err := resVm.CpuAllocation(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get VirtualMachine CPU allocation")
	}

	mem, err := resVm.MemoryAllocation(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get VirtualMachine memory allocation")
	}

	glog.Infof("VirtualMachine %q reservation/limit CPU: %d/%d Memory: %d/%d", vm.NamespacedName(),
		*cpu.Reservation, *cpu.Limit, *mem.Reservation, *mem.Limit)

	/*
		// TODO(bryanv) Handle Class changes later. We need to keep track of the current class
		// TODO: and if it has changed.
		// TODO: Only processing memory RLS for now

		needUpdate := false

		newRes := vmToUpdate.Spec.Resources.Requests.Memory
		newLimit := vmToUpdate.Spec.Resources.Limits.Memory
		glog.Infof("New Memory allocation: %d/%d", newRes, newLimit)

		if newRes != 0 && newRes != *mem.Reservation {
			glog.Infof("Updating mem reservation from %d to %d", *mem.Reservation, newRes)
			*mem.Reservation = newRes
			needUpdate = true
		}

		if newLimit != 0 && newLimit != *mem.Limit {
			glog.Infof("Updating mem limit from %d to %d", *mem.Limit, newLimit)
			*mem.Limit = newLimit
			needUpdate = true
		}

		if needUpdate {
			configSpec := types.VirtualMachineConfigSpec{}
			configSpec.CpuAllocation = cpu
			configSpec.MemoryAllocation = mem

			task, err := vm.VirtualMachine.Reconfigure(ctx, configSpec)
			if err != nil {
				glog.Errorf("Failed to change power state to %s", vmToUpdate.Spec.PowerState)
				return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
			}

			//taskInfo, err := task.WaitForResult(ctx, nil)
			err = task.Wait(ctx)
			if err != nil {
				glog.Errorf("VM RLS change task failed %s", err.Error())
				return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
			}

			// TODO: Resolve the issue with TaskResult and reconfigure (nil MoRef?)
			return vm, nil
			//return resources.NewVMFromReference(*vclient, vm.Datacenter, taskInfo.Result.(types.ManagedObjectReference))
		}
	*/

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
func (vs *VSphereVmProvider) UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	vmName := vm.NamespacedName()

	glog.Infof("Updating VirtualMachine %v", vmName)

	ses, err := vs.sessions.GetSession(ctx, vm.Namespace)
	if err != nil {
		return nil, err
	}

	resVm, err := ses.GetVirtualMachine(ctx, vm.Name)
	if err != nil {
		return nil, transformVmError(vmName, err)
	}

	err = vs.updateResourceSettings(ctx, vm, resVm)
	if err != nil {
		return nil, transformVmError(vmName, err)
	}

	err = vs.updatePowerState(ctx, vm, resVm)
	if err != nil {
		return nil, transformVmError(vmName, err)
	}

	vm, err = vs.mergeVmStatus(ctx, vm, resVm)
	if err != nil {
		return nil, transformVmError(vmName, err)
	}

	return vm, nil
}

func (vs *VSphereVmProvider) DeleteVirtualMachine(ctx context.Context, vmToDelete *v1alpha1.VirtualMachine) error {
	vmName := vmToDelete.NamespacedName()

	glog.Infof("Deleting VirtualMachine %v", vmName)

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
		glog.Errorf("Delete VirtualMachine %v sequence failed: %v", vmName, err)
		return err
	}

	return nil
}

//mergeVmStatus merges the v1alpha1 VM's status with resource VM's status
func (vs *VSphereVmProvider) mergeVmStatus(ctx context.Context, vm *v1alpha1.VirtualMachine, resVm *res.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	vmStatus, err := resVm.GetStatus(ctx)
	if err != nil {
		glog.Infof("GetStatus returned error: %v", err)
		return nil, errors.Wrapf(err, "unable to get status of the VM")
	}

	vm.Status = *vmStatus
	return vm, nil
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
