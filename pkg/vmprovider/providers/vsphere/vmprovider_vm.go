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
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/storage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
)

// vmCreateArgs contains the arguments needed to create a VM on VC.
type vmCreateArgs struct {
	VMClass             *vmopv1alpha1.VirtualMachineClass
	VMImage             *vmopv1alpha1.VirtualMachineImage
	ResourcePolicy      *vmopv1alpha1.VirtualMachineSetResourcePolicy
	VMMetadata          vmprovider.VMMetadata
	ContentLibraryUUID  string
	StorageClassesToIDs map[string]string
	StorageProvisioning string

	ConfigSpec          *types.VirtualMachineConfigSpec
	PlacementConfigSpec *types.VirtualMachineConfigSpec

	// TODO: FolderMoID, ResourcePoolMoID
}

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

	err = vs.vmCreateIsReady(vmCtx, createArgs)
	if err != nil {
		return nil, err
	}

	allowed, createDeferFn := vs.vmCreateConcurrentAllowed(vmCtx)
	if !allowed {
		return nil, nil
	}
	defer createDeferFn()

	var vcVM *object.VirtualMachine
	{
		// Hack
		vmCtx.Logger.Info("Creating VirtualMachine")

		ses, err := vs.sessions.GetSessionForVM(vmCtx)
		if err != nil {
			return nil, err
		}

		vmConfigArgs := vmprovider.VMConfigArgs{
			VMClass:            *createArgs.VMClass,
			VMImage:            createArgs.VMImage,
			ResourcePolicy:     createArgs.ResourcePolicy,
			VMMetadata:         createArgs.VMMetadata,
			StorageProfileID:   createArgs.StorageClassesToIDs[vmCtx.VM.Spec.StorageClass],
			ContentLibraryUUID: createArgs.ContentLibraryUUID,
		}

		resVM, err := ses.CloneVirtualMachine(vmCtx, vmConfigArgs)
		if err != nil {
			vmCtx.Logger.Error(err, "Clone VirtualMachine failed")
			return nil, err
		}

		// Set a few Status fields that we easily have on hand here. The controller will immediately call
		// UpdateVirtualMachine() which will set it all.
		vmCtx.VM.Status.Phase = vmopv1alpha1.Created
		vmCtx.VM.Status.UniqueID = resVM.MoRef().Value

		vcVM = resVM.VcVM()
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

		vmConfigArgs := vmprovider.VMConfigArgs{
			VMClass:            *createArgs.VMClass,
			VMImage:            createArgs.VMImage,
			ResourcePolicy:     createArgs.ResourcePolicy,
			VMMetadata:         createArgs.VMMetadata,
			StorageProfileID:   createArgs.StorageClassesToIDs[vmCtx.VM.Spec.StorageClass],
			ContentLibraryUUID: createArgs.ContentLibraryUUID,
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

	childRPName := ""
	if createArgs.ResourcePolicy != nil {
		childRPName = createArgs.ResourcePolicy.Spec.ResourcePool.Name
	}

	return placement.Placement(vmCtx, vs.k8sClient, vcClient.VimClient(), createArgs.PlacementConfigSpec, childRPName)
}

func (vs *vSphereVMProvider) vmCreateIsReady(
	vmCtx context.VirtualMachineContext,
	vmCreateArgs *vmCreateArgs) error {

	if rp := vmCreateArgs.ResourcePolicy; rp != nil {
		if !rp.DeletionTimestamp.IsZero() {
			return fmt.Errorf("cannot create VirtualMachine when its resource policy is being deleted")
		}

		// TODO: Parts of this check should be done as to filter the placement candidates.
		ready, err := vs.IsVirtualMachineSetResourcePolicyReady(vmCtx,
			vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey], rp)
		if err != nil {
			return err
		}

		if !ready {
			return fmt.Errorf("VirtualMachineSetResourcePolicy is not yet ready")
		}
	}

	if lib.IsInstanceStorageFSSEnabled() {
		if instancestorage.IsConfigured(vmCtx.VM) {
			if _, ok := vmCtx.VM.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey]; !ok {
				return fmt.Errorf("instance storage PVCs are not bound yet")
			}
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
	}

	err = vs.vmCreateGetStoragePrereqs(vmCtx, vcClient, createArgs)
	if err != nil {
		return nil, err
	}

	err = vs.vmCreateGenConfigSpec(vmCtx, createArgs)
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
	createArgs.ResourcePolicy = resourcePolicy
	createArgs.VMMetadata = vmMetadata

	// TODO: Perhaps a condition type for each resource is better so all missing one(s)
	// 	     can be reported at once (and will help for the best-effort update changes).
	conditions.MarkTrue(vmCtx.VM, vmopv1alpha1.VirtualMachinePrereqReadyCondition)

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

	provisioningType, err := virtualmachine.GetDefaultDiskProvisioningType(vmCtx, vcClient,
		storageClassesToIDs[vmCtx.VM.Spec.StorageClass])
	if err != nil {
		return err
	}

	createArgs.StorageClassesToIDs = storageClassesToIDs
	createArgs.StorageProvisioning = provisioningType

	return nil
}

func (vs *vSphereVMProvider) vmCreateGenConfigSpec(
	vmCtx context.VirtualMachineContext,
	createArgs *vmCreateArgs) error {

	var minCPUFreq uint64

	if createArgs.ResourcePolicy != nil {
		rp := createArgs.ResourcePolicy.Spec.ResourcePool

		if !rp.Reservations.Cpu.IsZero() || !rp.Limits.Cpu.IsZero() {
			freq, err := vs.ComputeAndGetCPUMinFrequency(vmCtx)
			if err != nil {
				return err
			}
			minCPUFreq = freq
		}
	}

	// Create both until we can sanitize the placement ConfigSpec for creation.

	createArgs.ConfigSpec = virtualmachine.CreateConfigSpec(
		vmCtx.VM.Name,
		&createArgs.VMClass.Spec,
		minCPUFreq)

	createArgs.PlacementConfigSpec = virtualmachine.CreateConfigSpecForPlacement(
		vmCtx,
		&createArgs.VMClass.Spec,
		minCPUFreq,
		createArgs.StorageClassesToIDs)

	return nil
}
