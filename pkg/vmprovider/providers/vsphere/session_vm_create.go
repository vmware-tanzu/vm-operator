// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/pbm"
	pbmTypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/vcenter"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/utils/pointer"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

func (s *Session) deployOvf(vmCtx VMCloneContext, itemID string, storageProfileID string) (*res.VirtualMachine, error) {
	deploymentSpec := vcenter.DeploymentSpec{
		Name:                vmCtx.VM.Name,
		StorageProvisioning: vmCtx.StorageProvisioning,
		AcceptAllEULA:       true, // TODO (GCM-509): Plumb in AcceptAllEULA
	}

	if storageProfileID != "" {
		deploymentSpec.StorageProfileID = storageProfileID
	} else {
		// Without a storage profile, fall back to the datastore.
		deploymentSpec.DefaultDatastoreID = s.datastore.Reference().Value
	}

	deploy := vcenter.Deploy{
		DeploymentSpec: deploymentSpec,
		Target: vcenter.Target{
			ResourcePoolID: vmCtx.ResourcePool.Reference().Value,
			FolderID:       vmCtx.Folder.Reference().Value,
		},
	}

	vmCtx.Logger.Info("Deploying Library Item", "itemID", itemID, "deploy", deploy)
	deployedVM, err := vcenter.NewManager(s.Client.RestClient()).DeployLibraryItem(vmCtx, itemID, deploy)
	if err != nil {
		return nil, err
	}

	ref, err := s.Finder.ObjectReference(vmCtx, deployedVM.Reference())
	if err != nil {
		return nil, err
	}

	return res.NewVMFromObject(ref.(*object.VirtualMachine))
}

// getClusterVMConfigOptions fetches the virtualmachine config options from the cluster specifically
// 1. valid guestOS descriptor IDs for the cluster
// 2. default cluster hardware version
func getClusterVMConfigOptions(
	ctx context.Context,
	cluster *object.ClusterComputeResource,
	client *vim25.Client) (map[string]string, int32, error) {
	if cluster == nil {
		return nil, 0, fmt.Errorf("no cluster exists, can't get cluster properties")
	}

	log.V(4).Info("Fetching supported guestOS types and default hardware version for the cluster")
	var computeResource mo.ComputeResource
	if err := cluster.Properties(ctx, cluster.Reference(), []string{"environmentBrowser"}, &computeResource); err != nil {
		log.Error(err, "Failed to get environment browser for the cluster")
		return nil, 0, err
	}

	req := vimTypes.QueryConfigOptionEx{
		This: *computeResource.EnvironmentBrowser,
		Spec: &vimTypes.EnvironmentBrowserConfigOptionQuerySpec{},
	}

	opt, err := methods.QueryConfigOptionEx(ctx, client.RoundTripper, &req)
	if err != nil {
		log.Error(err, "Failed to query config options for cluster properties")
		return nil, 0, err
	}

	guestOSIdsToFamily := make(map[string]string)
	var clusterHwVersion int32
	if opt.Returnval != nil {
		for _, descriptor := range opt.Returnval.GuestOSDescriptor {
			// Fetch all ids and families that have supportLevel other than unsupported
			if descriptor.SupportLevel != "unsupported" {
				guestOSIdsToFamily[descriptor.Id] = descriptor.Family
			}
		}
		// fetch cluster's default hardware version
		clusterHwVersion = opt.Returnval.HardwareOptions.HwVersion
	}

	return guestOSIdsToFamily, clusterHwVersion, nil
}

// checkVMConfigOptions validates
// 1. VMService supported osTypes
// 2. Incompatible cluster hardware versions
func checkVMConfigOptions(
	vmCtx VMCloneContext,
	vmConfigArgs vmprovider.VmConfigArgs,
	clusterHwVersion int32,
	guestOSIdsToFamily map[string]string) error {

	val := vmCtx.VM.Annotations[VMOperatorImageSupportedCheckKey]
	if val != VMOperatorImageSupportedCheckDisable && len(guestOSIdsToFamily) > 0 {
		osType := vmConfigArgs.VmImage.Spec.OSInfo.Type
		// osFamily will be present for supported OSTypes and support only VirtualMachineGuestOsFamilyLinuxGuest for now
		if osFamily := guestOSIdsToFamily[osType]; osFamily != string(vimTypes.VirtualMachineGuestOsFamilyLinuxGuest) {
			return fmt.Errorf("image osType '%s' is not supported by VMService", osType)
		}
	}

	hwVersion := vmConfigArgs.VmImage.Spec.HardwareVersion
	if hwVersion != 0 && clusterHwVersion != 0 && hwVersion > clusterHwVersion {
		return fmt.Errorf("image has a hardware version '%d' higher than "+
			"cluster's default hardware version '%d'", hwVersion, clusterHwVersion)
	}

	return nil
}

