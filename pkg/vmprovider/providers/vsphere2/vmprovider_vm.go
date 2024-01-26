// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	goctx "context"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/session"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/network"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/placement"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/storage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/vmlifecycle"
)

// VMCreateArgs contains the arguments needed to create a VM on VC.
type VMCreateArgs struct {
	vmlifecycle.CreateArgs
	vmlifecycle.BootstrapData

	VMClass        *vmopv1.VirtualMachineClass
	ResourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
	ImageObj       ctrlclient.Object
	ImageSpec      *vmopv1.VirtualMachineImageSpec
	ImageStatus    *vmopv1.VirtualMachineImageStatus

	StorageClassesToIDs   map[string]string
	HasInstanceStorage    bool
	ChildResourcePoolName string
	ChildFolderName       string
	ClusterMoRef          types.ManagedObjectReference

	NetworkResults network.NetworkInterfaceResults
}

// TODO: Until we sort out what the Session becomes.
type vmUpdateArgs = session.VMUpdateArgs

var (
	createCountLock       sync.Mutex
	concurrentCreateCount int

	// SkipVMImageCLProviderCheck skips the checks that a VM Image has a Content Library item provider
	// since a VirtualMachineImage created for a VM template won't have either. This has been broken for
	// a long time but was otherwise masked on how the tests used to be organized.
	SkipVMImageCLProviderCheck = false
)

func (vs *vSphereVMProvider) CreateOrUpdateVirtualMachine(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine) error {

	vmCtx := context.VirtualMachineContextA2{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "createOrUpdateVM")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return err
	}

	vcVM, err := vs.getVM(vmCtx, client, false)
	if err != nil {
		return err
	}

	if vcVM == nil {
		var createArgs *VMCreateArgs

		vcVM, createArgs, err = vs.createVirtualMachine(vmCtx, client)
		if err != nil {
			return err
		}

		if vcVM == nil {
			// Creation was not ready or blocked for some reason. We depend on the controller
			// to eventually retry the create.
			return nil
		}

		return vs.createdVirtualMachineFallthroughUpdate(vmCtx, vcVM, client, createArgs)
	}

	return vs.updateVirtualMachine(vmCtx, vcVM, client, nil)
}

func (vs *vSphereVMProvider) DeleteVirtualMachine(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine) error {

	vmCtx := context.VirtualMachineContextA2{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "deleteVM")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return err
	}

	vcVM, err := vs.getVM(vmCtx, client, false)
	if err != nil {
		return err
	} else if vcVM == nil {
		// VM does not exist.
		return nil
	}

	return virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)
}

func (vs *vSphereVMProvider) PublishVirtualMachine(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine,
	vmPub *vmopv1.VirtualMachinePublishRequest,
	cl *imgregv1a1.ContentLibrary,
	actID string) (string, error) {

	vmCtx := context.VirtualMachineContextA2{
		Context: ctx,
		// Update logger info
		Logger: log.WithValues("vmName", vm.NamespacedName()).
			WithValues("clName", fmt.Sprintf("%s/%s", cl.Namespace, cl.Name)).
			WithValues("vmPubName", fmt.Sprintf("%s/%s", vmPub.Namespace, vmPub.Name)),
		VM: vm,
	}

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get vCenter client")
	}

	itemID, err := virtualmachine.CreateOVF(vmCtx, client.RestClient(), vmPub, cl, actID)
	if err != nil {
		return "", err
	}

	return itemID, nil
}

func (vs *vSphereVMProvider) GetVirtualMachineGuestHeartbeat(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error) {

	vmCtx := context.VirtualMachineContextA2{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "heartbeat")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return "", err
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return "", err
	}

	status, err := virtualmachine.GetGuestHeartBeatStatus(vmCtx, vcVM)
	if err != nil {
		return "", err
	}

	return status, nil
}

func (vs *vSphereVMProvider) GetVirtualMachineGuestInfo(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine) (map[string]string, error) {

	vmCtx := context.VirtualMachineContextA2{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "guestInfo")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return nil, err
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return nil, err
	}

	guestInfo, err := virtualmachine.GetExtraConfigGuestInfo(vmCtx, vcVM)
	if err != nil {
		return nil, err
	}

	return guestInfo, nil
}

