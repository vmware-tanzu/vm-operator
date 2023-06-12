// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"encoding/base64"
	"fmt"

	"k8s.io/utils/pointer"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/vcenter"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/placement"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

// VMCreateArgs contains the arguments needed to create a VM on VC.
type VMCreateArgs struct {
	VMClass             *vmopv1.VirtualMachineClass
	VMImageStatus       *vmopv1.VirtualMachineImageStatus
	VMImageSpec         *vmopv1.VirtualMachineImageSpec
	ResourcePolicy      *vmopv1.VirtualMachineSetResourcePolicy
	VMMetadata          VMMetadata
	ContentLibraryUUID  string
	StorageClassesToIDs map[string]string
	StorageProvisioning string
	HasInstanceStorage  bool

	// From the ResourcePolicy if specified
	ChildResourcePoolName string
	ChildFolderName       string
	MinCPUFreq            uint64

	ConfigSpec          *vimTypes.VirtualMachineConfigSpec
	PlacementConfigSpec *vimTypes.VirtualMachineConfigSpec
	ClassConfigSpec     *vimTypes.VirtualMachineConfigSpec

	FolderMoID       string
	ResourcePoolMoID string
	HostMoID         string
	StorageProfileID string
	DatastoreMoID    string // gce2e only: used if StorageProfileID is unset
}

func (s *Session) deployVMFromCL(
	vmCtx context.VirtualMachineContext,
	item *library.Item,
	createArgs *VMCreateArgs) (*object.VirtualMachine, error) {

	deploymentSpec := vcenter.DeploymentSpec{
		Name:                vmCtx.VM.Name,
		StorageProfileID:    createArgs.StorageProfileID,
		StorageProvisioning: createArgs.StorageProvisioning,
		AcceptAllEULA:       true, // TODO (): Plumb in AcceptAllEULA
	}

	if deploymentSpec.StorageProfileID == "" {
		// Without a storage profile, fall back to the datastore.
		deploymentSpec.DefaultDatastoreID = createArgs.DatastoreMoID
	}

	if lib.IsVMClassAsConfigFSSDaynDateEnabled() && createArgs.ConfigSpec != nil {
		configSpecXML, err := util.MarshalConfigSpecToXML(createArgs.ConfigSpec)
		if err != nil {
			return nil, err
		}

		deploymentSpec.VmConfigSpec = &vcenter.VmConfigSpec{
			Provider: constants.ConfigSpecProviderXML,
			XML:      base64.StdEncoding.EncodeToString(configSpecXML),
		}
	}

	deploy := vcenter.Deploy{
		DeploymentSpec: deploymentSpec,
		Target: vcenter.Target{
			ResourcePoolID: createArgs.ResourcePoolMoID,
			FolderID:       createArgs.FolderMoID,
			HostID:         createArgs.HostMoID,
		},
	}

	vmCtx.Logger.Info("Deploying Library Item", "itemID", item.ID, "deploy", deploy)

	vmMoRef, err := vcenter.NewManager(s.Client.RestClient()).DeployLibraryItem(vmCtx, item.ID, deploy)
	if err != nil {
		return nil, err
	}

	return object.NewVirtualMachine(s.Client.VimClient(), *vmMoRef), nil
}

func (s *Session) cloneVMFromInventory(
	vmCtx context.VirtualMachineContext,
	createArgs *VMCreateArgs) (*object.VirtualMachine, error) {

	srcVMName := vmCtx.VM.Spec.ImageName

	srcVM, err := s.Finder.VirtualMachine(vmCtx, srcVMName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find clone source VM: %s", srcVMName)
	}

	cloneSpec, err := s.createCloneSpec(vmCtx, createArgs, srcVM)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create CloneSpec")
	}

	// We always set cloneSpec.Location.Folder so use that to get the parent folder object.
	folder := object.NewFolder(s.Client.VimClient(), *cloneSpec.Location.Folder)

	vmMoRef, err := res.NewVMFromObject(srcVM).Clone(vmCtx, folder, cloneSpec)
	if err != nil {
		return nil, errors.Wrapf(err, "clone from source VM %s failed", srcVMName)
	}

	return object.NewVirtualMachine(s.Client.VimClient(), *vmMoRef), nil
}

func (s *Session) cloneVMFromContentLibrary(
	vmCtx context.VirtualMachineContext,
	createArgs *VMCreateArgs) (*object.VirtualMachine, error) {

	item, err := s.Client.ContentLibClient().GetLibraryItem(
		vmCtx,
		createArgs.ContentLibraryUUID,
		createArgs.VMImageStatus.ImageName, true)
	if err != nil {
		return nil, err
	}

	switch item.Type {
	case library.ItemTypeOVF:
		return s.deployVMFromCL(vmCtx, item, createArgs)
	case library.ItemTypeVMTX:
		// BMV: Does this work? We'll try to find the source VM with the VM.Spec.ImageName name.
		return s.cloneVMFromInventory(vmCtx, createArgs)
	default:
		return nil, errors.Errorf("item %v not a supported type: %s", item.Name, item.Type)
	}
}

