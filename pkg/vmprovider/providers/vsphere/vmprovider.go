/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"

	"vmware.com/kubevsphere/pkg"

	"vmware.com/kubevsphere/pkg/vmprovider/iface"

	"github.com/pkg/errors"

	"github.com/golang/glog"
	"github.com/vmware/govmomi/find"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"vmware.com/kubevsphere/pkg/apis/vmoperator"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	res "vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/sequence"
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

func NewVSphereVmProvider(clientSet *kubernetes.Clientset) (*VSphereVmProvider, error) {
	config, err := GetProviderConfigFromConfigMap(clientSet)
	if err != nil {
		return nil, err
	}

	return NewVSphereVmProviderFromConfig(config)
}

func NewVSphereVmProviderFromConfig(config *VSphereVmProviderConfig) (*VSphereVmProvider, error) {
	vmProvider := &VSphereVmProvider{
		sessions: NewSessionManager(),
	}

	// Support existing behavior by setting up a Session for whatever namespace we're using.
	_, err := vmProvider.sessions.NewSession(config)
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
	glog.Infof("Listing VirtualMachineImages for namespace %q", namespace)

	ses, err := vs.sessions.GetSession()
	if err != nil {
		return nil, err
	}

	resVms, err := ses.ListVirtualMachines(ctx, "")
	if err != nil {
		return nil, transformVmImageError("", err)
	}

	var images []*v1alpha1.VirtualMachineImage
	for _, resVm := range resVms {
		images = append(images, resVmToVirtualMachineImage(ctx, resVm))
	}

	return images, nil
}

func (vs *VSphereVmProvider) GetVirtualMachineImage(ctx context.Context, name string) (*v1alpha1.VirtualMachineImage, error) {
	glog.Infof("Getting image for VirtualMachine %q", name)

	ses, err := vs.sessions.GetSession()
	if err != nil {
		return nil, err
	}

	resVm, err := ses.GetVirtualMachine(ctx, name)
	if err != nil {
		return nil, transformVmImageError(name, err)
	}

	return resVmToVirtualMachineImage(ctx, resVm), nil
}

func (vs *VSphereVmProvider) ListVirtualMachines(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachine, error) {
	return nil, nil
}

func (vs *VSphereVmProvider) GetVirtualMachine(ctx context.Context, name string) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Getting VirtualMachine %q", name)

	ses, err := vs.sessions.GetSession()
	if err != nil {
		return nil, err
	}

	resVm, err := ses.GetVirtualMachine(ctx, name)
	if err != nil {
		return nil, transformVmError(name, err)
	}

	vmStatus, err := vs.generateVmStatus(ctx, resVm)
	if err != nil {
		return nil, transformVmError(name, err)
	}

	// BMV: Only need to set Status here?
	return &v1alpha1.VirtualMachine{
		Status: *vmStatus.DeepCopy(),
	}, nil
}

func (vs *VSphereVmProvider) addProviderAnnotations(objectMeta *v1.ObjectMeta, vmRes *res.VirtualMachine) {
	annotations := objectMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[pkg.VmOperatorVmProviderKey] = VsphereVmProviderName
	//annotations[VmOperatorVcUuidKey] = vs.Config.VcUrl
	annotations[VmOperatorMoRefKey] = vmRes.ReferenceValue()

	objectMeta.SetAnnotations(annotations)
}

func (vs *VSphereVmProvider) CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmClass *v1alpha1.VirtualMachineClass) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Creating VirtualMachine %q", vm.Name)

	ses, err := vs.sessions.GetSession()
	if err != nil {
		return nil, err
	}

	// Determine if this is a clone of an existing image or creation from scratch.
	// The later is really only useful for dummy VMs at the moment.
	var resVm *res.VirtualMachine
	if vm.Spec.Image == "" {
		resVm, err = ses.CreateVirtualMachine(ctx, vm, vmClass)
	} else {
		resVm, err = ses.CloneVirtualMachine(ctx, vm, vmClass)
	}

	if err != nil {
		glog.Errorf("VirtualMachine %q create failed: %v", vm.Name, err)
		return nil, transformVmError(vm.Name, err)
	}

	vs.addProviderAnnotations(&vm.ObjectMeta, resVm)

	return vs.mergeVmStatus(ctx, vm, resVm)
}

