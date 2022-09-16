// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	goctx "context"
	"fmt"
	"sync"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/storage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
)

type vmCreateArgs = session.VMCreateArgs // Until we sort out what the Session becomes

var (
	createCountLock          sync.Mutex
	concurrentCreateCount    int
	maxConcurrentCreateCount = 16 // TODO: If we don't get to CL DeployVOF task stuff, add back some % from max reconciles
)

func (vs *vSphereVMProvider) CreateOrUpdateVirtualMachine(
	ctx goctx.Context,
	vm *vmopv1alpha1.VirtualMachine) error {

	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "createOrUpdateVM")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.GetClient(vmCtx)
	if err != nil {
		return err
	}

	vcVM, err := vcenter.GetVirtualMachine(vmCtx, vs.k8sClient, client.Finder(), nil)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if vcVM == nil {
		vcVM, err = vs.createVirtualMachine(vmCtx, client)
		if err != nil {
			return err
		}

		if vcVM == nil {
			// Creation was not ready or blocked for some reason. We depend on the controller
			// to eventually retry the create.
			return nil
		}
	}

	return vs.updateVirtualMachine(vmCtx, vcVM)
}

func (vs *vSphereVMProvider) DeleteVirtualMachine(
	ctx goctx.Context,
	vm *vmopv1alpha1.VirtualMachine) error {

	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "deleteVM")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vcVM, err := vs.getVM(vmCtx)
	if err != nil {
		return err
	}

	return virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)
}

func (vs *vSphereVMProvider) GetVirtualMachineGuestHeartbeat(
	ctx goctx.Context,
	vm *vmopv1alpha1.VirtualMachine) (vmopv1alpha1.GuestHeartbeatStatus, error) {

	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "heartbeat")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vcVM, err := vs.getVM(vmCtx)
	if err != nil {
		return "", err
	}

	status, err := virtualmachine.GetGuestHeartBeatStatus(vmCtx, vcVM)
	if err != nil {
		return "", err
	}

	return status, nil
}

func (vs *vSphereVMProvider) GetVirtualMachineWebMKSTicket(
	ctx goctx.Context,
	vm *vmopv1alpha1.VirtualMachine,
	pubKey string) (string, error) {

	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "webconsole")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vcVM, err := vs.getVM(vmCtx)
	if err != nil {
		return "", err
	}

	ticket, err := virtualmachine.GetWebConsoleTicket(vmCtx, vcVM, pubKey)
	if err != nil {
		return "", err
	}

	return ticket, nil
}