func (s *Session) CreateVirtualMachine(
	vmCtx context.VirtualMachineContext,
	createArgs *VMCreateArgs) (*object.VirtualMachine, error) {

	// The ContentLibraryUUID can be empty when we want to clone from inventory VMs. This is
	// not a supported workflow but we have tests that use this.
	if createArgs.ContentLibraryUUID != "" {
		return s.cloneVMFromContentLibrary(vmCtx, createArgs)
	}

	// Fall back to using the inventory. Note that here is only reachable in our test suite
	// because without SkipVMImageCLProviderCheck we require the VirtualMachineImage to have
	// a ref to the ContentLibrary.
	return s.cloneVMFromInventory(vmCtx, createArgs)
}

func (s *Session) createCloneSpec(
	vmCtx context.VirtualMachineContext,
	createArgs *VMCreateArgs,
	srcVM *object.VirtualMachine) (*vimTypes.VirtualMachineCloneSpec, error) {

	cloneSpec := &vimTypes.VirtualMachineCloneSpec{
		Config: createArgs.ConfigSpec,
		Memory: pointer.Bool(false), // No full memory clones.
	}

	virtualDevices, err := srcVM.Device(vmCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM devices: %w", err)
	}

	virtualDisks := virtualDevices.SelectByType((*vimTypes.VirtualDisk)(nil))

	diskDeviceChanges, err := updateVirtualDiskDeviceChanges(vmCtx, virtualDisks)
	if err != nil {
		return nil, err
	}

	for _, deviceChange := range diskDeviceChanges {
		if deviceChange.GetVirtualDeviceConfigSpec().Operation == vimTypes.VirtualDeviceConfigSpecOperationEdit {
			cloneSpec.Location.DeviceChange = append(cloneSpec.Location.DeviceChange, deviceChange)
		} else {
			cloneSpec.Config.DeviceChange = append(cloneSpec.Config.DeviceChange, deviceChange)
		}
	}

	if createArgs.StorageProfileID != "" {
		cloneSpec.Location.Profile = []vimTypes.BaseVirtualMachineProfileSpec{
			&vimTypes.VirtualMachineDefinedProfileSpec{ProfileId: createArgs.StorageProfileID},
		}
	} else {
		// BMV: Used to compute placement? Otherwise, always overwritten later.
		cloneSpec.Location.Datastore = &vimTypes.ManagedObjectReference{
			Type:  "Datastore",
			Value: createArgs.DatastoreMoID,
		}
	}

	cloneSpec.Location.Pool = &vimTypes.ManagedObjectReference{
		Type:  "ResourcePool",
		Value: createArgs.ResourcePoolMoID,
	}
	cloneSpec.Location.Folder = &vimTypes.ManagedObjectReference{
		Type:  "Folder",
		Value: createArgs.FolderMoID,
	}

	rpOwner, err := object.NewResourcePool(srcVM.Client(), *cloneSpec.Location.Pool).Owner(vmCtx)
	if err != nil {
		return nil, err
	}

	cluster, ok := rpOwner.(*object.ClusterComputeResource)
	if !ok {
		return nil, fmt.Errorf("owner of the ResourcePool is not a cluster but %T", rpOwner)
	}

	relocateSpec, err := placement.CloneVMRelocateSpec(vmCtx, cluster, srcVM.Reference(), cloneSpec)
	if err != nil {
		return nil, err
	}

	cloneSpec.Location.Host = relocateSpec.Host
	cloneSpec.Location.Datastore = relocateSpec.Datastore
	cloneSpec.Location.Disk = cloneVMDiskLocators(virtualDisks, createArgs, cloneSpec.Location)

	return cloneSpec, nil
}

func cloneVMDiskLocators(
	disks object.VirtualDeviceList,
	createArgs *VMCreateArgs,
	location vimTypes.VirtualMachineRelocateSpec) []vimTypes.VirtualMachineRelocateSpecDiskLocator {

	diskLocators := make([]vimTypes.VirtualMachineRelocateSpecDiskLocator, 0, len(disks))

	for _, disk := range disks {
		locator := vimTypes.VirtualMachineRelocateSpecDiskLocator{
			DiskId:    disk.GetVirtualDevice().Key,
			Datastore: *location.Datastore,
			Profile:   location.Profile,
			// TODO: Check if policy is encrypted and use correct DiskMoveType
			DiskMoveType: string(vimTypes.VirtualMachineRelocateDiskMoveOptionsMoveChildMostDiskBacking),
		}

		if backing, ok := disk.(*vimTypes.VirtualDisk).Backing.(*vimTypes.VirtualDiskFlatVer2BackingInfo); ok {
			switch createArgs.StorageProvisioning {
			case string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeThin):
				thin := true
				backing.ThinProvisioned = &thin
			case string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeThick):
				thin := false
				backing.ThinProvisioned = &thin
			case string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeEagerZeroedThick):
				scrub := true
				backing.EagerlyScrub = &scrub
			}
			locator.DiskBackingInfo = backing
		}

		diskLocators = append(diskLocators, locator)
	}

	return diskLocators
}