func (vs *VSphereVmProvider) updateResourceSettings(ctx context.Context, vm *v1alpha1.VirtualMachine, resVm *res.VirtualMachine) error {

	cpu, err := resVm.CpuAllocation(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get VirtualMachine %q CPU allocation", resVm.Name)
	}

	mem, err := resVm.MemoryAllocation(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get VirtualMachine %q memory allocation", resVm.Name)
	}

	glog.Infof("VirtualMachine %q reservation/limit CPU: %d/%d Memory: %d/%d", vm.Name,
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

	err := resVm.SetPowerState(ctx, powerState)
	if err != nil {
		return transformVmError(vm.Name, err)
	}

	return nil
}

func (vs *VSphereVmProvider) UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Updating VirtualMachine %q", vm.Name)

	ses, err := vs.sessions.GetSession()
	if err != nil {
		return nil, err
	}

	resVm, err := ses.GetVirtualMachine(ctx, vm.Name)
	if err != nil {
		return nil, transformVmError(vm.Name, err)
	}

	err = vs.updateResourceSettings(ctx, vm, resVm)
	if err != nil {
		return nil, transformVmError(vm.Name, err)
	}

	err = vs.updatePowerState(ctx, vm, resVm)
	if err != nil {
		return nil, transformVmError(vm.Name, err)
	}

	// Update status
	return vs.mergeVmStatus(ctx, vm, resVm)
}

func (vs *VSphereVmProvider) DeleteVirtualMachine(ctx context.Context, vmToDelete *v1alpha1.VirtualMachine) error {
	glog.Infof("Deleting VirtualMachine %q", vmToDelete.Name)

	ses, err := vs.sessions.GetSession()
	if err != nil {
		return err
	}

	resVm, err := ses.GetVirtualMachine(ctx, vmToDelete.Name)
	if err != nil {
		return transformVmError(vmToDelete.Name, err)
	}

	deleteSequence := sequence.NewVirtualMachineDeleteSequence(vmToDelete, resVm)
	err = deleteSequence.Execute(ctx)

	glog.Infof("Delete VirtualMachine %q sequence completed: %v", vmToDelete.Name, err)
	return err
}

func (vs *VSphereVmProvider) generateVmStatus(ctx context.Context, resVm *res.VirtualMachine) (v1alpha1.VirtualMachineStatus, error) {
	powerState, hostSystemName, ip := resVm.StatusFields(ctx)
	_ = hostSystemName

	status := v1alpha1.VirtualMachineStatus{
		Phase:      "",
		PowerState: powerState,
		Host:       ip, // TODO(bryanv) Use hostSystemName?
		VmIp:       ip,
	}

	glog.Infof("Generated VirtualMachine %q status: %+v", resVm.Name, status)

	return status, nil
}

func (vs *VSphereVmProvider) mergeVmStatus(ctx context.Context, vm *v1alpha1.VirtualMachine, resVm *res.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	vmStatus, err := vs.generateVmStatus(ctx, resVm)
	if err != nil {
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	return &v1alpha1.VirtualMachine{
		TypeMeta:   vm.TypeMeta,
		ObjectMeta: vm.ObjectMeta,
		Spec:       vm.Spec,
		Status:     *vmStatus.DeepCopy(),
	}, nil
}

func resVmToVirtualMachineImage(ctx context.Context, resVm *res.VirtualMachine) *v1alpha1.VirtualMachineImage {
	powerState, uuid, reference := resVm.ImageFields(ctx)

	return &v1alpha1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{Name: resVm.Name},
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