func (vs *vSphereVMProvider) createVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client) (*object.VirtualMachine, error) {

	createArgs, err := vs.vmCreateGetArgs(vmCtx, vcClient)
	if err != nil {
		return nil, err
	}

	// Historically this is about the point when we say we're creating but there
	// are still several steps before then.
	vmCtx.VM.Status.Phase = vmopv1alpha1.Creating

	err = vs.vmCreateDoPlacement(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	err = vs.vmCreateGetFolderAndRPMoIDs(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	err = vs.vmCreateIsReady(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	// BMV: This is about where we used to do this check but it prb make more sense
	// to do earlier, as to limit wasted work.
	allowed, createDeferFn := vs.vmCreateConcurrentAllowed(vmCtx)
	if !allowed {
		return nil, nil
	}
	defer createDeferFn()

	var vcVM *object.VirtualMachine
	{
		// Hack - create just enough of the Session that's needed for create
		vmCtx.Logger.Info("Creating VirtualMachine")

		ses := &session.Session{
			K8sClient: vs.k8sClient,
			Client:    vcClient,
			Finder:    vcClient.Finder(),
		}

		vcVM, err = ses.CreateVirtualMachine(vmCtx, createArgs)
		if err != nil {
			vmCtx.Logger.Error(err, "CreateVirtualMachine failed")
			return nil, err
		}

		// Set a few Status fields that we easily have on hand here. We will immediately call
		// UpdateVirtualMachine() next which will set it all.
		vmCtx.VM.Status.Phase = vmopv1alpha1.Created
		vmCtx.VM.Status.UniqueID = vcVM.Reference().Value
	}

	return vcVM, nil
}

func (vs *vSphereVMProvider) updateVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vcVM *object.VirtualMachine) error {

	{
		// Hack
		vmCtx.Logger.V(4).Info("Updating VirtualMachine")

		if lib.IsWcpFaultDomainsFSSEnabled() {
			if err := vs.reverseVMZoneLookup(vmCtx); err != nil {
				return err
			}
		}

		ses, err := vs.sessions.GetSessionForVM(vmCtx)
		if err != nil {
			return err
		}

		createArgs, err := vs.vmCreateGetPrereqs(vmCtx)
		if err != nil {
			return err
		}

		// VM update path utilizes the VM class config spec
		createArgs.ConfigSpec = virtualmachine.CreateConfigSpec(
			vmCtx.VM.Name,
			&createArgs.VMClass.Spec,
			createArgs.MinCPUFreq,
			createArgs.ClassConfigSpec)

		vmConfigArgs := vmprovider.VMConfigArgs{
			VMClass:            *createArgs.VMClass,
			VMImage:            createArgs.VMImage,
			ResourcePolicy:     createArgs.ResourcePolicy,
			VMMetadata:         createArgs.VMMetadata,
			StorageProfileID:   createArgs.StorageClassesToIDs[vmCtx.VM.Spec.StorageClass],
			ContentLibraryUUID: createArgs.ContentLibraryUUID,
			ConfigSpec:         createArgs.ConfigSpec,
			MinCPUFreq:         createArgs.MinCPUFreq,
		}

		err = ses.UpdateVirtualMachine(vmCtx, vcVM, vmConfigArgs)
		if err != nil {
			return err
		}
	}

	return nil
}

// vmCreateDoPlacement determines placement of the VM prior to creating the VM on VC.
func (vs *vSphereVMProvider) vmCreateDoPlacement(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *vmCreateArgs) error {

	result, err := placement.Placement(vmCtx, vs.k8sClient, vcClient.VimClient(),
		createArgs.PlacementConfigSpec, createArgs.ChildResourcePoolName)
	if err != nil {
		return err
	}

	if result.PoolMoRef.Value != "" {
		createArgs.ResourcePoolMoID = result.PoolMoRef.Value
	}

	if result.HostMoRef != nil {
		createArgs.HostMoID = result.HostMoRef.Value
	}

	if result.InstanceStoragePlacement {
		hostMoID := createArgs.HostMoID

		if hostMoID == "" {
			return fmt.Errorf("placement result missing host required for instance storage")
		}

		hostFQDN, err := vcenter.GetESXHostFQDN(vmCtx, vcClient.VimClient(), hostMoID)
		if err != nil {
			return err
		}

		if vmCtx.VM.Annotations == nil {
			vmCtx.VM.Annotations = map[string]string{}
		}
		vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = hostMoID
		vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey] = hostFQDN
	}

	if result.ZonePlacement {
		if vmCtx.VM.Labels == nil {
			vmCtx.VM.Labels = map[string]string{}
		}
		vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] = result.ZoneName
	}

	return nil
}

