/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"fmt"
	"io"

	"github.com/golang/glog"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"vmware.com/kubevsphere/pkg"
	"vmware.com/kubevsphere/pkg/apis/vmoperator"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	"vmware.com/kubevsphere/pkg/vmprovider"
	"vmware.com/kubevsphere/pkg/vmprovider/iface"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/sequence"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/session"
)

const VsphereVmProviderName string = "vsphere"

func InitProvider(clientSet *kubernetes.Clientset) error {
	providerConfig, err := GetProviderConfigFromConfigMap(clientSet)
	if err != nil {
		return err
	}

	InitProviderWithConfig(providerConfig)
	return nil
}

func InitProviderWithConfig(providerConfig *VSphereVmProviderConfig) {

	factory := func(config io.Reader) (iface.VirtualMachineProviderInterface, error) {
		return newVSphereVmProvider(providerConfig)
	}

	vmprovider.RegisterVmProvider(VsphereVmProviderName, factory)
}

type VSphereVmProvider struct {
	config  VSphereVmProviderConfig
	manager *VSphereManager
}

var _ iface.VirtualMachineProviderInterface = &VSphereVmProvider{}
var _ iface.VirtualMachines = &VSphereVmProvider{}
var _ iface.VirtualMachineImages = &VSphereVmProvider{}

func (vs *VSphereVmProvider) Initialize(stop <-chan struct{}) {
}

func (vs *VSphereVmProvider) VirtualMachines() (iface.VirtualMachines, bool) {
	return vs, true
}

func (vs *VSphereVmProvider) VirtualMachineImages() (iface.VirtualMachineImages, bool) {
	return vs, true
}

func newVSphereVmProvider(providerConfig *VSphereVmProviderConfig) (*VSphereVmProvider, error) {
	vs, err := NewVSphereManager(providerConfig)
	if err != nil {
		return nil, err
	}

	return &VSphereVmProvider{
		config:  *providerConfig,
		manager: vs,
	}, nil
}

func (vs *VSphereVmProvider) ListVirtualMachineImages(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error) {
	glog.Infof("Listing VirtualMachineImages in namespace %q", namespace)

	sc, err := vs.manager.GetSession()
	if err != nil {
		return nil, err
	}

	vms, err := vs.manager.ListVms(ctx, sc, "")
	if err != nil {
		return nil, transformError(vmoperator.InternalVirtualMachineImage.GetKind(), "", err)
	}

	var newImages []*v1alpha1.VirtualMachineImage
	for _, vm := range vms {
		powerState, _ := vm.VirtualMachine.PowerState(ctx)
		ps := string(powerState)
		newImages = append(newImages,
			&v1alpha1.VirtualMachineImage{
				ObjectMeta: v1.ObjectMeta{Name: vm.VirtualMachine.Name()},
				Status: v1alpha1.VirtualMachineImageStatus{
					Uuid:       vm.VirtualMachine.UUID(ctx),
					PowerState: ps,
					InternalId: vm.VirtualMachine.Reference().Value,
				},
			},
		)
	}

	return newImages, nil
}

func (vs *VSphereVmProvider) GetVirtualMachineImage(ctx context.Context, name string) (*v1alpha1.VirtualMachineImage, error) {
	glog.Infof("Getting image for VirtualMachine %q", name)

	sc, err := vs.manager.GetSession()
	if err != nil {
		return nil, err
	}

	vm, err := vs.manager.LookupVm(ctx, sc, name)
	if err != nil {
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	powerState, _ := vm.VirtualMachine.PowerState(ctx)
	ps := string(powerState)

	return &v1alpha1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{Name: vm.VirtualMachine.Name()},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       vm.VirtualMachine.UUID(ctx),
			PowerState: ps,
			InternalId: vm.VirtualMachine.Reference().Value,
		},
	}, nil
}