func (vs *vSphereVMProvider) GetVirtualMachineWebMKSTicket(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine,
	pubKey string) (string, error) {

	vmCtx := context.VirtualMachineContextA2{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "webconsole")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return "", err
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return "", err
	}

	ticket, err := virtualmachine.GetWebConsoleTicket(vmCtx, vcVM, pubKey)
	if err != nil {
		return "", err
	}

	return ticket, nil
}

func (vs *vSphereVMProvider) GetVirtualMachineHardwareVersion(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine) (int32, error) {

	vmCtx := context.VirtualMachineContextA2{
		Context: goctx.WithValue(ctx, types.ID{}, vs.getOpID(vm, "hardware-version")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return 0, err
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return 0, err
	}

	var o mo.VirtualMachine
	err = vcVM.Properties(vmCtx, vcVM.Reference(), []string{"config.version"}, &o)
	if err != nil {
		return 0, err
	}

	return contentlibrary.ParseVirtualHardwareVersion(o.Config.Version), nil
}

func (vs *vSphereVMProvider) createVirtualMachine(
	vmCtx context.VirtualMachineContextA2,
	vcClient *vcclient.Client) (*object.VirtualMachine, *VMCreateArgs, error) {

	createArgs, err := vs.vmCreateGetArgs(vmCtx, vcClient)
	if err != nil {
		return nil, nil, err
	}

	err = vs.vmCreateDoPlacement(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, nil, err
	}

	err = vs.vmCreateGetFolderAndRPMoIDs(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, nil, err
	}

	err = vs.vmCreateFixupConfigSpec(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, nil, err
	}

	err = vs.vmCreateIsReady(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, nil, err
	}

	// BMV: This is about where we used to do this check but it prb make more sense to do
	// earlier, as to limit wasted work. Before DoPlacement() is likely the best place so
	// the window between the placement decision and creating the VM on VC is small(ish).
	allowed, createDeferFn := vs.vmCreateConcurrentAllowed(vmCtx)
	if !allowed {
		return nil, nil, nil
	}
	defer createDeferFn()

	moRef, err := vmlifecycle.CreateVirtualMachine(
		vmCtx,
		vcClient.ContentLibClient(),
		vcClient.RestClient(),
		vcClient.Finder(),
		&createArgs.CreateArgs)
	if err != nil {
		vmCtx.Logger.Error(err, "CreateVirtualMachine failed")
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionCreated, "Error", err.Error())
		return nil, nil, err
	}

	vmCtx.VM.Status.UniqueID = moRef.Reference().Value
	vmCtx.VM.Status.Image = &common.LocalObjectRef{
		APIVersion: vmopv1.SchemeGroupVersion.String(),
		Kind:       createArgs.ImageObj.GetObjectKind().GroupVersionKind().Kind,
		Name:       createArgs.ImageObj.GetName(),
	}
	vmCtx.VM.Status.Class = &common.LocalObjectRef{
		APIVersion: vmopv1.SchemeGroupVersion.String(),
		Kind:       createArgs.VMClass.Kind,
		Name:       createArgs.VMClass.Name,
	}
	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionCreated)

	return object.NewVirtualMachine(vcClient.VimClient(), *moRef), createArgs, nil
}

func (vs *vSphereVMProvider) createdVirtualMachineFallthroughUpdate(
	vmCtx context.VirtualMachineContextA2,
	vcVM *object.VirtualMachine,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	// TODO: In the common case, we'll call directly into update right after create succeeds, and
	// can use the createArgs to avoid doing a bunch of lookup work again.

	return vs.updateVirtualMachine(vmCtx, vcVM, vcClient, createArgs)
}

