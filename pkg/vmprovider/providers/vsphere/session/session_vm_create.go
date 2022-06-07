// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	goctx "context"
	"encoding/base64"
	"fmt"

	"github.com/pkg/errors"
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

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/placement"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
)

type VirtualMachineCloneContext struct {
	context.VirtualMachineContext
	ResourcePool        *object.ResourcePool
	Folder              *object.Folder
	StorageProvisioning string
	HostID              string
	ConfigSpec          *vimTypes.VirtualMachineConfigSpec
}

func (s *Session) deployOvf(vmCtx VirtualMachineCloneContext, itemID string, storageProfileID string) (*res.VirtualMachine, error) {
	deploymentSpec := vcenter.DeploymentSpec{
		Name:                vmCtx.VM.Name,
		StorageProvisioning: vmCtx.StorageProvisioning,
		AcceptAllEULA:       true, // TODO (): Plumb in AcceptAllEULA
	}

	if storageProfileID != "" {
		deploymentSpec.StorageProfileID = storageProfileID
	} else {
		// Without a storage profile, fall back to the datastore.
		deploymentSpec.DefaultDatastoreID = s.datastore.Reference().Value
	}

	if lib.IsVMClassAsConfigFSSEnabled() && vmCtx.ConfigSpec != nil {
		configSpecBytes, err := virtualmachine.MarshalConfigSpec(*vmCtx.ConfigSpec)
		if err != nil {
			return nil, err
		}

		deploymentSpec.VmConfigSpec = &vcenter.VmConfigSpec{
			XML:      base64.StdEncoding.EncodeToString(configSpecBytes),
			Provider: constants.ConfigSpecProviderXML,
		}
	}

	deploy := vcenter.Deploy{
		DeploymentSpec: deploymentSpec,
		Target: vcenter.Target{
			ResourcePoolID: vmCtx.ResourcePool.Reference().Value,
			FolderID:       vmCtx.Folder.Reference().Value,
			HostID:         vmCtx.HostID,
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

// GetClusterVMConfigOptions fetches the virtualmachine config options from the cluster specifically
// 1. valid guestOS descriptor IDs for the cluster.
func GetClusterVMConfigOptions(
	ctx goctx.Context,
	cluster *object.ClusterComputeResource,
	client *vim25.Client) (map[string]string, error) {
	if cluster == nil {
		return nil, fmt.Errorf("no cluster exists, can't get cluster properties")
	}

	log.V(4).Info("Fetching supported guestOS types and default hardware version for the cluster")
	var computeResource mo.ComputeResource
	if err := cluster.Properties(ctx, cluster.Reference(), []string{"environmentBrowser"}, &computeResource); err != nil {
		log.Error(err, "Failed to get environment browser for the cluster")
		return nil, err
	}

	req := vimTypes.QueryConfigOptionEx{
		This: *computeResource.EnvironmentBrowser,
		Spec: &vimTypes.EnvironmentBrowserConfigOptionQuerySpec{},
	}

	opt, err := methods.QueryConfigOptionEx(ctx, client.RoundTripper, &req)
	if err != nil {
		log.Error(err, "Failed to query config options for cluster properties")
		return nil, err
	}

	guestOSIdsToFamily := make(map[string]string)
	if opt.Returnval != nil {
		for _, descriptor := range opt.Returnval.GuestOSDescriptor {
			// Fetch all ids and families that have supportLevel other than unsupported
			if descriptor.SupportLevel != "unsupported" {
				guestOSIdsToFamily[descriptor.Id] = descriptor.Family
			}
		}
	}

	return guestOSIdsToFamily, nil
}

// CheckVMConfigOptions validates that the specified VM Image has a supported OS type
// Only when FSS_WCP_Unified_TKG is disabled.
func CheckVMConfigOptions(
	vmCtx VirtualMachineCloneContext,
	vmConfigArgs vmprovider.VMConfigArgs,
	guestOSIdsToFamily map[string]string) error {
	if lib.IsUnifiedTKGFSSEnabled() {
		return nil
	}
	val := vmCtx.VM.Annotations[constants.VMOperatorImageSupportedCheckKey]
	if val != constants.VMOperatorImageSupportedCheckDisable && len(guestOSIdsToFamily) > 0 {
		osType := vmConfigArgs.VMImage.Spec.OSInfo.Type
		// osFamily will be present for supported OSTypes and support only VirtualMachineGuestOsFamilyLinuxGuest for now
		if osFamily := guestOSIdsToFamily[osType]; osFamily != string(vimTypes.VirtualMachineGuestOsFamilyLinuxGuest) {
			return fmt.Errorf("image osType '%s' is not supported by VMService", osType)
		}
	}
	return nil
}

func deployVMFromCLPreCheck(
	vmCtx VirtualMachineCloneContext,
	vmConfigArgs vmprovider.VMConfigArgs,
	cluster *object.ClusterComputeResource,
	client *vim25.Client) error {

	guestOSIdsToFamily, err := GetClusterVMConfigOptions(vmCtx.Context, cluster, client)
	if err != nil {
		return errors.Wrapf(err, "Failed to get guestOS descriptors and hardware options from cluster")
	}

	return CheckVMConfigOptions(vmCtx, vmConfigArgs, guestOSIdsToFamily)
}

func (s *Session) deployVMFromCL(vmCtx VirtualMachineCloneContext, vmConfigArgs vmprovider.VMConfigArgs, item *library.Item) (*res.VirtualMachine, error) {
	vmCtx.Logger.Info("Performing preChecks before deploying library item", "itemName", item.Name, "itemType", item.Type)

	if err := deployVMFromCLPreCheck(vmCtx, vmConfigArgs, s.cluster, s.Client.VimClient()); err != nil {
		return nil, errors.Wrapf(err, "deploy VM preCheck failed for image %q", vmCtx.VM.Spec.ImageName)
	}

	vmCtx.Logger.Info("Deploying Content Library item", "itemName", item.Name,
		"itemType", item.Type, "imageName", vmCtx.VM.Spec.ImageName,
		"resourcePolicyName", vmCtx.VM.Spec.ResourcePolicyName, "storageProfileID", vmConfigArgs.StorageProfileID)

	deployedVM, err := s.deployOvf(vmCtx, item.ID, vmConfigArgs.StorageProfileID)
	if err != nil {
		return nil, errors.Wrapf(err, "deploy from content library failed for image %q", vmCtx.VM.Spec.ImageName)
	}

	return deployedVM, nil
}

func (s *Session) cloneVM(vmCtx context.VirtualMachineContext, resSrcVM *res.VirtualMachine, cloneSpec *vimTypes.VirtualMachineCloneSpec) (*res.VirtualMachine, error) {
	vmCtx.Logger.Info("Cloning VM", "cloneSpec", *cloneSpec)

	// We always set cloneSpec.Location.Folder so Clone ignores the s.folder param.
	clonedVM, err := resSrcVM.Clone(vmCtx, s.folder, cloneSpec)
	if err != nil {
		return nil, err
	}

	ref, err := s.Finder.ObjectReference(vmCtx, clonedVM.Reference())
	if err != nil {
		return nil, err
	}

	return res.NewVMFromObject(ref.(*object.VirtualMachine))
}

func (s *Session) cloneVMFromInventory(vmCtx VirtualMachineCloneContext, vmConfigArgs vmprovider.VMConfigArgs) (*res.VirtualMachine, error) {
	sourceVM, err := s.lookupVMByName(vmCtx, vmCtx.VM.Spec.ImageName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to lookup clone source %q", vmCtx.VM.Spec.ImageName)
	}

	cloneSpec, err := s.createCloneSpec(vmCtx, sourceVM, vmConfigArgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create clone spec")
	}

	clonedVM, err := s.cloneVM(vmCtx.VirtualMachineContext, sourceVM, cloneSpec)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to clone %q from %q", vmCtx.VM.Name, sourceVM.Name)
	}

	return clonedVM, nil
}

func (s *Session) cloneVMFromContentLibrary(vmCtx VirtualMachineCloneContext, vmConfigArgs vmprovider.VMConfigArgs) (*res.VirtualMachine, error) {
	item, err := s.Client.ContentLibClient().GetLibraryItem(vmCtx, vmConfigArgs.ContentLibraryUUID, vmConfigArgs.VMImage.Status.ImageName)
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
	vmCtx context.VirtualMachineContext,
	vmConfigArgs vmprovider.VMConfigArgs) (*res.VirtualMachine, error) {

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

	vmCloneCtx := VirtualMachineCloneContext{
		VirtualMachineContext: vmCtx,
		ResourcePool:          resourcePool,
		Folder:                folder,
		StorageProvisioning:   storageProvisioning,
	}

	if lib.IsInstanceStorageFSSEnabled() {
		if hostMOID, ok := vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey]; ok {
			vmCloneCtx.HostID = hostMOID
		}
	}

	vmCloneCtx.ConfigSpec = virtualmachine.CreateConfigSpec(
		vmCtx.VM.Name,
		&vmConfigArgs.VMClass.Spec,
		s.GetCPUMinMHzInCluster())

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
// The UI presents options for "thin" (0%), 25%, 50%, 75% and "thick" (100%).
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
func (s *Session) getStorageProvisioning(vmCtx context.VirtualMachineContext, storageProfileID string) (string, error) {
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

func (s *Session) createCloneSpec(
	vmCtx VirtualMachineCloneContext,
	sourceVM *res.VirtualMachine,
	vmConfigArgs vmprovider.VMConfigArgs) (*vimTypes.VirtualMachineCloneSpec, error) {

	cloneSpec := &vimTypes.VirtualMachineCloneSpec{
		Config: vmCtx.ConfigSpec,
		Memory: pointer.BoolPtr(false), // No full memory clones.
	}

	virtualDevices, err := sourceVM.GetVirtualDevices(vmCtx)
	if err != nil {
		return nil, err
	}

	virtualDisks := virtualDevices.SelectByType((*vimTypes.VirtualDisk)(nil))
	virtualNICs := virtualDevices.SelectByType((*vimTypes.VirtualEthernetCard)(nil))

	diskDeviceChanges, err := updateVirtualDiskDeviceChanges(vmCtx.VirtualMachineContext, virtualDisks)
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

	relocateSpec, err := placement.CloneVMRelocateSpec(vmCtx, s.cluster, sourceVM.MoRef(), cloneSpec)
	if err != nil {
		return nil, err
	}
	cloneSpec.Location.Host = relocateSpec.Host
	cloneSpec.Location.Datastore = relocateSpec.Datastore

	diskLocators := cloneVMDiskLocators(vmCtx, virtualDisks, cloneSpec.Location.Datastore, cloneSpec.Location.Profile)
	cloneSpec.Location.Disk = diskLocators

	return cloneSpec, nil
}

// cloneEthCardDeviceChanges returns changes for network device changes that need to be performed
// on a new VM being cloned from the source VM.
func (s *Session) cloneEthCardDeviceChanges(
	vmCtx VirtualMachineCloneContext,
	srcEthCards object.VirtualDeviceList) ([]vimTypes.BaseVirtualDeviceConfigSpec, error) {

	// BMV: Is this really required for cloning, or OK to defer to later update reconcile like OVF deploy?

	netIfList, err := s.ensureNetworkInterfaces(vmCtx.VirtualMachineContext)
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
	vmCtx VirtualMachineCloneContext,
	disks object.VirtualDeviceList,
	datastore *vimTypes.ManagedObjectReference,
	profile []vimTypes.BaseVirtualMachineProfileSpec) []vimTypes.VirtualMachineRelocateSpecDiskLocator {

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

	return diskLocators
}
