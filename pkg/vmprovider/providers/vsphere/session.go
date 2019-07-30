/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"fmt"
	"math"
	"net/url"

	"github.com/vmware/govmomi/vapi/vcenter"

	"github.com/vmware/govmomi/vapi/rest"
	"k8s.io/klog"

	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"

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
	contentlib   *library.Library
	creds        *VSphereVmProviderCredentials
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

	s.creds = config.VcCreds

	if config.ContentSource != "" {
		if err = s.withRestClient(ctx, func(c *rest.Client) error {
			s.contentlib, err = library.NewManager(c).GetLibraryByName(ctx, config.ContentSource)
			return err
		}); err != nil {
			klog.Errorf("failed to init ContentSource %q", config.ContentSource)
		}
	}

	return nil
}

func (s *Session) Logout(ctx context.Context) {
	s.client.Logout(ctx)
}

func (s *Session) ListVirtualMachineImagesFromCL(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error) {
	var items []library.Item
	var err error
	err = s.withRestClient(ctx, func(c *rest.Client) error {
		items, err = library.NewManager(c).GetLibraryItems(ctx, s.contentlib.ID)
		return err
	})
	if err != nil {
		return nil, err
	}

	var images []*v1alpha1.VirtualMachineImage
	for _, item := range items {
		images = append(images, libItemToVirtualMachineImage(&item, namespace))
	}

	return images, err
}

func (s *Session) GetVirtualMachineImageFromCL(ctx context.Context, name string, namespace string) (*v1alpha1.VirtualMachineImage, error) {
	var item *library.Item

	err := s.withRestClient(ctx, func(c *rest.Client) error {
		itemIDs, err := library.NewManager(c).FindLibraryItems(ctx, library.FindItem{LibraryID: s.contentlib.ID, Name: name})
		if err != nil {
			return err
		}

		if len(itemIDs) > 0 {
			//Handle multiple IDs found as an error or return the first one?
			item, err = library.NewManager(c).GetLibraryItem(ctx, itemIDs[0])
		}
		return err
	})

	if err != nil {
		return nil, err
	}
	//Return nil when the image with 'name' is not found in CL
	if item == nil {
		return nil, nil
	}

	return libItemToVirtualMachineImage(item, namespace), nil
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
	vmClass v1alpha1.VirtualMachineClass, vmMetadata vmprovider.VirtualMachineMetadata) (*res.VirtualMachine, error) {

	name := vm.Name
	configSpec, err := s.configSpecFromClassSpec(name, &vm.Spec, &vmClass.Spec, vmMetadata)
	if err != nil {
		return nil, err
	}

	resVm, err := s.createVm(ctx, name, configSpec)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new VM %q", name)
	}

	return resVm, nil
}

func (s *Session) CloneVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine,
	vmClass v1alpha1.VirtualMachineClass, vmMetadata vmprovider.VirtualMachineMetadata) (*res.VirtualMachine, error) {

	name := vm.Name

	if s.contentlib != nil {
		image, err := s.GetVirtualMachineImageFromCL(ctx, vm.Spec.ImageName, vm.Namespace)
		if err != nil {
			return nil, err
		}

		// If image is found, deploy VM from the image
		if image != nil {
			//Get configSpec to honor VM Class
			configSpec, err := s.configSpecFromClassSpec(name, &vm.Spec, &vmClass.Spec, vmMetadata)
			if err != nil {
				return nil, err
			}

			deployedVm, err := s.deployOvf(ctx, image.Status.Uuid, name)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to deploy new VM %q from %q", name, vm.Spec.ImageName)
			}

			//reconfigure with VirtualMachineClass config
			if err := deployedVm.Reconfigure(ctx, configSpec); err != nil {
				//Reconfigure failed: delete poweredOff deployedVM
				if err1 := deployedVm.Delete(ctx); err1 != nil {
					klog.Errorf("failed to delete Stale VM: %q with err: %v", deployedVm.Name, err1)
				}
				return nil, errors.Wrapf(err, "failed to reconfigure new VM %q", deployedVm.Name)
			}

			return deployedVm, nil
		}
	}

	resSrcVm, err := s.lookupVm(ctx, vm.Spec.ImageName)
	if err != nil {
		return nil, err
	}

	cloneSpec, err := s.cloneSpecFromClassSpec(ctx, name, resSrcVm, &vm.Spec, &vmClass.Spec, vmMetadata)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create clone spec from %q", resSrcVm.Name)
	}

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

func cpuQuantityToMhz(q resource.Quantity) int64 {
	return int64(math.Ceil(float64(q.Value()) / float64(1024*1024)))
}

