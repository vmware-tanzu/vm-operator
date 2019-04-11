/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"fmt"
	"math"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	"k8s.io/apimachinery/pkg/api/resource"
)

type Session struct {
	client *Client

	finder       *find.Finder
	datacenter   *object.Datacenter
	dcFolders    *object.DatacenterFolders
	resourcepool *object.ResourcePool
	datastore    *object.Datastore
}

func NewSession(ctx context.Context, config *VSphereVmProviderConfig) (*Session, error) {
	c, err := NewClient(ctx, config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create client for new session")
	}

	s := &Session{
		client: c,
	}

	if err = s.initSession(ctx, config); err != nil {
		s.Logout(ctx)
		return nil, err
	}

	return s, nil
}

func (s *Session) initSession(ctx context.Context, config *VSphereVmProviderConfig) error {
	s.finder = find.NewFinder(s.client.VimClient(), false)

	dc, err := s.finder.Datacenter(ctx, config.Datacenter)
	if err != nil {
		return errors.Wrapf(err, "failed to init Datacenter %q", config.Datacenter)
	}

	s.datacenter = dc
	s.finder.SetDatacenter(dc)

	s.dcFolders, err = dc.Folders(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to init Datacenter %q folders", config.Datacenter)
	}

	s.resourcepool, err = s.finder.ResourcePool(ctx, config.ResourcePool)
	if err != nil {
		return errors.Wrapf(err, "failed to init ResourcePool %q", config.ResourcePool)
	}

	s.datastore, err = s.finder.Datastore(ctx, config.Datastore)
	if err != nil {
		return errors.Wrapf(err, "failed to init Datastore %q", config.Datastore)
	}

	return nil
}

func (s *Session) Logout(ctx context.Context) {
	s.client.Logout(ctx)
}

func (s *Session) ListVirtualMachines(ctx context.Context, path string) ([]*res.VirtualMachine, error) {
	var vms []*res.VirtualMachine

	objVms, err := s.finder.VirtualMachineList(ctx, path)
	if err != nil {
		switch err.(type) {
		case *find.NotFoundError, *find.DefaultNotFoundError:
			return vms, nil
		default:
			return nil, err
		}
	}

	for _, objVm := range objVms {
		if resVm, err := res.NewVMFromObject(objVm); err == nil {
			vms = append(vms, resVm)
		}
	}

	return vms, nil
}

func (s *Session) GetVirtualMachine(ctx context.Context, name string) (*res.VirtualMachine, error) {
	return s.lookupVm(ctx, name)
}

func (s *Session) CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine,
	class *v1alpha1.VirtualMachineClass) (*res.VirtualMachine, error) {

	name := vm.Name
	configSpec := s.configSpecFromClassSpec(name, &class.Spec)

	resVm, err := s.createVm(ctx, name, configSpec)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new VM %q", name)
	}

	return resVm, nil
}

func (s *Session) CloneVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine,
	class *v1alpha1.VirtualMachineClass) (*res.VirtualMachine, error) {

	resSrcVm, err := s.lookupVm(ctx, vm.Spec.ImageName)
	if err != nil {
		return nil, err
	}

	name := vm.Name
	powerOn := vm.Spec.PowerState == v1alpha1.VirtualMachinePoweredOn
	cloneSpec := s.cloneSpecFromClassSpec(name, powerOn, &class.Spec)

	cloneResVm, err := s.cloneVm(ctx, resSrcVm, cloneSpec)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to clone new VM %q from %q", name, resSrcVm.Name)
	}

	return cloneResVm, nil
}

func (s *Session) DeleteVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error {

	resVm, err := s.lookupVm(ctx, vm.Name)
	if err != nil {
		return err
	}

	err = resVm.Delete(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to delete VM %q", vm.Name)
	}

	return nil
}

func (s *Session) lookupVm(ctx context.Context, name string) (*res.VirtualMachine, error) {
	objVm, err := s.finder.VirtualMachine(ctx, name)
	if err != nil {
		return nil, err
	}

	return res.NewVMFromObject(objVm)
}

func memoryQuantityToMb(q resource.Quantity) int64 {
	return int64(math.Ceil(float64(q.Value()) / float64(1024*1024)))
}

func (s *Session) configSpecFromClassSpec(name string, classSpec *v1alpha1.VirtualMachineClassSpec) vimTypes.VirtualMachineConfigSpec {

	configSpec := vimTypes.VirtualMachineConfigSpec{
		Name:     name,
		NumCPUs:  int32(classSpec.Hardware.Cpus),
		MemoryMB: memoryQuantityToMb(classSpec.Hardware.Memory),
	}

	configSpec.CpuAllocation = &vimTypes.ResourceAllocationInfo{
		Reservation: &classSpec.Policies.Resources.Requests.Cpu,
		Limit:       &classSpec.Policies.Resources.Limits.Cpu,
	}

	configSpec.MemoryAllocation = &vimTypes.ResourceAllocationInfo{}

	if !classSpec.Policies.Resources.Requests.Memory.IsZero() {
		rsv := memoryQuantityToMb(classSpec.Policies.Resources.Requests.Memory)
		configSpec.MemoryAllocation.Reservation = &rsv
	}

	if !classSpec.Policies.Resources.Limits.Memory.IsZero() {
		lim := memoryQuantityToMb(classSpec.Policies.Resources.Limits.Memory)
		configSpec.MemoryAllocation.Limit = &lim
	}

	configSpec.Annotation = fmt.Sprint("Virtual Machine managed by VM Operator")

	return configSpec
}

func (s *Session) cloneSpecFromClassSpec(name string, powerOn bool, classSpec *v1alpha1.VirtualMachineClassSpec) vimTypes.VirtualMachineCloneSpec {

	// TODO(bryanv) CloneSpec's config is deprecated:
	//    as of vSphere API 6.0. Use deviceChange in location instead for specifying any virtual
	//    device changes for disks and networks. All other VM configuration changes should use
	//    ReconfigVM_Task API after the clone operation finishes.
	configSpec := s.configSpecFromClassSpec(name, classSpec)
	memory := false // No full memory clones

	cloneSpec := vimTypes.VirtualMachineCloneSpec{
		Config:  &configSpec,
		PowerOn: powerOn,
		Memory:  &memory,
	}

	cloneSpec.Location.Datastore = vimTypes.NewReference(s.datastore.Reference())
	cloneSpec.Location.Pool = vimTypes.NewReference(s.resourcepool.Reference())
	//cloneSpec.Location.DiskMoveType = string(vimTypes.VirtualMachineRelocateDiskMoveOptionsMoveAllDiskBackingsAndConsolidate)

	return cloneSpec
}

func (s *Session) createVm(ctx context.Context, name string, configSpec vimTypes.VirtualMachineConfigSpec) (*res.VirtualMachine, error) {

	configSpec.Files = &vimTypes.VirtualMachineFileInfo{
		VmPathName: fmt.Sprintf("[%s]", s.datastore.Name()),
	}

	resVm := res.NewVMForCreate(name)
	err := resVm.Create(ctx, s.dcFolders.VmFolder, s.resourcepool, &configSpec)
	if err != nil {
		return nil, err
	}

	return resVm, nil
}

func (s *Session) cloneVm(ctx context.Context, resSrcVm *res.VirtualMachine, cloneSpec vimTypes.VirtualMachineCloneSpec) (*res.VirtualMachine, error) {

	cloneResVm, err := resSrcVm.Clone(ctx, s.dcFolders.VmFolder, &cloneSpec)
	if err != nil {
		return nil, err
	}

	return cloneResVm, nil
}