func (vs *VSphereVmProvider) generateVmStatus(ctx context.Context, actualVm *resources.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	powerState, _ := actualVm.VirtualMachine.PowerState(ctx)
	ps := string(powerState)

	host, err := actualVm.VirtualMachine.HostSystem(ctx)
	if err != nil {
		glog.Infof("Failed to get host system for VirtualMachine %q: %v", actualVm.Name, err)
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	// If guest is powered off, IP acquisition will fail.
	// If guest is powered on, IP acquisition may fail.  Subsequently reconciliation will resolve it.
	// TODO: Consider separating vm status collection into powered off and powered on versions
	// TODO: Consider how to balance between wanting to acquire the IP and supporting a VM without IPs.
	vmIp := ""
	if powerState == types.VirtualMachinePowerStatePoweredOn {
		if vmIp, err = actualVm.IpAddress(ctx); err != nil {
			glog.Infof("Failed to get IP for VirtualMachine %q: %v", actualVm.Name, err)
		}
	}

	glog.Infof("VirtualMachine %q host: %s IP: %s", actualVm.Name, host.Name(), vmIp)

	vm := &v1alpha1.VirtualMachine{
		Status: v1alpha1.VirtualMachineStatus{
			Phase:      "",
			PowerState: ps,
			Host:       vmIp, // TODO(bryanv) host.Name()?
			VmIp:       vmIp,
		},
	}

	glog.Infof("Generated VM status: %+v", vm.Status)

	return vm, nil
}

func (vs *VSphereVmProvider) mergeVmStatus(ctx context.Context, desiredVm *v1alpha1.VirtualMachine, actualVm *resources.VirtualMachine) (*v1alpha1.VirtualMachine, error) {

	statusVm, err := vs.generateVmStatus(ctx, actualVm)
	if err != nil {
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	return &v1alpha1.VirtualMachine{
		TypeMeta:   desiredVm.TypeMeta,
		ObjectMeta: desiredVm.ObjectMeta,
		Spec:       desiredVm.Spec,
		Status:     *statusVm.Status.DeepCopy(),
	}, nil
}

func (vs *VSphereVmProvider) ListVirtualMachines(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachine, error) {
	return nil, nil
}

func (vs *VSphereVmProvider) GetVirtualMachine(ctx context.Context, name string) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Getting VirtualMachine %q", name)

	sc, err := vs.manager.GetSession()
	if err != nil {
		return nil, err
	}

	vm, err := vs.manager.LookupVm(ctx, sc, name)
	if err != nil {
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	return vs.generateVmStatus(ctx, vm)
}

func (vs *VSphereVmProvider) addProviderAnnotations(objectMeta *v1.ObjectMeta, moRef string) {
	// Add vSphere provider annotations to the object meta
	annotations := objectMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[pkg.VmOperatorVmProviderKey] = VsphereVmProviderName
	//annotations[pkg.VmOperatorVcUuidKey] = vs.Config.VcUrl
	annotations[pkg.VmOperatorMoRefKey] = moRef

	objectMeta.SetAnnotations(annotations)
}

func (vs *VSphereVmProvider) CreateVirtualMachine(ctx context.Context, newVm *v1alpha1.VirtualMachine, vmClass *v1alpha1.VirtualMachineClass) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Creating VirtualMachine %q", newVm.Name)

	sc, err := vs.manager.GetSession()
	if err != nil {
		return nil, err
	}

	// Determine if this create is a clone of an existing image or creation from scratch.
	// Create from scratch is really only useful for dummy VMs at the moment.
	var vm *resources.VirtualMachine
	if newVm.Spec.Image == "" {
		vm, err = vs.manager.CreateVm(ctx, sc, newVm, vmClass)
	} else {
		vm, err = vs.manager.CloneVm(ctx, sc, newVm, vmClass)
	}

	if err != nil {
		glog.Errorf("VirtualMachine %q create failed: %v", newVm.Name, err)
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	vs.addProviderAnnotations(&newVm.ObjectMeta, vm.VirtualMachine.Reference().Value)

	return vs.mergeVmStatus(ctx, newVm, vm)
}

func (vs *VSphereVmProvider) updateResourceSettings(ctx context.Context, sc *session.SessionContext, vmToUpdate *v1alpha1.VirtualMachine, vm *resources.VirtualMachine) (*resources.VirtualMachine, error) {

	cpu, err := vm.CpuAllocation(ctx)
	if err != nil {
		glog.Errorf("Failed to acquire cpu allocation info: %s", err.Error())
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	mem, err := vm.MemoryAllocation(ctx)
	if err != nil {
		glog.Errorf("Failed to acquire memory allocation info: %s", err.Error())
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	glog.Infof("Current CPU allocation: %d/%d", *cpu.Reservation, *cpu.Limit)
	glog.Infof("Current Memory allocation: %d/%d", *mem.Reservation, *mem.Limit)

	/*
		// TODO(bryanv) Handle Class changes later.
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

	return vm, nil
}

func (vs *VSphereVmProvider) updatePowerState(ctx context.Context, sc *session.SessionContext, vmToUpdate *v1alpha1.VirtualMachine, vm *resources.VirtualMachine) (*resources.VirtualMachine, error) {
	vmName := vmToUpdate.Name

	// Default to Powered On
	desiredPowerState := v1alpha1.VirtualMachinePoweredOn
	if vmToUpdate.Spec.PowerState != "" {
		desiredPowerState = vmToUpdate.Spec.PowerState
	}

	ps, err := vm.VirtualMachine.PowerState(ctx)
	if err != nil {
		glog.Errorf("Failed to get VirtualMachine %q power state: %v", vmName, err)
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	glog.Infof("VirtualMachine %q current power state: %s, desired: %s", vmName, ps, vmToUpdate.Spec.PowerState)

	if string(ps) == string(desiredPowerState) {
		return vm, nil
	}

	// Bring PowerState into conformance
	var task *object.Task
	switch desiredPowerState {
	case v1alpha1.VirtualMachinePoweredOn:
		task, err = vm.VirtualMachine.PowerOn(ctx)
	case v1alpha1.VirtualMachinePoweredOff:
		task, err = vm.VirtualMachine.PowerOff(ctx)
	default:
		// TODO(bryanv) Suspend? How would we handle reset?
		err = fmt.Errorf("invalid desired power state %s", desiredPowerState)
	}

	if err != nil {
		glog.Errorf("Failed to change VirtualMachine %q to power state %s: %v", vmName, desiredPowerState, err)
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	taskInfo, err := task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("VirtualMachine %q change power state task failed: %v", vmName, err)
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	return resources.NewVMFromReference(*sc.Client, taskInfo.Result.(types.ManagedObjectReference))
}

func (vs *VSphereVmProvider) UpdateVirtualMachine(ctx context.Context, vmToUpdate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Updating VirtualMachine %q", vmToUpdate.Name)

	sc, err := vs.manager.GetSession()
	if err != nil {
		return nil, err
	}

	vm, err := vs.manager.LookupVm(ctx, sc, vmToUpdate.Name)
	if err != nil {
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	vm, err = vs.updateResourceSettings(ctx, sc, vmToUpdate, vm)
	if err != nil {
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	vm, err = vs.updatePowerState(ctx, sc, vmToUpdate, vm)
	if err != nil {
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	// Update spec
	return vs.mergeVmStatus(ctx, vmToUpdate, vm)
}

func (vs *VSphereVmProvider) DeleteVirtualMachine(ctx context.Context, vmToDelete *v1alpha1.VirtualMachine) error {
	glog.Infof("Deleting VirtualMachine %q", vmToDelete.Name)

	sc, err := vs.manager.GetSession()
	if err != nil {
		return err
	}

	vm, err := vs.manager.LookupVm(ctx, sc, vmToDelete.Name)
	if err != nil {
		return transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	deleteSequence := sequence.NewVirtualMachineDeleteSequence(vmToDelete, vm)
	err = deleteSequence.Execute(ctx)

	glog.Infof("Delete sequence completed: %s", err)
	return err
}

// Transform Govmomi error to Kubernetes error
// TODO: Fill out with VIM fault types
func transformError(resourceType string, resource string, err error) error {
	var transformed error
	switch err.(type) {
	case *find.NotFoundError:
		transformed = errors.NewNotFound(vmoperator.Resource(resourceType), resource)
	default:
		transformed = err
	}

	return transformed
}