func (vs *vSphereVMProvider) updateVirtualMachine(
	vmCtx context.VirtualMachineContextA2,
	vcVM *object.VirtualMachine,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	vmCtx.Logger.V(4).Info("Updating VirtualMachine")

	{
		// Hack - create just enough of the Session that's needed for update

		cluster, err := virtualmachine.GetVMClusterComputeResource(vmCtx, vcVM)
		if err != nil {
			return err
		}

		ses := &session.Session{
			K8sClient: vs.k8sClient,
			Client:    vcClient,
			Finder:    vcClient.Finder(),
			Cluster:   cluster,
		}

		getUpdateArgsFn := func() (*vmUpdateArgs, error) {
			// TODO: Use createArgs if we already got them
			_ = createArgs
			return vs.vmUpdateGetArgs(vmCtx)
		}

		err = ses.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgsFn)
		if err != nil {
			return err
		}
	}

	// Back up the VM at the end after a successful update.
	// Skip TKG VMs since they are backed up differently than VM Service VMs.
	if pkgconfig.FromContext(vmCtx).Features.AutoVADPBackupRestore &&
		!kubeutil.HasTKGLabels(vmCtx.VM.Labels) {
		vmCtx.Logger.V(4).Info("Backing up VM Service managed VM")

		diskUUIDToPVC, err := GetAttachedDiskUUIDToPVC(vmCtx, vs.k8sClient)
		if err != nil {
			vmCtx.Logger.Error(err, "failed to get disk uuid to PVC mapping for backup")
			return err
		}

		additionalResources, err := GetAdditionalResourcesForBackup(vmCtx, vs.k8sClient)
		if err != nil {
			vmCtx.Logger.Error(err, "failed to get additional resources for backup")
			return err
		}

		backupOpts := virtualmachine.BackupVirtualMachineOptions{
			VMCtx:               vmCtx,
			VcVM:                vcVM,
			DiskUUIDToPVC:       diskUUIDToPVC,
			AdditionalResources: additionalResources,
		}
		if err := virtualmachine.BackupVirtualMachine(backupOpts); err != nil {
			vmCtx.Logger.Error(err, "failed to backup VM")
			return err
		}
	}

	return nil
}

// vmCreateDoPlacement determines placement of the VM prior to creating the VM on VC.
func (vs *vSphereVMProvider) vmCreateDoPlacement(
	vmCtx context.VirtualMachineContextA2,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	placementConfigSpec := virtualmachine.CreateConfigSpecForPlacement(
		vmCtx,
		createArgs.ConfigSpec,
		createArgs.StorageClassesToIDs)

	result, err := placement.Placement(
		vmCtx,
		vs.k8sClient,
		vcClient.VimClient(),
		placementConfigSpec,
		createArgs.ChildResourcePoolName)
	if err != nil {
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionPlacementReady, "NotReady", err.Error())
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
			conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionPlacementReady, "NotReady", err.Error())
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
		// Note if the VM create fails for some reason, but this label gets updated on the k8s VM,
		// then this is the pre-assigned zone on later create attempts.
		vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] = result.ZoneName
	}

	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionPlacementReady)

	return nil
}

// vmCreateGetFolderAndRPMoIDs gets the MoIDs of the Folder and Resource Pool the VM will be created under.
func (vs *vSphereVMProvider) vmCreateGetFolderAndRPMoIDs(
	vmCtx context.VirtualMachineContextA2,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

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
		// Placement already selected the ResourcePool/Cluster, so we just need this namespace's Folder.
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

	// Now that we know the ResourcePool, use that to look up the CCR.
	clusterMoRef, err := vcenter.GetResourcePoolOwnerMoRef(vmCtx, vcClient.VimClient(), createArgs.ResourcePoolMoID)
	if err != nil {
		return err
	}
	createArgs.ClusterMoRef = clusterMoRef

	return nil
}