// vmCreateGetFolderAndRPMoIDs gets the MoIDs of the Folder and Resource Pool the VM will be created under.
func (vs *vSphereVMProvider) vmCreateGetFolderAndRPMoIDs(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *vmCreateArgs) error {

	if createArgs.ResourcePoolMoID == "" {
		// We did not do placement so find this namespace/zone ResourcePool and Folder.

		nsFolderMoID, rpMoID, err := topology.GetNamespaceFolderAndRPMoID(vmCtx, vs.k8sClient,
			vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey], vmCtx.VM.Namespace)
		if err != nil {
			return err
		}

		// If this VM has a ResourcePolicy ResourcePool, lookup the child ResourcePool under the
		// namespace/zone's root ResourcePool. This will be the VM's ResourcePool.
		if createArgs.ChildResourcePoolName != "" {
			parentRP := object.NewResourcePool(vcClient.VimClient(),
				types.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID})

			childRP, err := vcenter.GetChildResourcePool(vmCtx, parentRP, createArgs.ChildResourcePoolName)
			if err != nil {
				return err
			}

			rpMoID = childRP.Reference().Value
		}

		createArgs.ResourcePoolMoID = rpMoID
		createArgs.FolderMoID = nsFolderMoID

	} else {
		// Placement already selected the ResourcePool/Cluster, so we just need this namespace's folder.
		nsFolderMoID, err := topology.GetNamespaceFolderMoID(vmCtx, vs.k8sClient, vmCtx.VM.Namespace)
		if err != nil {
			return err
		}

		createArgs.FolderMoID = nsFolderMoID
	}

	// If this VM has a ResourcePolicy Folder, lookup the child Folder under the namespace's Folder.
	// This will be the VM's parent Folder in the VC inventory.
	if createArgs.ChildFolderName != "" {
		parentFolder := object.NewFolder(vcClient.VimClient(),
			types.ManagedObjectReference{Type: "Folder", Value: createArgs.FolderMoID})

		childFolder, err := vcenter.GetChildFolder(vmCtx, parentFolder, createArgs.ChildFolderName)
		if err != nil {
			return err
		}

		createArgs.FolderMoID = childFolder.Reference().Value
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateIsReady(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *vmCreateArgs) error {

	if policy := createArgs.ResourcePolicy; policy != nil {
		if !policy.DeletionTimestamp.IsZero() {
			return fmt.Errorf("cannot create VirtualMachine when its resource policy is being deleted")
		}

		clusterMoRef, err := vcenter.GetResourcePoolOwnerMoRef(vmCtx, vcClient.VimClient(), createArgs.ResourcePoolMoID)
		if err != nil {
			return err
		}

		// TODO: May want to do this as to filter the placement candidates.
		exists, err := vs.doClusterModulesExist(vmCtx, vcClient.ClusterModuleClient(), clusterMoRef, policy)
		if err != nil {
			return err
		} else if !exists {
			return fmt.Errorf("VirtualMachineSetResourcePolicy cluster modules is not ready")
		}
	}

	if createArgs.HasInstanceStorage {
		if _, ok := vmCtx.VM.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey]; !ok {
			return fmt.Errorf("instance storage PVCs are not bound yet")
		}
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateConcurrentAllowed(vmCtx context.VirtualMachineContext) (bool, func()) {
	createCountLock.Lock()
	if concurrentCreateCount >= maxConcurrentCreateCount {
		createCountLock.Unlock()
		vmCtx.Logger.Info("Too many create VirtualMachine already occurring. Re-queueing request")
		return false, nil
	}

	concurrentCreateCount++
	createCountLock.Unlock()

	decrementFn := func() {
		createCountLock.Lock()
		concurrentCreateCount--
		createCountLock.Unlock()
	}

	return true, decrementFn
}

func (vs *vSphereVMProvider) vmCreateGetArgs(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client) (*vmCreateArgs, error) {

	createArgs, err := vs.vmCreateGetPrereqs(vmCtx)
	if err != nil {
		return nil, err
	}

	if lib.IsInstanceStorageFSSEnabled() {
		// This must be done here so the instance storage volumes are present so the next
		// step can fetch all the storage profiles.
		if err := AddInstanceStorageVolumes(vmCtx, createArgs.VMClass); err != nil {
			return nil, err
		}

		createArgs.HasInstanceStorage = instancestorage.IsConfigured(vmCtx.VM)
	}

	err = vs.vmCreateGetStoragePrereqs(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	vs.vmCreateGenConfigSpec(vmCtx, createArgs)

	err = vs.vmCreateValidateArgs(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	return createArgs, nil
}

// vmCreateGetPrereqs returns the vmCreateArgs populated with the k8s objects required to
// create the VM on VC.
func (vs *vSphereVMProvider) vmCreateGetPrereqs(
	vmCtx context.VirtualMachineContext) (*vmCreateArgs, error) {

	vmClass, err := GetVirtualMachineClass(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	vmImage, clUUID, err := GetVMImageAndContentLibraryUUID(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	resourcePolicy, err := GetVMSetResourcePolicy(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	vmMetadata, err := GetVMMetadata(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	createArgs := &vmCreateArgs{}
	createArgs.VMClass = vmClass
	createArgs.VMImage = vmImage
	createArgs.ContentLibraryUUID = clUUID
	createArgs.VMMetadata = vmMetadata

	// TODO: Perhaps a condition type for each resource is better so all missing one(s)
	// 	     can be reported at once (and will help for the best-effort update changes).
	// This is about where historically we set this condition but there are still a lot
	// more checks to go.
	conditions.MarkTrue(vmCtx.VM, vmopv1alpha1.VirtualMachinePrereqReadyCondition)

	if resourcePolicy != nil {
		rp := resourcePolicy.Spec.ResourcePool

		createArgs.ResourcePolicy = resourcePolicy
		createArgs.ChildFolderName = resourcePolicy.Spec.Folder.Name
		createArgs.ChildResourcePoolName = rp.Name
		if !rp.Reservations.Cpu.IsZero() || !rp.Limits.Cpu.IsZero() {
			freq, err := vs.getOrComputeCPUMinFrequency(vmCtx)
			if err != nil {
				return nil, err
			}
			createArgs.MinCPUFreq = freq
		}
	}

	if lib.IsVMClassAsConfigFSSEnabled() {
		if cs := createArgs.VMClass.Spec.ConfigSpec; cs != nil {
			if cs.XML != "" {
				// TODO: Have a single func that unmarshals & excludes
				classConfigSpec, err := util.UnmarshalConfigSpecFromBase64XML([]byte(cs.XML))
				if err != nil {
					return nil, err
				}
				util.ProcessVMClassConfigSpecExclusions(classConfigSpec)
				createArgs.ClassConfigSpec = classConfigSpec
			}
		}
	}

	return createArgs, nil
}

func (vs *vSphereVMProvider) vmCreateGetStoragePrereqs(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *vmCreateArgs) error {

	storageClassesToIDs, err := storage.GetVMStoragePoliciesIDs(vmCtx, vs.k8sClient)
	if err != nil {
		return err
	}

	vmStorageProfileID := storageClassesToIDs[vmCtx.VM.Spec.StorageClass]

	provisioningType, err := virtualmachine.GetDefaultDiskProvisioningType(vmCtx, vcClient, vmStorageProfileID)
	if err != nil {
		return err
	}

	createArgs.StorageClassesToIDs = storageClassesToIDs
	createArgs.StorageProvisioning = provisioningType
	createArgs.StorageProfileID = vmStorageProfileID

	return nil
}

func (vs *vSphereVMProvider) vmCreateGenConfigSpec(
	vmCtx context.VirtualMachineContext,
	createArgs *vmCreateArgs) {

	var vmClassConfigSpec *types.VirtualMachineConfigSpec
	if createArgs.ClassConfigSpec != nil {
		// With DaynDate FFS, the VM created is based on the VMClass ConfigSpec. Otherwise, the VMClass
		// ConfigSpec is handled during the post-create Update.
		if lib.IsVMClassAsConfigFSSDaynDateEnabled() {
			t := *createArgs.ClassConfigSpec
			// Remove the NICs since we don't know the backing yet; they'll be added in the post-create Update.
			util.RemoveDevicesFromConfigSpec(&t, util.IsEthernetCard)
			vmClassConfigSpec = &t
		}
	}

	// Create both until we can sanitize the placement ConfigSpec for creation.

	createArgs.ConfigSpec = virtualmachine.CreateConfigSpec(
		vmCtx.VM.Name,
		&createArgs.VMClass.Spec,
		createArgs.MinCPUFreq,
		vmClassConfigSpec)

	createArgs.PlacementConfigSpec = virtualmachine.CreateConfigSpecForPlacement(
		vmCtx,
		&createArgs.VMClass.Spec,
		createArgs.MinCPUFreq,
		createArgs.StorageClassesToIDs,
		vmClassConfigSpec)
}

func (vs *vSphereVMProvider) vmCreateValidateArgs(
	vmCtx context.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *vmCreateArgs) error {

	// Some of this would be better done in the validation webhook but have it here for now.
	cfg := vcClient.Config()

	if cfg.StorageClassRequired {
		// In WCP this is always required.
		if vmCtx.VM.Spec.StorageClass == "" {
			return fmt.Errorf("StorageClass is required but not specified")
		}

		if createArgs.StorageProfileID == "" {
			// GetVMStoragePoliciesIDs() would have returned an error if the policy didn't exist, but
			// ensure the field is set.
			return fmt.Errorf("no StorageProfile found for StorageClass %s", vmCtx.VM.Spec.StorageClass)
		}

	} else if vmCtx.VM.Spec.StorageClass == "" {
		// This is only set in gce2e.
		if cfg.Datastore == "" {
			return fmt.Errorf("no Datastore provided in configuration")
		}

		datastore, err := vcClient.Finder().Datastore(vmCtx, cfg.Datastore)
		if err != nil {
			return fmt.Errorf("failed to find Datastore %s: %w", cfg.Datastore, err)
		}

		createArgs.DatastoreMoID = datastore.Reference().Value
	}

	return nil
}
