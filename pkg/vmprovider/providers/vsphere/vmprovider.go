/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"io"

	"github.com/golang/glog"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
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

var _ = &VSphereVmProvider{}

const VsphereVmProviderName string = "vsphere"

func InitProviderWithConfig(providerConfig *VSphereVmProviderConfig) error {
	SetProviderConfigWithConfig(providerConfig)

	vmprovider.RegisterVmProvider(VsphereVmProviderName, func(config io.Reader) (iface.VirtualMachineProviderInterface, error) {
		return newVSphereVmProvider(providerConfig)
	})

	return nil
}
func InitProvider(clientSet *kubernetes.Clientset) error {
	if err := SetProviderConfigWithClientset(clientSet); err != nil {
		return err
	}

	vmprovider.RegisterVmProvider(VsphereVmProviderName, func(config io.Reader) (iface.VirtualMachineProviderInterface, error) {
		providerConfig := GetVsphereVmProviderConfig()
		return newVSphereVmProvider(providerConfig)
	})

	return nil
}

type VSphereVmProvider struct {
	Config  VSphereVmProviderConfig
	manager *VSphereManager
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

// Creates new Controller node interface and returns
func newVSphereVmProvider(providerConfig *VSphereVmProviderConfig) (*VSphereVmProvider, error) {
	vs := NewVSphereManager()
	vmProvider := &VSphereVmProvider{*providerConfig, vs}
	return vmProvider, nil
}

func (vs *VSphereVmProvider) VirtualMachines() (iface.VirtualMachines, bool) {
	return vs, true
}

func (vs *VSphereVmProvider) VirtualMachineImages() (iface.VirtualMachineImages, bool) {
	return vs, true
}

func (vs *VSphereVmProvider) Initialize(stop <-chan struct{}) {
}

func (vs *VSphereVmProvider) ListVirtualMachineImages(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error) {
	glog.Info("Listing VM images")

	// Get a session from the config. Return the cached session if any. This assumes that there is ONLY ONE session per vspheremanager.
	// If we want to extend it, then we can create multiple sessions based on namespace/vcurl/username and cache them in the vspheremanager
	sc, err := vs.manager.GetSession()
	if err != nil {
		return nil, err
	}

	vms, err := vs.manager.ListVms(ctx, sc, "")
	if err != nil {
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	newImages := []*v1alpha1.VirtualMachineImage{}
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
	glog.Info("Getting VM images")

	// Get a session from the config. Return the cached session if any. This assumes that there is ONLY ONE session per vspheremanager.
	// If we want to extend it, then we can create multiple sessions based on namespace/vcurl/username and cache them in the vspheremanager
	sc, err := vs.manager.GetSession()
	if err != nil {
		return nil, err
	}

	// DWB: Reason about how to handle client management and logout
	//defer vClient.Logout(ctx)

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

func NewVirtualMachineImageFake(name string) *v1alpha1.VirtualMachineImage {
	return &v1alpha1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{Name: name},
	}
}

func NewVirtualMachineImageListFake() []*v1alpha1.VirtualMachineImage {
	images := []*v1alpha1.VirtualMachineImage{}

	fake1 := NewVirtualMachineImageFake("Fake")
	fake2 := NewVirtualMachineImageFake("Fake2")
	images = append(images, fake1)
	images = append(images, fake2)
	return images
}

func (vs *VSphereVmProvider) generateVmStatus(ctx context.Context, actualVm *resources.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	powerState, _ := actualVm.VirtualMachine.PowerState(ctx)
	ps := string(powerState)

	host, err := actualVm.VirtualMachine.HostSystem(ctx)
	if err != nil {
		glog.Infof("Failed to acquire host system for VM %s: %s", err, actualVm.Name)
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	// If guest is powered off, IP acquisition will fail.
	// If guest is powered on, IP acquisition may fail.  Subsequenty reconcilitaion will resolve it.
	// TODO: Consider separating vm status collection into powered off and powered on versions
	// TODO: Consider how to balance between wanting to acquire the IP and supporting a VM without IPs.
	vmIp := ""
	if powerState == types.VirtualMachinePowerStatePoweredOn {
		vmIp, err = actualVm.IpAddress(ctx)
		if err != nil {
			glog.Infof("Failed to acquire IP for VM %s: %s", err, actualVm.Name)
		}
	}

	glog.Infof("VM Host/IP is %s/%s", host.Name(), vmIp)

	vm := &v1alpha1.VirtualMachine{
		Status: v1alpha1.VirtualMachineStatus{
			Phase:      "",
			PowerState: ps,
			Host:       vmIp,
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
	glog.Info("Getting VMs")

	// Get a session from the config. Return the cached session if any. This assumes that there is ONLY ONE session per vspheremanager.
	// If we want to extend it, then we can create multiple sessions based on namespace/vcurl/username and cache them in the vspheremanager
	sc, err := vs.manager.GetSession()
	if err != nil {
		return nil, err
	}

	// DWB: Reason about how to handle client management and logout
	//defer vClient.Logout(ctx)

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
	annotations[pkg.VmOperatorMorefKey] = moRef

	objectMeta.SetAnnotations(annotations)
}

func (vs *VSphereVmProvider) CreateVirtualMachine(ctx context.Context, vmToCreate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Creating Vm: %s", vmToCreate.Name)

	// Get a session from the config. Return the cached session if any. This assumes that there is ONLY ONE session per vspheremanager.
	// If we want to extend it, then we can create multiple sessions based on namespace/vcurl/username and cache them in the vspheremanager
	sc, err := vs.manager.GetSession()
	if err != nil {
		return nil, err
	}

	// Determine if we should clone from an existing image or create from scratch.  Create from scratch is really
	// only useful for dummy VMs at the moment.
	var newVm *resources.VirtualMachine
	switch {
	case vmToCreate.Spec.Image != "":
		glog.Infof("Cloning VM from %s", vmToCreate.Spec.Image)
		newVm, err = vs.manager.CloneVm(ctx, sc, vmToCreate)
	default:
		glog.Info("Creating new VM")
		newVm, err = vs.manager.CreateVm(ctx, sc, vmToCreate)
	}

	if err != nil {
		glog.Infof("Create VM failed %s!", err)
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	vs.addProviderAnnotations(&vmToCreate.ObjectMeta, newVm.VirtualMachine.Reference().Value)
	return vs.mergeVmStatus(ctx, vmToCreate, newVm)
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

	// TODO: Only processing memory RLS for now
	needUpdate := false

	newRes := vmToUpdate.Spec.Resources.Limits.Memory
	newLimit := vmToUpdate.Spec.Resources.Requests.Memory
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

		// TODO: Resolve the issue with TaskResult and reconfigure
		return vm, nil
		//return resources.NewVMFromReference(*vclient, vm.Datacenter, taskInfo.Result.(types.ManagedObjectReference))
	}

	return vm, nil
}

func (vs *VSphereVmProvider) updatePowerState(ctx context.Context, sc *session.SessionContext, vmToUpdate *v1alpha1.VirtualMachine, vm *resources.VirtualMachine) (*resources.VirtualMachine, error) {
	// Default to Powered On
	desiredPowerState := v1alpha1.VirtualMachinePoweredOn
	if vmToUpdate.Spec.PowerState != "" {
		desiredPowerState = vmToUpdate.Spec.PowerState
	}

	ps, err := vm.VirtualMachine.PowerState(ctx)
	if err != nil {
		glog.Errorf("Failed to acquire power state: %s", err.Error())
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	glog.Infof("Current power state: %s, desired power state: %s", ps, vmToUpdate.Spec.PowerState)

	if string(ps) == string(desiredPowerState) {
		glog.Infof("Power state already at desired state of %s", ps)
		return vm, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	// Bring PowerState into conformance
	var task *object.Task
	switch desiredPowerState {
	case v1alpha1.VirtualMachinePoweredOn:
		task, err = vm.VirtualMachine.PowerOn(ctx)
	case v1alpha1.VirtualMachinePoweredOff:
		task, err = vm.VirtualMachine.PowerOff(ctx)
	default:
		glog.Errorf("Impossible %s", desiredPowerState)
	}

	if err != nil {
		glog.Errorf("Failed to change power state to %s", vmToUpdate.Spec.PowerState)
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	taskInfo, err := task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("VM Power State change task failed %s", err.Error())
		return nil, transformError(vmoperator.InternalVirtualMachine.GetKind(), "", err)
	}

	return resources.NewVMFromReference(*sc.Client, taskInfo.Result.(types.ManagedObjectReference))
}

func (vs *VSphereVmProvider) UpdateVirtualMachine(ctx context.Context, vmToUpdate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Updating Vm: %s", vmToUpdate.Name)

	// Get a session from the config. Return the cached session if any. This assumes that there is ONLY ONE session per vspheremanager.
	// If we want to extend it, then we can create multiple sessions based on namespace/vcurl/username and cache them in the vspheremanager
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
	glog.Infof("Deleting Vm: %s", vmToDelete.Name)

	// Get a session from the config. Return the cached session if any. This assumes that there is ONLY ONE session per vspheremanager.
	// If we want to extend it, then we can create multiple sessions based on namespace/vcurl/username and cache them in the vspheremanager
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