func deployVMFromCLPreCheck(
	vmCtx VMCloneContext,
	vmConfigArgs vmprovider.VmConfigArgs,
	cluster *object.ClusterComputeResource,
	client *vim25.Client) error {

	guestOSIdsToFamily, clusterHwVersion, err := getClusterVMConfigOptions(vmCtx.Context, cluster, client)
	if err != nil {
		return errors.Wrapf(err, "Failed to get guestOS descriptors and hardware options from cluster")
	}

	if err := checkVMConfigOptions(vmCtx, vmConfigArgs, clusterHwVersion, guestOSIdsToFamily); err != nil {
		return err
	}

	return nil
}

func (s *Session) deployVMFromCL(vmCtx VMCloneContext, vmConfigArgs vmprovider.VmConfigArgs, item *library.Item) (*res.VirtualMachine, error) {
	vmCtx.Logger.Info("Performing preChecks before deploying library item", "itemName", item.Name, "itemType", item.Type)

	if err := deployVMFromCLPreCheck(vmCtx, vmConfigArgs, s.cluster, s.Client.vimClient); err != nil {
		return nil, errors.Wrapf(err, "deploy VM preCheck failed for image %q", vmCtx.VM.Spec.ImageName)
	}

	vmCtx.Logger.Info("Deploying Content Library item", "itemName", item.Name,
		"itemType", item.Type, "imageName", vmCtx.VM.Spec.ImageName,
		"resourcePolicyName", vmCtx.VM.Spec.ResourcePolicyName, "storageProfileID", vmConfigArgs.StorageProfileID)

	deployedVm, err := s.deployOvf(vmCtx, item.ID, vmConfigArgs.StorageProfileID)
	if err != nil {
		return nil, errors.Wrapf(err, "deploy from content library failed for image %q", vmCtx.VM.Spec.ImageName)
	}

	return deployedVm, nil
}

func (s *Session) cloneVm(vmCtx VMContext, resSrcVm *res.VirtualMachine, cloneSpec *vimTypes.VirtualMachineCloneSpec) (*res.VirtualMachine, error) {
	vmCtx.Logger.Info("Cloning VM", "cloneSpec", *cloneSpec)

	// We always set cloneSpec.Location.Folder so Clone ignores the s.folder param.
	clonedVM, err := resSrcVm.Clone(vmCtx, s.folder, cloneSpec)
	if err != nil {
		return nil, err
	}

	ref, err := s.Finder.ObjectReference(vmCtx, clonedVM.Reference())
	if err != nil {
		return nil, err
	}

	return res.NewVMFromObject(ref.(*object.VirtualMachine))
}

func (s *Session) cloneVMFromInventory(vmCtx VMCloneContext, vmConfigArgs vmprovider.VmConfigArgs) (*res.VirtualMachine, error) {
	sourceVM, err := s.lookupVMByName(vmCtx, vmCtx.VM.Spec.ImageName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to lookup clone source %q", vmCtx.VM.Spec.ImageName)
	}

	cloneSpec, err := s.createCloneSpec(vmCtx, sourceVM, vmConfigArgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create clone spec")
	}

	clonedVM, err := s.cloneVm(vmCtx.VMContext, sourceVM, cloneSpec)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to clone %q from %q", vmCtx.VM.Name, sourceVM.Name)
	}

	return clonedVM, nil
}

func (s *Session) cloneVMFromContentLibrary(vmCtx VMCloneContext, vmConfigArgs vmprovider.VmConfigArgs) (*res.VirtualMachine, error) {
	item, err := s.contentLibProvider.GetLibraryItem(vmCtx, vmConfigArgs.ContentLibraryUUID, vmCtx.VM.Spec.ImageName)
	if err != nil {
		return nil, err
	}

	switch item.Type {
	case library.ItemTypeOVF:
		return s.deployVMFromCL(vmCtx, vmConfigArgs, item)
	case library.ItemTypeVMTX:
		return s.cloneVMFromInventory(vmCtx, vmConfigArgs)
	default:
		return nil, errors.Errorf("item %v not a supported type: %s", item.Name, item.Type)
	}
}

