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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"

	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

const (
	DefaultEthernetCardType = "vmxnet3"
)

type Session struct {
	client *Client

	Finder       *find.Finder
	datacenter   *object.Datacenter
	cluster      *object.ClusterComputeResource
	folder       *object.Folder
	resourcepool *object.ResourcePool
	datastore    *object.Datastore
	contentlib   *library.Library
	creds        *VSphereVmProviderCredentials
	usePlaceVM   bool //Used to avoid calling PlaceVM in integration tests (since PlaceVm is not implemented in vcsim yet)
}

func NewSessionAndConfigure(ctx context.Context, config *VSphereVmProviderConfig) (*Session, error) {
	s, err := NewSession(ctx, config)
	if err != nil {
		return nil, err
	}

	if err = s.ConfigureContent(ctx, config.ContentSource); err != nil {
		return nil, err
	}

	return s, nil
}

func NewSession(ctx context.Context, config *VSphereVmProviderConfig) (*Session, error) {
	c, err := NewClient(ctx, config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create client for new session")
	}

	s := &Session{
		client:     c,
		usePlaceVM: !config.AvoidUsingPlaceVM,
	}

	if err = s.initSession(ctx, config); err != nil {
		s.Logout(ctx)
		return nil, err
	}

	return s, nil
}

func (s *Session) initSession(ctx context.Context, config *VSphereVmProviderConfig) error {
	s.Finder = find.NewFinder(s.client.VimClient(), false)

	dc, err := s.Finder.Datacenter(ctx, config.Datacenter)
	if err != nil {
		return errors.Wrapf(err, "failed to init Datacenter %q", config.Datacenter)
	}

	s.datacenter = dc
	s.Finder.SetDatacenter(dc)

	s.resourcepool, err = GetResourcePool(ctx, s.Finder, config.ResourcePool)
	if err != nil {
		return errors.Wrapf(err, "failed to init Resource Pool %q", config.ResourcePool)
	}

	s.folder, err = GetVMFolder(ctx, s.Finder, config.Folder)
	if err != nil {
		return errors.Wrapf(err, "failed to init folder %q", config.Folder)
	}

	s.cluster, err = GetResourcePoolOwner(ctx, s.resourcepool)
	if err != nil {
		return errors.Wrapf(err, "failed to init cluster %q", config.ResourcePool)
	}

	s.datastore, err = s.Finder.Datastore(ctx, config.Datastore)
	if err != nil {
		return errors.Wrapf(err, "failed to init Datastore %q", config.Datastore)
	}

	s.creds = config.VcCreds

	return nil
}

func (s *Session) ConfigureContent(ctx context.Context, contentSource string) error {
	if contentSource == "" {
		return nil
	}

	var err error
	if err = s.WithRestClient(ctx, func(c *rest.Client) error {
		s.contentlib, err = library.NewManager(c).GetLibraryByName(ctx, contentSource)
		return err
	}); err != nil {
		return errors.Wrapf(err, "failed to init Content Library %q", contentSource)
	}

	return nil
}

func (s *Session) Logout(ctx context.Context) {
	s.client.Logout(ctx)
}