func (vs *vSphereVMProvider) vmCreateFixupConfigSpec(
	vmCtx context.VirtualMachineContextA2,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	fixedUp, err := network.ResolveBackingPostPlacement(
		vmCtx,
		vcClient.VimClient(),
		createArgs.ClusterMoRef,
		&createArgs.NetworkResults)
	if err != nil {
		return err
	}

	if fixedUp {
		// Now that the backing is resolved for this CCR, re-zip to update the ConfigSpec. What a mess.
		err = vs.vmCreateGenConfigSpecZipNetworkInterfaces(vmCtx, createArgs)
		if err != nil {
			return err
		}
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateIsReady(
	vmCtx context.VirtualMachineContextA2,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	if policy := createArgs.ResourcePolicy; policy != nil {
		// TODO: May want to do this as to filter the placement candidates.
		exists, err := vs.doClusterModulesExist(vmCtx, vcClient.ClusterModuleClient(), createArgs.ClusterMoRef, policy)
		if err != nil {
			return err
		} else if !exists {
			return fmt.Errorf("VirtualMachineSetResourcePolicy cluster module is not ready")
		}
	}

	if createArgs.HasInstanceStorage {
		if _, ok := vmCtx.VM.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey]; !ok {
			return fmt.Errorf("instance storage PVCs are not bound yet")
		}
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateConcurrentAllowed(vmCtx context.VirtualMachineContextA2) (bool, func()) {
	maxDeployThreads := pkgconfig.FromContext(vmCtx).GetMaxDeployThreadsOnProvider()

	createCountLock.Lock()
	if concurrentCreateCount >= maxDeployThreads {
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
	vmCtx context.VirtualMachineContextA2,
	vcClient *vcclient.Client) (*VMCreateArgs, error) {

	createArgs, err := vs.vmCreateGetPrereqs(vmCtx, vcClient)
	if err != nil {
		return nil, err
	}

	err = vs.vmCreateDoNetworking(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	err = vs.vmCreateGenConfigSpec(vmCtx, createArgs)
	if err != nil {
		return nil, err
	}

	err = vs.vmCreateValidateArgs(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	return createArgs, nil
}

// vmCreateGetPrereqs returns the VMCreateArgs populated with the k8s objects required to
// create the VM on VC.
func (vs *vSphereVMProvider) vmCreateGetPrereqs(
	vmCtx context.VirtualMachineContextA2,
	vcClient *vcclient.Client) (*VMCreateArgs, error) {

	createArgs := &VMCreateArgs{}
	var prereqErrs []error

	if err := vs.vmCreateGetVirtualMachineClass(vmCtx, createArgs); err != nil {
		prereqErrs = append(prereqErrs, err)
	}

	if err := vs.vmCreateGetVirtualMachineImage(vmCtx, createArgs); err != nil {
		prereqErrs = append(prereqErrs, err)
	}

	if err := vs.vmCreateGetSetResourcePolicy(vmCtx, createArgs); err != nil {
		prereqErrs = append(prereqErrs, err)
	}

	if err := vs.vmCreateGetBootstrap(vmCtx, createArgs); err != nil {
		prereqErrs = append(prereqErrs, err)
	}

	if err := vs.vmCreateGetStoragePrereqs(vmCtx, vcClient, createArgs); err != nil {
		prereqErrs = append(prereqErrs, err)
	}

	// This is about the point where historically we'd declare the prereqs ready or not. There
	// is still a lot of work to do - and things to fail - before the actual create, but there
	// is no point in continuing if the above checks aren't met since we are missing data
	// required to create the VM.
	if len(prereqErrs) > 0 {
		return nil, k8serrors.NewAggregate(prereqErrs)
	}

	// Note that once the VM is created, it is hard for us to later resolve what image was used,
	// since a NS or cluster scoped image could have been created or deleted.
	vmCtx.VM.Status.Image = &common.LocalObjectRef{
		APIVersion: createArgs.ImageObj.GetObjectKind().GroupVersionKind().Version,
		Kind:       createArgs.ImageObj.GetObjectKind().GroupVersionKind().Kind,
		Name:       createArgs.ImageObj.GetName(),
	}

	vmCtx.VM.Status.Class = &common.LocalObjectRef{
		APIVersion: createArgs.VMClass.APIVersion,
		Kind:       createArgs.VMClass.Kind,
		Name:       createArgs.VMClass.Name,
	}

	return createArgs, nil
}

func (vs *vSphereVMProvider) vmCreateGetVirtualMachineClass(
	vmCtx context.VirtualMachineContextA2,
	createArgs *VMCreateArgs) error {

	vmClass, err := GetVirtualMachineClass(vmCtx, vs.k8sClient)
	if err != nil {
		return err
	}

	createArgs.VMClass = vmClass

	return nil
}

func (vs *vSphereVMProvider) vmCreateGetVirtualMachineImage(
	vmCtx context.VirtualMachineContextA2,
	createArgs *VMCreateArgs) error {

	imageObj, imageSpec, imageStatus, err := GetVirtualMachineImageSpecAndStatus(vmCtx, vs.k8sClient)
	if err != nil {
		return err
	}

	createArgs.ImageObj = imageObj
	createArgs.ImageSpec = imageSpec
	createArgs.ImageStatus = imageStatus

	// This is clunky, but we need to know how to use the image to create the VM. Our only supported
	// method is via the ContentLibrary, so check if this image was derived from a CL item.
	switch imageSpec.ProviderRef.Kind {
	case "ClusterContentLibraryItem", "ContentLibraryItem":
		createArgs.UseContentLibrary = true
		createArgs.ProviderItemID = imageStatus.ProviderItemID
	default:
		if !SkipVMImageCLProviderCheck {
			err := fmt.Errorf("unsupported image provider kind: %s", imageSpec.ProviderRef.Kind)
			conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady, "NotSupported", err.Error())
			return err
		}
		// Testing only: we'll clone the source VM found in the Inventory.
		createArgs.UseContentLibrary = false
		createArgs.ProviderItemID = vmCtx.VM.Spec.ImageName
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateGetSetResourcePolicy(
	vmCtx context.VirtualMachineContextA2,
	createArgs *VMCreateArgs) error {

	resourcePolicy, err := GetVMSetResourcePolicy(vmCtx, vs.k8sClient)
	if err != nil {
		return err
	}

	// The SetResourcePolicy is optional (TKG VMs will always have it).
	if resourcePolicy != nil {
		createArgs.ResourcePolicy = resourcePolicy
		createArgs.ChildFolderName = resourcePolicy.Spec.Folder
		createArgs.ChildResourcePoolName = resourcePolicy.Spec.ResourcePool.Name
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateGetBootstrap(
	vmCtx context.VirtualMachineContextA2,
	createArgs *VMCreateArgs) error {

	bsData, err := GetVirtualMachineBootstrap(vmCtx, vs.k8sClient)
	if err != nil {
		return err
	}

	createArgs.BootstrapData.Data = bsData.Data
	createArgs.BootstrapData.VAppData = bsData.VAppData
	createArgs.BootstrapData.VAppExData = bsData.VAppExData
	createArgs.BootstrapData.CloudConfig = bsData.CloudConfig

	return nil
}

func (vs *vSphereVMProvider) vmCreateGetStoragePrereqs(
	vmCtx context.VirtualMachineContextA2,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	if pkgconfig.FromContext(vmCtx).Features.InstanceStorage {
		// To determine all the storage profiles, we need the class because of the possibility of
		// InstanceStorage volumes. If we weren't able to get the class earlier, still check & set
		// the storage condition because instance storage usage is rare, it is helpful to report
		// as many prereqs as possible, and we'll reevaluate this once the class is available.
		if createArgs.VMClass != nil {
			// Add the class's instance storage disks - if any - to the VM.Spec. Once the instance
			// storage disks are added to the VM, they are set in stone even if the class itself or
			// the VM's assigned class changes.
			createArgs.HasInstanceStorage = AddInstanceStorageVolumes(vmCtx, createArgs.VMClass)
		}
	}

	storageClassesToIDs, err := storage.GetVMStoragePoliciesIDs(vmCtx, vs.k8sClient)
	if err != nil {
		reason, msg := errToConditionReasonAndMessage(err)
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionStorageReady, reason, msg)
		return err
	}

	vmStorageProfileID := storageClassesToIDs[vmCtx.VM.Spec.StorageClass]

	provisioningType, err := virtualmachine.GetDefaultDiskProvisioningType(vmCtx, vcClient, vmStorageProfileID)
	if err != nil {
		reason, msg := errToConditionReasonAndMessage(err)
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionStorageReady, reason, msg)
		return err
	}

	createArgs.StorageClassesToIDs = storageClassesToIDs
	createArgs.StorageProvisioning = provisioningType
	createArgs.StorageProfileID = vmStorageProfileID
	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionStorageReady)

	return nil
}

func (vs *vSphereVMProvider) vmCreateDoNetworking(
	vmCtx context.VirtualMachineContextA2,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	networkSpec := vmCtx.VM.Spec.Network
	if networkSpec == nil || networkSpec.Disabled {
		return nil
	}

	results, err := network.CreateAndWaitForNetworkInterfaces(
		vmCtx,
		vs.k8sClient,
		vcClient.VimClient(),
		vcClient.Finder(),
		nil, // Don't know the CCR yet (needed to resolve backings for NSX-T)
		networkSpec.Interfaces)
	if err != nil {
		conditions.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionNetworkReady, "NotReady", err.Error())
		return err
	}

	createArgs.NetworkResults = results
	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionNetworkReady)

	return nil
}

func (vs *vSphereVMProvider) vmCreateGenConfigSpec(
	vmCtx context.VirtualMachineContextA2,
	createArgs *VMCreateArgs) error {

	// TODO: This is a partial dupe of what's done in the update path in the remaining Session code. I got
	// tired of trying to keep that in sync so we get to live with a frankenstein thing longer.

	var vmClassConfigSpec *types.VirtualMachineConfigSpec
	if rawConfigSpec := createArgs.VMClass.Spec.ConfigSpec; pkgconfig.FromContext(vmCtx).Features.VMClassAsConfigDayNDate && len(rawConfigSpec) > 0 {
		configSpec, err := GetVMClassConfigSpec(vmCtx, rawConfigSpec)
		if err != nil {
			return err
		}
		vmClassConfigSpec = configSpec
	} else {
		vmClassConfigSpec = virtualmachine.ConfigSpecFromVMClassDevices(&createArgs.VMClass.Spec)
	}

	var minCPUFreq uint64
	if res := createArgs.VMClass.Spec.Policies.Resources; !res.Requests.Cpu.IsZero() || !res.Limits.Cpu.IsZero() {
		freq, err := vs.getOrComputeCPUMinFrequency(vmCtx)
		if err != nil {
			return err
		}
		minCPUFreq = freq
	}

	createArgs.ConfigSpec = virtualmachine.CreateConfigSpec(
		vmCtx,
		vmClassConfigSpec,
		&createArgs.VMClass.Spec,
		createArgs.ImageStatus,
		minCPUFreq)

	// TODO: This should be in CreateConfigSpec()
	if createArgs.ConfigSpec.Version == "" {
		imageVer := int32(0)
		if createArgs.ImageStatus.HardwareVersion != nil {
			imageVer = *createArgs.ImageStatus.HardwareVersion
		}

		version := HardwareVersionForPVCandPCIDevices(imageVer, createArgs.ConfigSpec, HasPVC(vmCtx.VM.Spec))
		if version != 0 {
			createArgs.ConfigSpec.Version = fmt.Sprintf("vmx-%d", version)
		}
	}

	util.EnsureMinHardwareVersionInConfigSpec(createArgs.ConfigSpec, vmCtx.VM.Spec.MinHardwareVersion)

	err := vs.vmCreateGenConfigSpecExtraConfig(vmCtx, createArgs)
	if err != nil {
		return err
	}

	err = vs.vmCreateGenConfigSpecChangeBootDiskSize(vmCtx, createArgs)
	if err != nil {
		return err
	}

	err = vs.vmCreateGenConfigSpecZipNetworkInterfaces(vmCtx, createArgs)
	if err != nil {
		return err
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateGenConfigSpecExtraConfig(
	vmCtx context.VirtualMachineContextA2,
	createArgs *VMCreateArgs) error {

	ecMap := make(map[string]string, len(vs.globalExtraConfig))

	// The only use of this template is for the JSON_EXTRA_CONFIG that is set in gce2e env
	// to populate {{.ImageName }} so vcsim will create a container for the VM.
	// BMV: This should be removable now that vcsim gce2e is gone.
	renderTemplateFn := func(name, text string) string {
		t, err := template.New(name).Parse(text)
		if err != nil {
			return text
		}
		b := strings.Builder{}
		if err := t.Execute(&b, createArgs.ImageStatus); err != nil {
			return text
		}
		return b.String()
	}
	for k, v := range vs.globalExtraConfig {
		ecMap[k] = renderTemplateFn(k, v)
	}

	hasPassthroughDevices := len(util.SelectVirtualPCIPassthrough(util.DevicesFromConfigSpec(createArgs.ConfigSpec))) > 0

	if hasPassthroughDevices {
		mmioSize := vmCtx.VM.Annotations[constants.PCIPassthruMMIOOverrideAnnotation]
		if mmioSize == "" {
			mmioSize = constants.PCIPassthruMMIOSizeDefault
		}
		if mmioSize != "0" {
			ecMap[constants.PCIPassthruMMIOExtraConfigKey] = constants.ExtraConfigTrue
			ecMap[constants.PCIPassthruMMIOSizeExtraConfigKey] = mmioSize
		}
	}

	// The ConfigSpec's current ExtraConfig values (that came from the class) take precedence over what was set here.
	createArgs.ConfigSpec.ExtraConfig = util.AppendNewExtraConfigValues(createArgs.ConfigSpec.ExtraConfig, ecMap)

	// Leave constants.VMOperatorV1Alpha1ExtraConfigKey for the update path (if that's still even needed)

	return nil
}

func (vs *vSphereVMProvider) vmCreateGenConfigSpecChangeBootDiskSize(
	vmCtx context.VirtualMachineContextA2,
	_ *VMCreateArgs) error {

	advanced := vmCtx.VM.Spec.Advanced
	if advanced == nil || advanced.BootDiskCapacity == nil || advanced.BootDiskCapacity.IsZero() {
		return nil
	}

	// TODO: How to we determine the DeviceKey for the DeviceChange entry? We probably have to
	// crack the image/source, which is hard to do ATM. Punt on this for a placement consideration
	// and we'll resize the boot (first) disk after VM create like before.

	return nil
}

func (vs *vSphereVMProvider) vmCreateGenConfigSpecZipNetworkInterfaces(
	vmCtx context.VirtualMachineContextA2,
	createArgs *VMCreateArgs) error {

	if vmCtx.VM.Spec.Network == nil || vmCtx.VM.Spec.Network.Disabled {
		util.RemoveDevicesFromConfigSpec(createArgs.ConfigSpec, util.IsEthernetCard)
		return nil
	}

	resultsIdx := 0
	var unmatchedEthDevices []int

	for idx := range createArgs.ConfigSpec.DeviceChange {
		spec := createArgs.ConfigSpec.DeviceChange[idx].GetVirtualDeviceConfigSpec()
		if spec == nil || !util.IsEthernetCard(spec.Device) {
			continue
		}

		device := spec.Device
		ethCard := device.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()

		if resultsIdx < len(createArgs.NetworkResults.Results) {
			err := network.ApplyInterfaceResultToVirtualEthCard(vmCtx, ethCard, &createArgs.NetworkResults.Results[resultsIdx])
			if err != nil {
				return err
			}
			resultsIdx++

		} else {
			// This ConfigSpec Ethernet device does not have a corresponding entry in the VM Spec, so we
			// won't ever have a backing for it. Remove it from the ConfigSpec since that is the easiest
			// thing to do, since extra NICs can cause later complications around GOSC and other customizations.
			// The downside with this is that if a NIC is added to the VM Spec, it won't necessarily have this
			// config but the default. Revisit this later if we don't like that behavior.
			unmatchedEthDevices = append(unmatchedEthDevices, idx-len(unmatchedEthDevices))
		}
	}

	if len(unmatchedEthDevices) > 0 {
		deviceChange := createArgs.ConfigSpec.DeviceChange
		for _, idx := range unmatchedEthDevices {
			deviceChange = append(deviceChange[:idx], deviceChange[idx+1:]...)
		}
		createArgs.ConfigSpec.DeviceChange = deviceChange
	}

	// Any remaining VM Spec network interfaces were not matched with a device in the ConfigSpec, so
	// create a default virtual ethernet card for them.
	for i := resultsIdx; i < len(createArgs.NetworkResults.Results); i++ {
		ethCardDev, err := network.CreateDefaultEthCard(vmCtx, &createArgs.NetworkResults.Results[i])
		if err != nil {
			return err
		}

		// May not have the backing yet (NSX-T). We come back through here after placement once we
		// know the backing.
		if ethCardDev != nil {
			createArgs.ConfigSpec.DeviceChange = append(createArgs.ConfigSpec.DeviceChange, &types.VirtualDeviceConfigSpec{
				Operation: types.VirtualDeviceConfigSpecOperationAdd,
				Device:    ethCardDev,
			})
		}
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateValidateArgs(
	vmCtx context.VirtualMachineContextA2,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

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

func (vs *vSphereVMProvider) vmUpdateGetArgs(
	vmCtx context.VirtualMachineContextA2) (*vmUpdateArgs, error) {

	vmClass, err := GetVirtualMachineClass(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	resourcePolicy, err := GetVMSetResourcePolicy(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	bsData, err := GetVirtualMachineBootstrap(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	updateArgs := &vmUpdateArgs{}
	updateArgs.VMClass = vmClass
	updateArgs.ResourcePolicy = resourcePolicy
	updateArgs.BootstrapData.Data = bsData.Data
	updateArgs.BootstrapData.VAppData = bsData.VAppData
	updateArgs.BootstrapData.VAppExData = bsData.VAppExData
	updateArgs.BootstrapData.CloudConfig = bsData.CloudConfig

	if res := vmClass.Spec.Policies.Resources; !res.Requests.Cpu.IsZero() || !res.Limits.Cpu.IsZero() {
		freq, err := vs.getOrComputeCPUMinFrequency(vmCtx)
		if err != nil {
			return nil, err
		}
		updateArgs.MinCPUFreq = freq
	}

	var vmClassConfigSpec *types.VirtualMachineConfigSpec
	if pkgconfig.FromContext(vmCtx).Features.VMClassAsConfigDayNDate {
		if cs := updateArgs.VMClass.Spec.ConfigSpec; cs != nil {
			var err error
			vmClassConfigSpec, err = GetVMClassConfigSpec(vmCtx, cs)
			if err != nil {
				return nil, err
			}
		}
	}

	var vmImageStatus *vmopv1.VirtualMachineImageStatus
	// Only get VM image when this is the VM first boot.
	if isVMFirstBoot(vmCtx) {
		var err error
		_, _, vmImageStatus, err = GetVirtualMachineImageSpecAndStatus(vmCtx, vs.k8sClient)
		if err != nil {
			return nil, err
		}

		// The only use of this is for the global JSON_EXTRA_CONFIG to set the image name.
		// The global extra config should only be set during first boot.
		// TODO: We can just finally kill this with the demise of old gce2e?
		renderTemplateFn := func(name, text string) string {
			t, err := template.New(name).Parse(text)
			if err != nil {
				return text
			}
			b := strings.Builder{}
			if err := t.Execute(&b, vmImageStatus); err != nil {
				return text
			}
			return b.String()
		}
		extraConfig := make(map[string]string, len(vs.globalExtraConfig))
		for k, v := range vs.globalExtraConfig {
			extraConfig[k] = renderTemplateFn(k, v)
		}
		updateArgs.ExtraConfig = extraConfig

		// Enabling the defer-cloud-init extraConfig key for V1Alpha1Compatible images defers cloud-init from running on first boot
		// and disables networking configurations by cloud-init. Therefore, only set the extraConfig key to enabled
		// when the vmMetadata is nil or when the transport requested is not CloudInit.
		// TODO: Is this still actually needed?
		updateArgs.VirtualMachineImageV1Alpha1Compatible =
			conditions.IsTrueFromConditions(vmImageStatus.Conditions, "VirtualMachineImageV1Alpha1Compatible" /*vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition*/)
	}

	updateArgs.ConfigSpec = virtualmachine.CreateConfigSpec(
		vmCtx,
		vmClassConfigSpec,
		&updateArgs.VMClass.Spec,
		vmImageStatus,
		updateArgs.MinCPUFreq)

	return updateArgs, nil
}

func isVMFirstBoot(vmCtx context.VirtualMachineContextA2) bool {
	if _, ok := vmCtx.VM.Annotations[vmopv1.FirstBootDoneAnnotation]; ok {
		return false
	}

	return true
}