func (s *Session) CloneVirtualMachine(
	vmCtx VMContext,
	vmConfigArgs vmprovider.VmConfigArgs) (*res.VirtualMachine, error) {

	if vmConfigArgs.StorageProfileID == "" {
		if s.storageClassRequired {
			// Note the storageProfileID is obtained from a StorageClass.
			return nil, fmt.Errorf("storage class is required but not specified")
		}

		if s.datastore == nil {
			return nil, fmt.Errorf("cannot clone VM when neither storage class or datastore is specified")
		}
	}

	resourcePool, folder, err := s.getResourcePoolAndFolder(vmCtx, vmConfigArgs.ResourcePolicy)
	if err != nil {
		return nil, err
	}

	storageProvisioning, err := s.getStorageProvisioning(vmCtx, vmConfigArgs.StorageProfileID)
	if err != nil {
		return nil, err
	}

	vmCloneCtx := VMCloneContext{
		VMContext:           vmCtx,
		ResourcePool:        resourcePool,
		Folder:              folder,
		StorageProvisioning: storageProvisioning,
	}

	// The ContentLibraryUUID can be empty when we want to clone from inventory VMs. This is
	// not a supported workflow but we have tests that use this.
	if vmConfigArgs.ContentLibraryUUID != "" {
		resVM, err := s.cloneVMFromContentLibrary(vmCloneCtx, vmConfigArgs)
		return resVM, err
	}

	if s.useInventoryForImages {
		resVM, err := s.cloneVMFromInventory(vmCloneCtx, vmConfigArgs)
		return resVM, err
	}

	return nil, fmt.Errorf("no Content Library specified and inventory disallowed")
}

// policyThickProvision returns true if the storage profile is vSAN and disk provisioning is thick, false otherwise.
// thick provisioning is determined based on its "proportionalCapacity":
// Percentage (0-100) of the logical size of the storage object that will be reserved upon provisioning.
// The UI presents options for "thin" (0%), 25%, 50%, 75% and "thick" (100%)
func policyThickProvision(profile pbmTypes.BasePbmProfile) bool {
	capProfile, ok := profile.(*pbmTypes.PbmCapabilityProfile)
	if !ok {
		return false
	}

	if capProfile.ResourceType.ResourceType != string(pbmTypes.PbmProfileResourceTypeEnumSTORAGE) {
		return false
	}

	if capProfile.ProfileCategory != string(pbmTypes.PbmProfileCategoryEnumREQUIREMENT) {
		return false
	}

	sub, ok := capProfile.Constraints.(*pbmTypes.PbmCapabilitySubProfileConstraints)
	if !ok {
		return false
	}

	for _, p := range sub.SubProfiles {
		for _, capability := range p.Capability {
			if capability.Id.Namespace != "VSAN" || capability.Id.Id != "proportionalCapacity" {
				continue
			}

			for _, c := range capability.Constraint {
				for _, prop := range c.PropertyInstance {
					if prop.Id != capability.Id.Id {
						continue
					}
					if val, ok := prop.Value.(int32); ok {
						// 100% means thick provisioning.
						return val == 100
					}
				}
			}
		}
	}

	return false
}

// getStorageProvisioning gets the storage provisioning VM Spec Advanced Options. If absent, the
// storage profile ID is used to try to determine provisioning.
func (s *Session) getStorageProvisioning(vmCtx VMContext, storageProfileID string) (string, error) {
	// Try to get storage provisioning from VM advanced options section of the spec
	if advOpts := vmCtx.VM.Spec.AdvancedOptions; advOpts != nil && advOpts.DefaultVolumeProvisioningOptions != nil {
		// Webhook validated the combination of provisioning options so we can set to EagerZeroedThick if set.
		if eagerZeroed := advOpts.DefaultVolumeProvisioningOptions.EagerZeroed; eagerZeroed != nil && *eagerZeroed {
			return string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeEagerZeroedThick), nil
		}
		if thinProv := advOpts.DefaultVolumeProvisioningOptions.ThinProvisioned; thinProv != nil {
			if *thinProv {
				return string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeThin), nil
			}
			// Explicitly setting ThinProvisioning to false means use thick provisioning.
			return string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeThick), nil
		}
	}

	if storageProfileID != "" {
		c, err := pbm.NewClient(vmCtx, s.Client.VimClient())
		if err != nil {
			return "", err
		}

		profiles, err := c.RetrieveContent(vmCtx, []pbmTypes.PbmProfileId{{UniqueId: storageProfileID}})
		if err != nil {
			return "", err
		}

		// BMV: Will there only be zero or one profile?
		if len(profiles) > 0 && policyThickProvision(profiles[0]) {
			return string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeThick), nil
		} // Else defer error handling to clone or deploy if storage profile does not exist.
	}

	return string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeThin), nil
}