func (s *Session) configSpecFromClassSpec(name string, vmSpec *v1alpha1.VirtualMachineSpec, vmClassSpec *v1alpha1.VirtualMachineClassSpec,
	metadata vmprovider.VirtualMachineMetadata) (*vimTypes.VirtualMachineConfigSpec, error) {

	configSpec := &vimTypes.VirtualMachineConfigSpec{
		Name:     name,
		NumCPUs:  int32(vmClassSpec.Hardware.Cpus),
		MemoryMB: memoryQuantityToMb(vmClassSpec.Hardware.Memory),
	}

	configSpec.CpuAllocation = &vimTypes.ResourceAllocationInfo{}

	if !vmClassSpec.Policies.Resources.Requests.Cpu.IsZero() {
		rsv := cpuQuantityToMhz(vmClassSpec.Policies.Resources.Requests.Cpu)
		configSpec.CpuAllocation.Reservation = &rsv
	}

	if !vmClassSpec.Policies.Resources.Limits.Cpu.IsZero() {
		lim := cpuQuantityToMhz(vmClassSpec.Policies.Resources.Limits.Cpu)
		configSpec.CpuAllocation.Limit = &lim
	}

	configSpec.MemoryAllocation = &vimTypes.ResourceAllocationInfo{}

	if !vmClassSpec.Policies.Resources.Requests.Memory.IsZero() {
		rsv := memoryQuantityToMb(vmClassSpec.Policies.Resources.Requests.Memory)
		configSpec.MemoryAllocation.Reservation = &rsv
	}

	if !vmClassSpec.Policies.Resources.Limits.Memory.IsZero() {
		lim := memoryQuantityToMb(vmClassSpec.Policies.Resources.Limits.Memory)
		configSpec.MemoryAllocation.Limit = &lim
	}

	if vmSpec.VmMetadata != nil {
		switch vmSpec.VmMetadata.Transport {
		case "ExtraConfig":
			var extraConfigs []vimTypes.BaseOptionValue
			for k, v := range metadata {
				extraConfigs = append(extraConfigs, &vimTypes.OptionValue{Key: k, Value: v})
			}
			configSpec.ExtraConfig = extraConfigs
		default:
			return nil, fmt.Errorf("unsupported metadata transport %q", vmSpec.VmMetadata.Transport)
		}
	}

	configSpec.Annotation = fmt.Sprint("Virtual Machine managed by VM Operator")

	return configSpec, nil
}

func (s *Session) cloneSpecFromClassSpec(ctx context.Context, name string, resSrcVm *res.VirtualMachine,
	vmSpec *v1alpha1.VirtualMachineSpec, vmClassSpec *v1alpha1.VirtualMachineClassSpec,
	vmMetadata vmprovider.VirtualMachineMetadata) (*vimTypes.VirtualMachineCloneSpec, error) {
	// TODO(bryanv) The CloneSpec Config is deprecated:
	//  "as of vSphere API 6.0. Use deviceChange in location instead for specifying any virtual
	//  device changes for disks and networks. All other VM configuration changes should use
	//  ReConfigVM_Task API after the clone operation finishes.
	//  TLDR: Don't use the ConfigSpec for virtual dev changes, use the RelocateSpec instead"
	//  Use of ConfigSpec is still supported and required.
	configSpec, err := s.configSpecFromClassSpec(name, vmSpec, vmClassSpec, vmMetadata)
	if err != nil {
		return nil, err
	}

	powerOn := vmSpec.PowerState == v1alpha1.VirtualMachinePoweredOn
	memory := false // No full memory clones

	cloneSpec := &vimTypes.VirtualMachineCloneSpec{
		Config:  configSpec,
		PowerOn: powerOn,
		Memory:  &memory,
	}

	cloneSpec.Location.Datastore = vimTypes.NewReference(s.datastore.Reference())
	cloneSpec.Location.Pool = vimTypes.NewReference(s.resourcepool.Reference())
	//cloneSpec.Location.DiskMoveType = string(vimTypes.VirtualMachineRelocateDiskMoveOptionsMoveAllDiskBackingsAndConsolidate)

	return cloneSpec, nil
}

func (s *Session) createVm(ctx context.Context, name string, configSpec *vimTypes.VirtualMachineConfigSpec) (*res.VirtualMachine, error) {
	configSpec.Files = &vimTypes.VirtualMachineFileInfo{
		VmPathName: fmt.Sprintf("[%s]", s.datastore.Name()),
	}

	resVm := res.NewVMForCreate(name)
	err := resVm.Create(ctx, s.dcFolders.VmFolder, s.resourcepool, configSpec)
	if err != nil {
		return nil, err
	}

	return resVm, nil
}

func (s *Session) cloneVm(ctx context.Context, resSrcVm *res.VirtualMachine, cloneSpec *vimTypes.VirtualMachineCloneSpec) (*res.VirtualMachine, error) {
	cloneResVm, err := resSrcVm.Clone(ctx, s.dcFolders.VmFolder, cloneSpec)
	if err != nil {
		return nil, err
	}

	return cloneResVm, nil
}

func (s *Session) deployOvf(ctx context.Context, itemID string, vmName string) (*res.VirtualMachine, error) {

	var deployment vcenter.Deployment
	var err error
	err = s.withRestClient(ctx, func(c *rest.Client) error {
		manager := vcenter.NewManager(c)

		deploySpec := vcenter.Deploy{
			DeploymentSpec: vcenter.DeploymentSpec{
				Name:               vmName,
				DefaultDatastoreID: s.datastore.Reference().Value,
				// TODO (): Plumb AcceptAllEULA to this Spec
				AcceptAllEULA: true,
			},
			Target: vcenter.Target{
				ResourcePoolID: s.resourcepool.Reference().Value,
			},
		}

		deployment, err = manager.DeployLibraryItem(ctx, itemID, deploySpec)
		return err
	})

	if err != nil {
		return nil, err
	}

	if !deployment.Succeeded {
		return nil, deployment.Error
	}

	ref, err := s.finder.ObjectReference(ctx, vimTypes.ManagedObjectReference{Type: deployment.ResourceID.Type, Value: deployment.ResourceID.ID})
	if err != nil {
		return nil, err
	}

	deployedVM, err := res.NewVMFromObject(ref.(*object.VirtualMachine))

	return deployedVM, nil
}

func (s *Session) withRestClient(ctx context.Context, f func(c *rest.Client) error) error {
	c := rest.NewClient(s.client.VimClient())

	userInfo := url.UserPassword(s.creds.Username, s.creds.Password)

	err := c.Login(ctx, userInfo)
	if err != nil {
		return err
	}

	defer func() {
		if err := c.Logout(ctx); err != nil {
			klog.Errorf("failed to logout: %v", err)
		}
	}()

	return f(c)
}