func (s *Session) ListVirtualMachineImagesFromCL(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error) {
	var items []library.Item
	var err error
	err = s.WithRestClient(ctx, func(c *rest.Client) error {
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

	err := s.WithRestClient(ctx, func(c *rest.Client) error {
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

	objVms, err := s.Finder.VirtualMachineList(ctx, path)
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

	deviceSpecs, err := s.deviceSpecsFromVMSpec(ctx, &vm.Spec)
	if err != nil {
		return nil, err
	}

	name := vm.Name
	configSpec, err := configSpecFromClassSpec(name, &vm.Spec, &vmClass.Spec, vmMetadata, deviceSpecs)
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
	vmClass v1alpha1.VirtualMachineClass, vmMetadata vmprovider.VirtualMachineMetadata, profileID string) (*res.VirtualMachine, error) {
	name := vm.Name

	if s.contentlib != nil {
		image, err := s.GetVirtualMachineImageFromCL(ctx, vm.Spec.ImageName, vm.Namespace)
		if err != nil {
			return nil, err
		}

		// If image is found, deploy VM from the image
		if image != nil {
			deployedVm, err := s.deployOvf(ctx, image.Status.Uuid, name)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to deploy new VM %q from %q", name, vm.Spec.ImageName)
			}

			// Add device change specs to configSpec
			deviceSpecs, err := s.deviceChangeSpecs(ctx, &vm.Spec, deployedVm)
			if err != nil {
				return nil, err
			}

			// Get configSpec to honor VM Class
			configSpec, err := configSpecFromClassSpec(name, &vm.Spec, &vmClass.Spec, vmMetadata, deviceSpecs)
			if err != nil {
				return nil, err
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

	cloneSpec, err := s.getCloneSpec(ctx, name, resSrcVm, &vm.Spec, &vmClass.Spec, vmMetadata, profileID)
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
	objVm, err := s.Finder.VirtualMachine(ctx, name)
	if err != nil {
		return nil, err
	}

	return res.NewVMFromObject(objVm)
}

func memoryQuantityToMb(q resource.Quantity) int64 {
	return int64(math.Ceil(float64(q.Value()) / float64(1024*1024)))
}

func cpuQuantityToMhz(q resource.Quantity) int64 {
	return int64(math.Ceil(float64(q.Value()) / float64(1000*1000)))
}

func (s *Session) deviceSpecsFromVMSpec(ctx context.Context, vmSpec *v1alpha1.VirtualMachineSpec) ([]vimTypes.BaseVirtualDeviceConfigSpec, error) {
	var deviceSpecs []vimTypes.BaseVirtualDeviceConfigSpec

	// The clients should ensure that existing device keys are not reused as temporary key values for the new device to be added, hence
	//  use unique negative integers as temporary keys.
	key := int32(-100)
	for _, vif := range vmSpec.NetworkInterfaces {
		// Add new NICs based on the vm spec
		// TODO: Validate the NIC type based on known types
		ethCardType := vif.EthernetCardType
		if vif.EthernetCardType == "" {
			ethCardType = DefaultEthernetCardType
		}

		ref, err := s.Finder.Network(ctx, vif.NetworkName)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to find network %q", vif.NetworkName)
		}
		backing, err := ref.EthernetCardBackingInfo(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to create new ethernet card backing info for network %q", vif.NetworkName)
		}
		dev, err := object.EthernetCardTypes().CreateEthernetCard(ethCardType, backing)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to create new ethernet card %q for network %q", ethCardType, vif.NetworkName)
		}

		// Get the actual NIC object. This is safe to assert without a check
		// because "object.EthernetCardTypes().CreateEthernetCard" returns a
		// "types.BaseVirtualEthernetCard" as a "types.BaseVirtualDevice".
		nic := dev.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()

		// Assign a temporary device key to ensure that a unique one will be
		// generated when the device is created.
		nic.Key = key

		deviceSpecs = append(deviceSpecs, &vimTypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
		})

		key--
	}
	return deviceSpecs, nil
}

func (s *Session) deviceChangeSpecs(ctx context.Context, vmSpec *v1alpha1.VirtualMachineSpec, resSrcVm *res.VirtualMachine) ([]vimTypes.BaseVirtualDeviceConfigSpec, error) {
	// Note: If no network interface is specified in the vm spec we don't remove existing interfaces while cloning.
	if len(vmSpec.NetworkInterfaces) == 0 {
		return nil, nil
	}

	netDevices, err := resSrcVm.GetNetworkDevices(ctx)
	if err != nil {
		return nil, err
	}

	var deviceSpecs []vimTypes.BaseVirtualDeviceConfigSpec

	// Remove any existing NICs
	for _, dev := range netDevices {
		deviceSpecs = append(deviceSpecs, &vimTypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimTypes.VirtualDeviceConfigSpecOperationRemove,
		})
	}

	// Add new NICs
	addDeviceSpecs, err := s.deviceSpecsFromVMSpec(ctx, vmSpec)
	if err != nil {
		return nil, err
	}
	deviceSpecs = append(deviceSpecs, addDeviceSpecs...)

	return deviceSpecs, nil
}

func processStorageClass(ctx context.Context, resSrcVM *res.VirtualMachine, profileID string) ([]types.BaseVirtualDeviceConfigSpec, []vimTypes.BaseVirtualMachineProfileSpec, error) {
	if len(profileID) == 0 {
		return nil, nil, nil
	}

	disks, err := resSrcVM.GetVirtualDisks(ctx)
	if err != nil {
		return nil, nil, err
	}
	vdcs, err := disks.ConfigSpec(vimTypes.VirtualDeviceConfigSpecOperationEdit)
	if err != nil {
		return nil, nil, err
	}

	var vmProfile []vimTypes.BaseVirtualMachineProfileSpec
	profileSpec := &vimTypes.VirtualMachineDefinedProfileSpec{ProfileId: profileID}
	vmProfile = append(vmProfile, profileSpec)

	for _, cs := range vdcs {
		cs.GetVirtualDeviceConfigSpec().Profile = vmProfile
		cs.GetVirtualDeviceConfigSpec().FileOperation = ""
	}

	return vdcs, vmProfile, nil
}

func (s *Session) getCloneSpec(ctx context.Context, name string, resSrcVM *res.VirtualMachine,
	vmSpec *v1alpha1.VirtualMachineSpec, vmClassSpec *v1alpha1.VirtualMachineClassSpec,
	vmMetadata vmprovider.VirtualMachineMetadata, profileID string) (*vimTypes.VirtualMachineCloneSpec, error) {

	vdcs, vmProfile, err := processStorageClass(ctx, resSrcVM, profileID)
	if err != nil {
		return nil, err
	}
	deviceSpecs, err := s.deviceChangeSpecs(ctx, vmSpec, resSrcVM)
	if err != nil {
		return nil, err
	}
	deviceSpecs = append(deviceSpecs, vdcs...)

	configSpec, err := configSpecFromClassSpec(name, vmSpec, vmClassSpec, vmMetadata, deviceSpecs)
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

	cloneSpec.Location.Pool = vimTypes.NewReference(s.resourcepool.Reference())

	vmRef := &vimTypes.ManagedObjectReference{Type: "VirtualMachine", Value: resSrcVM.ReferenceValue()}
	if s.usePlaceVM {
		rSpec, err := computeVMPlacement(ctx, s.cluster, vmRef, cloneSpec, vimTypes.PlacementSpecPlacementTypeClone)
		if err != nil {
			return nil, err
		}
		cloneSpec.Location = *rSpec
	} else {
		cloneSpec.Location.Datastore = vimTypes.NewReference(s.datastore.Reference())
		klog.Warningf("Skipping call to PlaceVM. Using preconfigured datastore: %v", s.datastore)
	}
	//cloneSpec.Location.DiskMoveType = string(vimTypes.VirtualMachineRelocateDiskMoveOptionsMoveAllDiskBackingsAndConsolidate)

	cloneSpec.Location.Profile = vmProfile

	return cloneSpec, nil
}

func (s *Session) createVm(ctx context.Context, name string, configSpec *vimTypes.VirtualMachineConfigSpec) (*res.VirtualMachine, error) {
	configSpec.Files = &vimTypes.VirtualMachineFileInfo{
		VmPathName: fmt.Sprintf("[%s]", s.datastore.Name()),
	}

	resVm := res.NewVMForCreate(name)
	err := resVm.Create(ctx, s.folder, s.resourcepool, configSpec)
	if err != nil {
		return nil, err
	}

	return resVm, nil
}

func (s *Session) cloneVm(ctx context.Context, resSrcVm *res.VirtualMachine, cloneSpec *vimTypes.VirtualMachineCloneSpec) (*res.VirtualMachine, error) {
	cloneResVm, err := resSrcVm.Clone(ctx, s.folder, cloneSpec)
	if err != nil {
		return nil, err
	}

	return cloneResVm, nil
}

func (s *Session) deployOvf(ctx context.Context, itemID string, vmName string) (*res.VirtualMachine, error) {

	var deployment *types.ManagedObjectReference
	var err error
	err = s.WithRestClient(ctx, func(c *rest.Client) error {
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

	ref, err := s.Finder.ObjectReference(ctx, vimTypes.ManagedObjectReference{Type: deployment.Type, Value: deployment.Value})
	if err != nil {
		return nil, err
	}

	deployedVM, err := res.NewVMFromObject(ref.(*object.VirtualMachine))

	return deployedVM, nil
}

func (s *Session) WithRestClient(ctx context.Context, f func(c *rest.Client) error) error {
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

func configSpecFromClassSpec(name string, vmSpec *v1alpha1.VirtualMachineSpec, vmClassSpec *v1alpha1.VirtualMachineClassSpec,
	metadata vmprovider.VirtualMachineMetadata, deviceSpecs []vimTypes.BaseVirtualDeviceConfigSpec) (*vimTypes.VirtualMachineConfigSpec, error) {

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

	configSpec.DeviceChange = deviceSpecs

	return configSpec, nil
}

// GetPool returns resource pool for a given invt path of a moref
func GetResourcePool(ctx context.Context, finder *find.Finder, rp string) (*object.ResourcePool, error) {
	ref := types.ManagedObjectReference{Type: "ResourcePool", Value: rp}
	if o, err := finder.ObjectReference(ctx, ref); err == nil {
		return o.(*object.ResourcePool), nil
	}
	return finder.ResourcePool(ctx, rp)
}

// GetPool returns VM folder for a given invt path of a moref
func GetVMFolder(ctx context.Context, finder *find.Finder, folder string) (*object.Folder, error) {
	ref := types.ManagedObjectReference{Type: "Folder", Value: folder}
	if o, err := finder.ObjectReference(ctx, ref); err == nil {
		return o.(*object.Folder), nil
	}
	return finder.Folder(ctx, folder)
}