// createConfigSpec creates the very basic configSpec for the VM when cloning.
func (s *Session) createConfigSpec(name string, vmClassSpec *v1alpha1.VirtualMachineClassSpec) *vimTypes.VirtualMachineConfigSpec {
	configSpec := &vimTypes.VirtualMachineConfigSpec{
		Name:       name,
		Annotation: VCVMAnnotation,
		NumCPUs:    int32(vmClassSpec.Hardware.Cpus),
		MemoryMB:   memoryQuantityToMb(vmClassSpec.Hardware.Memory),
		// Enable clients to differentiate the managed VMs from the regular VMs.
		ManagedBy: &vimTypes.ManagedByInfo{
			ExtensionKey: "com.vmware.vcenter.wcp",
			Type:         "VirtualMachine",
		},
	}

	configSpec.CpuAllocation = &vimTypes.ResourceAllocationInfo{}

	minFreq := s.GetCpuMinMHzInCluster()
	if !vmClassSpec.Policies.Resources.Requests.Cpu.IsZero() {
		rsv := CpuQuantityToMhz(vmClassSpec.Policies.Resources.Requests.Cpu, minFreq)
		configSpec.CpuAllocation.Reservation = &rsv
	}

	if !vmClassSpec.Policies.Resources.Limits.Cpu.IsZero() {
		lim := CpuQuantityToMhz(vmClassSpec.Policies.Resources.Limits.Cpu, minFreq)
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

	return configSpec
}

func (s *Session) createCloneSpec(
	vmCtx VMCloneContext,
	sourceVM *res.VirtualMachine,
	vmConfigArgs vmprovider.VmConfigArgs) (*vimTypes.VirtualMachineCloneSpec, error) {

	cloneSpec := &vimTypes.VirtualMachineCloneSpec{
		Config: s.createConfigSpec(vmCtx.VM.Name, &vmConfigArgs.VmClass.Spec),
		Memory: pointer.BoolPtr(false), // No full memory clones.
	}

	virtualDevices, err := sourceVM.GetVirtualDevices(vmCtx)
	if err != nil {
		return nil, err
	}

	virtualDisks := virtualDevices.SelectByType((*vimTypes.VirtualDisk)(nil))
	virtualNICs := virtualDevices.SelectByType((*vimTypes.VirtualEthernetCard)(nil))

	diskDeviceChanges, err := updateVirtualDiskDeviceChanges(vmCtx.VMContext, virtualDisks)
	if err != nil {
		return nil, err
	}

	ethCardDeviceChanges, err := s.cloneEthCardDeviceChanges(vmCtx, virtualNICs)
	if err != nil {
		return nil, err
	}

	for _, deviceChange := range append(diskDeviceChanges, ethCardDeviceChanges...) {
		if deviceChange.GetVirtualDeviceConfigSpec().Operation == vimTypes.VirtualDeviceConfigSpecOperationEdit {
			cloneSpec.Location.DeviceChange = append(cloneSpec.Location.DeviceChange, deviceChange)
		} else {
			cloneSpec.Config.DeviceChange = append(cloneSpec.Config.DeviceChange, deviceChange)
		}
	}

	if vmConfigArgs.StorageProfileID != "" {
		cloneSpec.Location.Profile = []vimTypes.BaseVirtualMachineProfileSpec{
			&vimTypes.VirtualMachineDefinedProfileSpec{ProfileId: vmConfigArgs.StorageProfileID},
		}
	} else {
		// BMV: Used to compute placement? Otherwise always overwritten later.
		cloneSpec.Location.Datastore = vimTypes.NewReference(s.datastore.Reference())
	}

	cloneSpec.Location.Pool = vimTypes.NewReference(vmCtx.ResourcePool.Reference())
	cloneSpec.Location.Folder = vimTypes.NewReference(vmCtx.Folder.Reference())

	relocateSpec, err := cloneVMRelocateSpec(vmCtx, s.cluster, sourceVM.MoRef(), cloneSpec)
	if err != nil {
		return nil, err
	}
	cloneSpec.Location.Host = relocateSpec.Host
	cloneSpec.Location.Datastore = relocateSpec.Datastore

	diskLocators, err := cloneVMDiskLocators(vmCtx, virtualDisks, cloneSpec.Location.Datastore, cloneSpec.Location.Profile)
	if err != nil {
		return nil, err
	}
	cloneSpec.Location.Disk = diskLocators

	return cloneSpec, nil
}

// cloneEthCardDeviceChanges returns changes for network device changes that need to be performed
// on a new VM being cloned from the source VM.
func (s *Session) cloneEthCardDeviceChanges(
	vmCtx VMCloneContext,
	srcEthCards object.VirtualDeviceList) ([]vimTypes.BaseVirtualDeviceConfigSpec, error) {

	// To ease local and simulation testing, if the VM Spec interfaces is empty, leave the
	// existing interfaces. If a default network as been configured, change the backing for
	// all the existing interfaces. In non-test environments, this can cause confusion
	// because the existing interfaces will end up on some default network so we should
	// work to remove or better guard this.
	if len(vmCtx.VM.Spec.NetworkInterfaces) == 0 {
		if s.network == nil {
			return nil, nil
		}

		backingInfo, err := s.network.EthernetCardBackingInfo(vmCtx)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get backing info for default network %+v", s.network.Reference())
		}

		deviceChanges := make([]vimTypes.BaseVirtualDeviceConfigSpec, 0, len(srcEthCards))
		for _, dev := range srcEthCards {
			dev.GetVirtualDevice().Backing = backingInfo
			deviceChanges = append(deviceChanges, &vimTypes.VirtualDeviceConfigSpec{
				Device:    dev,
				Operation: vimTypes.VirtualDeviceConfigSpecOperationEdit,
			})
		}

		return deviceChanges, nil
	}

	// BMV: Is this really required for cloning, or OK to defer to later update reconcile like OVF deploy?

	netIfList, err := s.ensureNetworkInterfaces(vmCtx.VMContext)
	if err != nil {
		return nil, err
	}

	newEthCards := netIfList.GetVirtualDeviceList()

	// Remove all the existing interfaces, and then add the new interfaces.
	deviceChanges := make([]vimTypes.BaseVirtualDeviceConfigSpec, 0, len(srcEthCards)+len(newEthCards))
	for _, dev := range srcEthCards {
		deviceChanges = append(deviceChanges, &vimTypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimTypes.VirtualDeviceConfigSpecOperationRemove,
		})
	}
	for _, dev := range newEthCards {
		deviceChanges = append(deviceChanges, &vimTypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
		})
	}

	return deviceChanges, nil
}

func cloneVMDiskLocators(
	vmCtx VMCloneContext,
	disks object.VirtualDeviceList,
	datastore *vimTypes.ManagedObjectReference,
	profile []vimTypes.BaseVirtualMachineProfileSpec) ([]vimTypes.VirtualMachineRelocateSpecDiskLocator, error) {

	diskLocators := make([]vimTypes.VirtualMachineRelocateSpecDiskLocator, 0, len(disks))
	for _, disk := range disks {
		locator := vimTypes.VirtualMachineRelocateSpecDiskLocator{
			DiskId:    disk.GetVirtualDevice().Key,
			Datastore: *datastore,
			Profile:   profile,
			// TODO: Check if policy is encrypted and use correct DiskMoveType
			DiskMoveType: string(vimTypes.VirtualMachineRelocateDiskMoveOptionsMoveChildMostDiskBacking),
		}

		if backing, ok := disk.(*vimTypes.VirtualDisk).Backing.(*vimTypes.VirtualDiskFlatVer2BackingInfo); ok {
			switch vmCtx.StorageProvisioning {
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

	return diskLocators, nil
}
