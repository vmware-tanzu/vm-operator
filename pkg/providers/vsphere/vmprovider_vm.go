// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"path"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/pbm"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/yaml"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	pkgcnd "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/clustermodules"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/storage"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

// VMCreateArgs contains the arguments needed to create a VM on VC.
type VMCreateArgs struct {
	vmlifecycle.CreateArgs
	vmlifecycle.BootstrapData

	VMClass        vmopv1.VirtualMachineClass
	ResourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
	ImageObj       ctrlclient.Object
	ImageSpec      vmopv1.VirtualMachineImageSpec
	ImageStatus    vmopv1.VirtualMachineImageStatus

	Storage               storage.VMStorageData
	HasInstanceStorage    bool
	ChildResourcePoolName string
	ChildFolderName       string
	ClusterMoRef          vimtypes.ManagedObjectReference

	NetworkResults network.NetworkInterfaceResults
}

// TODO: Until we sort out what the Session becomes.
type vmUpdateArgs = session.VMUpdateArgs
type vmResizeArgs = session.VMResizeArgs

var (
	createCountLock       sync.Mutex
	concurrentCreateCount int

	// currentlyReconciling tracks the VMs currently being created in a
	// non-blocking goroutine.
	currentlyReconciling sync.Map

	// SkipVMImageCLProviderCheck skips the checks that a VM Image has a Content Library item provider
	// since a VirtualMachineImage created for a VM template won't have either. This has been broken for
	// a long time but was otherwise masked on how the tests used to be organized.
	SkipVMImageCLProviderCheck = false
)

func (vs *vSphereVMProvider) CreateOrUpdateVirtualMachine(
	ctx context.Context,
	vm *vmopv1.VirtualMachine) error {

	_, err := vs.createOrUpdateVirtualMachine(ctx, vm, false)
	return err
}

func (vs *vSphereVMProvider) CreateOrUpdateVirtualMachineAsync(
	ctx context.Context,
	vm *vmopv1.VirtualMachine) (<-chan error, error) {

	return vs.createOrUpdateVirtualMachine(ctx, vm, true)
}

func (vs *vSphereVMProvider) createOrUpdateVirtualMachine(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	async bool) (chan error, error) {

	vmNamespacedName := vm.NamespacedName()

	if _, ok := currentlyReconciling.Load(vmNamespacedName); ok {
		// Do not process the VM again if it is already being reconciled in a
		// goroutine.
		return nil, providers.ErrReconcileInProgress
	}

	vmCtx := pkgctx.VirtualMachineContext{
		Context: context.WithValue(
			ctx,
			vimtypes.ID{},
			vs.getOpID(vm, "createOrUpdateVM"),
		),
		Logger: log.WithValues("vmName", vm.NamespacedName()),
		VM:     vm,
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return nil, err
	}

	// Set the VC UUID annotation on the VM before attempting creation or
	// update. Among other things, the annotation facilitates differential
	// handling of restore and fail-over operations.
	if vm.Annotations == nil {
		vm.Annotations = make(map[string]string)
	}
	vCenterInstanceUUID := client.VimClient().ServiceContent.About.InstanceUuid
	vm.Annotations[vmopv1.ManagerID] = vCenterInstanceUUID

	// Check to see if the VM can be found on the underlying platform.
	foundVM, err := vs.getVM(vmCtx, client, false)
	if err != nil {
		return nil, err
	}

	if foundVM != nil {
		// Mark that this is an update operation.
		ctxop.MarkUpdate(vmCtx)

		return nil, vs.updateVirtualMachine(vmCtx, foundVM, client, nil)
	}

	// Mark that this is a create operation.
	ctxop.MarkCreate(vmCtx)

	// Do not allow more than N create threads/goroutines.
	//
	// - In blocking create mode, this ensures there are reconciler threads
	//   available to reconcile non-create operations.
	//
	// - In non-blocking create mode, this ensures the number of goroutines
	//   spawned to create VMs does not take up too much memory.
	allowed, decrementConcurrentCreatesFn := vs.vmCreateConcurrentAllowed(vmCtx)
	if !allowed {
		return nil, providers.ErrTooManyCreates
	}

	// cleanupFn tracks the function(s) that must be invoked upon leaving this
	// function during a blocking create or after an async create.
	cleanupFn := decrementConcurrentCreatesFn

	if !async {
		defer cleanupFn()

		vmCtx.Logger.V(4).Info("Doing a blocking create")

		createArgs, err := vs.getCreateArgs(vmCtx, client)
		if err != nil {
			return nil, err
		}

		newVM, err := vs.createVirtualMachine(vmCtx, client, createArgs)
		if err != nil {
			return nil, err
		}

		// If the create actually occurred, fall-through to an update
		// post-reconfigure.
		return nil, vs.createdVirtualMachineFallthroughUpdate(
			vmCtx,
			newVM,
			client,
			createArgs)
	}

	if _, ok := currentlyReconciling.LoadOrStore(vmNamespacedName, struct{}{}); ok {
		// If the VM is already being created in a goroutine, then there is no
		// need to create it again.
		//
		// However, we need to make sure we decrement the number of concurrent
		// creates before returning.
		cleanupFn()
		return nil, providers.ErrReconcileInProgress
	}

	vmCtx.Logger.V(4).Info("Doing a non-blocking create")

	// Update the cleanup function to include indicating a concurrent create is
	// no longer occurring.
	cleanupFn = func() {
		currentlyReconciling.Delete(vmNamespacedName)
		decrementConcurrentCreatesFn()
	}

	createArgs, err := vs.getCreateArgs(vmCtx, client)
	if err != nil {
		// If getting the create args failed, the concurrent counter needs to be
		// decremented before returning.
		cleanupFn()
		return nil, err
	}

	// Create a copy of the context and replace its VM with a copy to
	// ensure modifications in the goroutine below are not impacted or
	// impact the operations above us in the call stack.
	copyOfCtx := vmCtx
	copyOfCtx.VM = vmCtx.VM.DeepCopy()

	// Start a goroutine to create the VM in the background.
	chanErr := make(chan error)
	go vs.createVirtualMachineAsync(
		copyOfCtx,
		client,
		createArgs,
		chanErr,
		cleanupFn)

	// Return with the error channel. The VM will be re-enqueued once the create
	// completes with success or failure.
	return chanErr, nil
}

func (vs *vSphereVMProvider) DeleteVirtualMachine(
	ctx context.Context,
	vm *vmopv1.VirtualMachine) error {

	vmNamespacedName := vm.NamespacedName()

	if _, ok := currentlyReconciling.Load(vmNamespacedName); ok {
		// If the VM is already being reconciled in a goroutine then it cannot
		// be deleted yet.
		return providers.ErrReconcileInProgress
	}

	vmCtx := pkgctx.VirtualMachineContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpID(vm, "deleteVM")),
		Logger:  log.WithValues("vmName", vmNamespacedName),
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
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	vmPub *vmopv1.VirtualMachinePublishRequest,
	cl *imgregv1a1.ContentLibrary,
	actID string) (string, error) {

	vmCtx := pkgctx.VirtualMachineContext{
		Context: ctx,
		// Update logger info
		Logger: log.WithValues("vmName", vm.NamespacedName()).
			WithValues("clName", fmt.Sprintf("%s/%s", cl.Namespace, cl.Name)).
			WithValues("vmPubName", fmt.Sprintf("%s/%s", vmPub.Namespace, vmPub.Name)),
		VM: vm,
	}

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get vCenter client: %w", err)
	}

	itemID, err := virtualmachine.CreateOVF(vmCtx, client.RestClient(), vmPub, cl, actID)
	if err != nil {
		return "", err
	}

	return itemID, nil
}

func (vs *vSphereVMProvider) GetVirtualMachineGuestHeartbeat(
	ctx context.Context,
	vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error) {

	vmCtx := pkgctx.VirtualMachineContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpID(vm, "heartbeat")),
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

func (vs *vSphereVMProvider) GetVirtualMachineProperties(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	propertyPaths []string) (map[string]any, error) {

	vmCtx := pkgctx.VirtualMachineContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpID(vm, "properties")),
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

	propSet := []vimtypes.PropertySpec{{Type: "VirtualMachine"}}
	if len(propertyPaths) == 0 {
		propSet[0].All = vimtypes.NewBool(true)
	} else {
		propSet[0].PathSet = propertyPaths
	}

	rep, err := property.DefaultCollector(client.VimClient()).RetrieveProperties(
		ctx,
		vimtypes.RetrieveProperties{
			SpecSet: []vimtypes.PropertyFilterSpec{
				{
					ObjectSet: []vimtypes.ObjectSpec{
						{
							Obj: vcVM.Reference(),
						},
					},
					PropSet: propSet,
				},
			},
		})

	if err != nil {
		return nil, err
	}

	if len(rep.Returnval) == 0 {
		return nil, fmt.Errorf("no properties")
	}

	result := map[string]any{}
	for i := range rep.Returnval[0].PropSet {
		dp := rep.Returnval[0].PropSet[i]
		result[dp.Name] = dp.Val
	}

	return result, nil
}

func (vs *vSphereVMProvider) GetVirtualMachineWebMKSTicket(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	pubKey string) (string, error) {

	vmCtx := pkgctx.VirtualMachineContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpID(vm, "webconsole")),
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
	ctx context.Context,
	vm *vmopv1.VirtualMachine) (vimtypes.HardwareVersion, error) {

	vmCtx := pkgctx.VirtualMachineContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpID(vm, "hardware-version")),
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

	return vimtypes.ParseHardwareVersion(o.Config.Version)
}

func (vs *vSphereVMProvider) vmCreatePathName(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	if len(vmCtx.VM.Spec.Cdrom) == 0 {
		return nil // only needed when deploying ISO library items
	}

	if createArgs.StorageProfileID == "" {
		return nil
	}
	if createArgs.ConfigSpec.Files == nil {
		createArgs.ConfigSpec.Files = &vimtypes.VirtualMachineFileInfo{}
	}
	if createArgs.ConfigSpec.Files.VmPathName != "" {
		return nil
	}

	vc := vcClient.VimClient()

	pc, err := pbm.NewClient(vmCtx, vc)
	if err != nil {
		return err
	}

	ds, err := pc.DatastoreMap(vmCtx, vc, createArgs.ClusterMoRef)
	if err != nil {
		return err
	}

	req := []pbmtypes.BasePbmPlacementRequirement{
		&pbmtypes.PbmPlacementCapabilityProfileRequirement{
			ProfileId: pbmtypes.PbmProfileId{UniqueId: createArgs.StorageProfileID},
		},
	}

	res, err := pc.CheckRequirements(vmCtx, ds.PlacementHub, nil, req)
	if err != nil {
		return err
	}

	hubs := res.CompatibleDatastores()
	if len(hubs) == 0 {
		return nil
	}

	hub := hubs[rand.Intn(len(hubs))] //nolint:gosec

	createArgs.ConfigSpec.Files.VmPathName = (&object.DatastorePath{
		Datastore: ds.Name[hub.HubId],
	}).String()

	vmCtx.Logger.Info("vmCreatePathName", "VmPathName", createArgs.ConfigSpec.Files.VmPathName)

	return nil
}

func (vs *vSphereVMProvider) vmCreatePathNameFromDatastoreRecommendation(
	vmCtx pkgctx.VirtualMachineContext,
	createArgs *VMCreateArgs) error {

	if createArgs.ConfigSpec.Files == nil {
		createArgs.ConfigSpec.Files = &vimtypes.VirtualMachineFileInfo{}
	}
	if createArgs.ConfigSpec.Files.VmPathName != "" {
		return nil
	}
	if len(createArgs.Datastores) == 0 {
		return errors.New("no compatible datastores")
	}

	createArgs.ConfigSpec.Files.VmPathName = fmt.Sprintf(
		"[%s] %s/%s.vmx",
		createArgs.Datastores[0].Name,
		vmCtx.VM.UID,
		vmCtx.VM.Name)

	vmCtx.Logger.Info(
		"vmCreatePathName",
		"VmPathName", createArgs.ConfigSpec.Files.VmPathName)

	return nil
}

func (vs *vSphereVMProvider) getCreateArgs(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client) (*VMCreateArgs, error) {

	createArgs, err := vs.vmCreateGetArgs(vmCtx, vcClient)
	if err != nil {
		return nil, err
	}

	if err := vs.vmCreateDoPlacement(vmCtx, vcClient, createArgs); err != nil {
		return nil, err
	}

	if err := vs.vmCreateGetFolderAndRPMoIDs(vmCtx, vcClient, createArgs); err != nil {
		return nil, err
	}

	if pkgcfg.FromContext(vmCtx).Features.FastDeploy {
		if err := vs.vmCreateGetSourceDiskPaths(vmCtx, vcClient, createArgs); err != nil {
			return nil, err
		}
		if err := vs.vmCreatePathNameFromDatastoreRecommendation(vmCtx, createArgs); err != nil {
			return nil, err
		}
	} else {
		if err := vs.vmCreatePathName(vmCtx, vcClient, createArgs); err != nil {
			return nil, err
		}
	}

	if err := vs.vmCreateIsReady(vmCtx, vcClient, createArgs); err != nil {
		return nil, err
	}

	return createArgs, nil
}

func (vs *vSphereVMProvider) createVirtualMachine(
	ctx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client,
	args *VMCreateArgs) (*object.VirtualMachine, error) {

	moRef, err := vmlifecycle.CreateVirtualMachine(
		ctx,
		vs.k8sClient,
		vcClient.RestClient(),
		vcClient.VimClient(),
		vcClient.Finder(),
		&args.CreateArgs)

	if err != nil {
		ctx.Logger.Error(err, "CreateVirtualMachine failed")
		pkgcnd.MarkError(
			ctx.VM,
			vmopv1.VirtualMachineConditionCreated,
			"Error",
			err)

		if pkgcfg.FromContext(ctx).Features.FastDeploy {
			pkgcnd.MarkError(
				ctx.VM,
				vmopv1.VirtualMachineConditionPlacementReady,
				"Error",
				err)
		}

		return nil, err
	}

	ctx.VM.Status.UniqueID = moRef.Reference().Value
	pkgcnd.MarkTrue(ctx.VM, vmopv1.VirtualMachineConditionCreated)

	if pkgcfg.FromContext(ctx).Features.FastDeploy {
		if zoneName := args.ZoneName; zoneName != "" {
			if ctx.VM.Labels == nil {
				ctx.VM.Labels = map[string]string{}
			}
			ctx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
		}
	}

	return object.NewVirtualMachine(vcClient.VimClient(), *moRef), nil
}

func (vs *vSphereVMProvider) createVirtualMachineAsync(
	ctx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client,
	args *VMCreateArgs,
	chanErr chan error,
	cleanupFn func()) {

	defer func() {
		close(chanErr)
		cleanupFn()
	}()

	moRef, vimErr := vmlifecycle.CreateVirtualMachine(
		ctx,
		vs.k8sClient,
		vcClient.RestClient(),
		vcClient.VimClient(),
		vcClient.Finder(),
		&args.CreateArgs)

	if vimErr != nil {
		ctx.Logger.Error(vimErr, "CreateVirtualMachine failed")
		chanErr <- vimErr
	}

	_, k8sErr := controllerutil.CreateOrPatch(
		ctx,
		vs.k8sClient,
		ctx.VM,
		func() error {

			if vimErr != nil {
				pkgcnd.MarkError(
					ctx.VM,
					vmopv1.VirtualMachineConditionCreated,
					"Error",
					vimErr)

				if pkgcfg.FromContext(ctx).Features.FastDeploy {
					pkgcnd.MarkError(
						ctx.VM,
						vmopv1.VirtualMachineConditionPlacementReady,
						"Error",
						vimErr)
				}

				return nil
			}

			if pkgcfg.FromContext(ctx).Features.FastDeploy {
				if zoneName := args.ZoneName; zoneName != "" {
					if ctx.VM.Labels == nil {
						ctx.VM.Labels = map[string]string{}
					}
					ctx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
				}
			}

			ctx.VM.Status.UniqueID = moRef.Reference().Value
			pkgcnd.MarkTrue(ctx.VM, vmopv1.VirtualMachineConditionCreated)

			return nil
		},
	)

	if k8sErr != nil {
		ctx.Logger.Error(k8sErr, "Failed to patch VM status after create")
		chanErr <- k8sErr
	}
}

func (vs *vSphereVMProvider) createdVirtualMachineFallthroughUpdate(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	// TODO: In the common case, we'll call directly into update right after create succeeds, and
	// can use the createArgs to avoid doing a bunch of lookup work again.

	return vs.updateVirtualMachine(vmCtx, vcVM, vcClient, createArgs)
}

// VMUpdatePropertiesSelector is the set of VM properties fetched at the start
// of UpdateVirtualMachine,
// It must be a super set of vmlifecycle.VMStatusPropertiesSelector[] since we
// may pass the properties collected here to vmlifecycle.UpdateStatus to avoid a
// second fetch of the VM properties.
var VMUpdatePropertiesSelector = []string{
	"config",
	"guest",
	"layoutEx",
	"resourcePool",
	"runtime",
	"summary",
}

func (vs *vSphereVMProvider) updateVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	vmCtx.Logger.V(4).Info("Updating VirtualMachine")

	{
		// Hack - create just enough of the Session that's needed for update

		if err := vcVM.Properties(
			vmCtx,
			vcVM.Reference(),
			VMUpdatePropertiesSelector,
			&vmCtx.MoVM); err != nil {

			return err
		}

		// Only reconcile connected VMs or if the connection state is empty.
		if cs := vmCtx.MoVM.Summary.Runtime.ConnectionState; cs != "" && cs !=
			vimtypes.VirtualMachineConnectionStateConnected {

			// Return a NoRequeueError so the VM is not requeued for
			// reconciliation.
			//
			// The watcher service ensures that VMs will be reconciled
			// immediately upon their summary.runtime.connectionState value
			// changing.
			//
			// TODO(akutz) Determine if we should surface some type of condition
			//             that indicates this state.
			return fmt.Errorf("failed to update VM: %w", pkgerr.NoRequeueError{
				Message: fmt.Sprintf("unsupported VM connection state: %s", cs),
			})
		}

		if vmCtx.MoVM.ResourcePool == nil {
			// Same error as govmomi VirtualMachine::ResourcePool().
			return fmt.Errorf("VM doesn't have a resourcePool")
		}

		clusterMoRef, err := vcenter.GetResourcePoolOwnerMoRef(
			vmCtx,
			vcVM.Client(),
			vmCtx.MoVM.ResourcePool.Value)
		if err != nil {
			return err
		}

		ses := &session.Session{
			K8sClient:    vs.k8sClient,
			Client:       vcClient.Client,
			Finder:       vcClient.Finder(),
			ClusterMoRef: clusterMoRef,
		}

		getUpdateArgsFn := func() (*vmUpdateArgs, error) {
			// TODO: Use createArgs if we already got them, except for:
			//       - createArgs.ConfigSpec.Crypto
			_ = createArgs
			return vs.vmUpdateGetArgs(vmCtx)
		}

		getResizeArgsFn := func() (*vmResizeArgs, error) {
			return vs.vmResizeGetArgs(vmCtx)
		}

		err = ses.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgsFn, getResizeArgsFn)
		if err != nil {
			return err
		}
	}

	// Back up the VM at the end after a successful update.  TKG nodes are skipped
	// from backup unless they specify the annotation to opt into backup.
	if !kubeutil.HasCAPILabels(vmCtx.VM.Labels) ||
		metav1.HasAnnotation(vmCtx.VM.ObjectMeta, vmopv1.ForceEnableBackupAnnotation) {

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
			BackupVersion:       fmt.Sprint(time.Now().UnixMilli()),
			ClassicDiskUUIDs:    GetAttachedClassicDiskUUIDs(vmCtx),
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
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) (retErr error) {

	defer func() {
		if retErr != nil {
			pkgcnd.MarkError(
				vmCtx.VM,
				vmopv1.VirtualMachineConditionPlacementReady,
				"NotReady",
				retErr)
		}
	}()

	placementConfigSpec, err := virtualmachine.CreateConfigSpecForPlacement(
		vmCtx,
		createArgs.ConfigSpec,
		createArgs.Storage.StorageClassToPolicyID)
	if err != nil {
		return err
	}

	pvcZones, err := kubeutil.GetPVCZoneConstraints(
		createArgs.Storage.StorageClasses,
		createArgs.Storage.PVCs)
	if err != nil {
		return err
	}

	constraints := placement.Constraints{
		ChildRPName: createArgs.ChildResourcePoolName,
		Zones:       pvcZones,
	}

	result, err := placement.Placement(
		vmCtx,
		vs.k8sClient,
		vcClient.VimClient(),
		vcClient.Finder(),
		placementConfigSpec,
		constraints)
	if err != nil {
		return err
	}

	if result.PoolMoRef.Value != "" {
		createArgs.ResourcePoolMoID = result.PoolMoRef.Value
	}

	if result.HostMoRef != nil {
		createArgs.HostMoID = result.HostMoRef.Value
	}

	if pkgcfg.FromContext(vmCtx).Features.FastDeploy {
		createArgs.DatacenterMoID = vcClient.Datacenter().Reference().Value
		createArgs.Datastores = make([]vmlifecycle.DatastoreRef, len(result.Datastores))
		for i := range result.Datastores {
			createArgs.Datastores[i].DiskKey = result.Datastores[i].DiskKey
			createArgs.Datastores[i].ForDisk = result.Datastores[i].ForDisk
			createArgs.Datastores[i].MoRef = result.Datastores[i].MoRef
			createArgs.Datastores[i].Name = result.Datastores[i].Name
			createArgs.Datastores[i].URL = result.Datastores[i].URL
			createArgs.Datastores[i].DiskFormats = result.Datastores[i].DiskFormats
		}
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
		if pkgcfg.FromContext(vmCtx).Features.FastDeploy {
			createArgs.ZoneName = result.ZoneName
		} else {
			if vmCtx.VM.Labels == nil {
				vmCtx.VM.Labels = map[string]string{}
			}
			// Note if the VM create fails for some reason, but this label gets updated on the k8s VM,
			// then this is the pre-assigned zone on later create attempts.
			vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] = result.ZoneName
		}
	}

	pkgcnd.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionPlacementReady)

	return nil
}

// vmCreateGetFolderAndRPMoIDs gets the MoIDs of the Folder and Resource Pool the VM will be created under.
func (vs *vSphereVMProvider) vmCreateGetFolderAndRPMoIDs(
	vmCtx pkgctx.VirtualMachineContext,
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
				vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID})

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
			vimtypes.ManagedObjectReference{Type: "Folder", Value: createArgs.FolderMoID})

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

// vmCreateGetSourceDiskPaths gets paths to the source disk(s) used to create
// the VM.
func (vs *vSphereVMProvider) vmCreateGetSourceDiskPaths(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	if len(createArgs.Datastores) == 0 {
		return errors.New("no compatible datastores")
	}

	var (
		datacenterID = vs.vcClient.Datacenter().Reference().Value
		datastoreID  = createArgs.Datastores[0].MoRef.Value
		itemID       = createArgs.ImageStatus.ProviderItemID
		itemVersion  = createArgs.ImageStatus.ProviderContentVersion
	)

	// Create/patch/get the VirtualMachineImageCache resource.
	obj := vmopv1.VirtualMachineImageCache{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pkgcfg.FromContext(vmCtx).PodNamespace,
			Name:      pkgutil.VMIName(itemID),
		},
	}
	if _, err := controllerutil.CreateOrPatch(
		vmCtx,
		vs.k8sClient,
		&obj,
		func() error {
			obj.Spec.ProviderID = itemID
			obj.Spec.ProviderVersion = itemVersion
			obj.AddLocation(datacenterID, datastoreID)
			return nil
		}); err != nil {
		return fmt.Errorf(
			"failed to createOrPatch image cache resource: %w", err)
	}

	// Check if the disks are cached.
	for i := range obj.Status.Locations {
		l := obj.Status.Locations[i]
		if l.DatacenterID == datacenterID && l.DatastoreID == datastoreID {
			if c := pkgcnd.Get(l, vmopv1.ReadyConditionType); c != nil {
				switch c.Status {
				case metav1.ConditionTrue:

					// Verify the disks are still cached. If the disks are
					// found to no longer exist, a reconcile request is enqueued
					// for the VMI cache object.
					if vs.vmCreateGetSourceDiskPathsVerify(
						vmCtx,
						vcClient,
						obj,
						l.Files) {

						// The location has the cached disks.
						vmCtx.Logger.Info("got source disks", "disks", l.Files)

						// Update the createArgs.DiskPaths with the paths from
						// the cached disks slice.
						for i := range l.Files {
							switch path.Ext(l.Files[i].ID) {
							case ".vmdk":
								createArgs.DiskPaths = append(createArgs.DiskPaths, l.Files[i].ID)
							default:
								createArgs.FilePaths = append(createArgs.FilePaths, l.Files[i].ID)
							}
						}

						return nil
					}

				case metav1.ConditionFalse:
					// The disks could not be cached at that location.
					return fmt.Errorf("failed to cache disks: %s", c.Message)
				}
			}
		}
	}

	// The cached disks are not yet ready, so return the following error.
	return pkgerr.VMICacheNotReadyError{
		Message:      "cached disks not ready",
		Name:         obj.Name,
		DatacenterID: datacenterID,
		DatastoreID:  datastoreID,
	}
}

// vmCreateGetSourceDiskPathsVerify verifies the provided disks are still
// available. If not, a reconcile request is enqueued for the VMI cache object.
func (vs *vSphereVMProvider) vmCreateGetSourceDiskPathsVerify(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client,
	obj vmopv1.VirtualMachineImageCache,
	srcDisks []vmopv1.VirtualMachineImageCacheFileStatus) bool {

	for i := range srcDisks {
		s := srcDisks[i]
		if err := pkgutil.DatastoreFileExists(
			vmCtx,
			vcClient.VimClient(),
			s.ID,
			vcClient.Datacenter()); err != nil {

			vmCtx.Logger.Error(err, "disk is invalid", "diskPath", s.ID)

			chanSource := cource.FromContextWithBuffer(
				vmCtx, "VirtualMachineImageCache", 100)
			chanSource <- event.GenericEvent{
				Object: &vmopv1.VirtualMachineImageCache{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: obj.Namespace,
						Name:      obj.Name,
					},
				},
			}

			return false
		}
	}

	return true
}

func (vs *vSphereVMProvider) vmCreateIsReady(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	if policy := createArgs.ResourcePolicy; policy != nil {
		// TODO: May want to do this as to filter the placement candidates.
		clusterModuleProvider := clustermodules.NewProvider(vcClient.RestClient())
		exists, err := vs.doClusterModulesExist(vmCtx, clusterModuleProvider, createArgs.ClusterMoRef, policy)
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

func (vs *vSphereVMProvider) vmCreateConcurrentAllowed(vmCtx pkgctx.VirtualMachineContext) (bool, func()) {
	maxDeployThreads := pkgcfg.FromContext(vmCtx).GetMaxDeployThreadsOnProvider()

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
	vmCtx pkgctx.VirtualMachineContext,
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

	return createArgs, nil
}

// vmCreateGetPrereqs returns the VMCreateArgs populated with the k8s objects required to
// create the VM on VC.
func (vs *vSphereVMProvider) vmCreateGetPrereqs(
	vmCtx pkgctx.VirtualMachineContext,
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
		return nil, apierrorsutil.NewAggregate(prereqErrs)
	}

	if !vmopv1util.IsClasslessVM(*vmCtx.VM) {
		// Only set VM Class field for non-synthesized classes.
		if f := pkgcfg.FromContext(vmCtx).Features; f.VMResize || f.VMResizeCPUMemory {
			vmopv1util.MustSetLastResizedAnnotation(vmCtx.VM, createArgs.VMClass)
		}
		vmCtx.VM.Status.Class = &common.LocalObjectRef{
			APIVersion: vmopv1.GroupVersion.String(),
			Kind:       createArgs.VMClass.Kind,
			Name:       createArgs.VMClass.Name,
		}
	}

	return createArgs, nil
}

func (vs *vSphereVMProvider) vmCreateGetVirtualMachineClass(
	vmCtx pkgctx.VirtualMachineContext,
	createArgs *VMCreateArgs) error {

	vmClass, err := GetVirtualMachineClass(vmCtx, vs.k8sClient)
	if err != nil {
		return err
	}

	createArgs.VMClass = vmClass

	return nil
}

func (vs *vSphereVMProvider) vmCreateGetVirtualMachineImage(
	vmCtx pkgctx.VirtualMachineContext,
	createArgs *VMCreateArgs) error {

	imageObj, imageSpec, imageStatus, err := GetVirtualMachineImageSpecAndStatus(vmCtx, vs.k8sClient)
	if err != nil {
		return err
	}

	createArgs.ImageObj = imageObj
	createArgs.ImageSpec = imageSpec
	createArgs.ImageStatus = imageStatus

	var providerRef common.LocalObjectRef
	if imageSpec.ProviderRef != nil {
		providerRef = *imageSpec.ProviderRef
	}

	// This is clunky, but we need to know how to use the image to create the VM. Our only supported
	// method is via the ContentLibrary, so check if this image was derived from a CL item.
	switch providerRef.Kind {
	case "ClusterContentLibraryItem", "ContentLibraryItem":
		createArgs.UseContentLibrary = true
		createArgs.ProviderItemID = imageStatus.ProviderItemID
	default:
		if !SkipVMImageCLProviderCheck {
			err := fmt.Errorf("unsupported image provider kind: %s", providerRef.Kind)
			pkgcnd.MarkError(vmCtx.VM, vmopv1.VirtualMachineConditionImageReady, "NotSupported", err)
			return err
		}
		// Testing only: we'll clone the source VM found in the Inventory.
		createArgs.UseContentLibrary = false
		createArgs.ProviderItemID = vmCtx.VM.Spec.Image.Name
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateGetSetResourcePolicy(
	vmCtx pkgctx.VirtualMachineContext,
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
	vmCtx pkgctx.VirtualMachineContext,
	createArgs *VMCreateArgs) error {

	bsData, err := GetVirtualMachineBootstrap(vmCtx, vs.k8sClient)
	if err != nil {
		return err
	}

	createArgs.BootstrapData = bsData

	return nil
}

func (vs *vSphereVMProvider) vmCreateGetStoragePrereqs(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	if pkgcfg.FromContext(vmCtx).Features.InstanceStorage {
		// To determine all the storage profiles, we need the class because of
		// the possibility of InstanceStorage volumes. If we weren't able to get
		// the class earlier, still check & set the storage condition because
		// instance storage usage is rare, it is helpful to report as many
		// prereqs as possible, and we'll reevaluate this once the class is
		// available.
		//
		// Add the class's instance storage disks - if any - to the VM.Spec.
		// Once the instance storage disks are added to the VM, they are set in
		// stone even if the class itself or the VM's assigned class changes.
		createArgs.HasInstanceStorage = AddInstanceStorageVolumes(
			vmCtx,
			createArgs.VMClass.Spec.Hardware.InstanceStorage)
	}

	vmStorageClass := vmCtx.VM.Spec.StorageClass
	if vmStorageClass == "" {
		cfg := vcClient.Config()

		// This will be true in WCP.
		if cfg.StorageClassRequired {
			err := fmt.Errorf("StorageClass is required but not specified")
			pkgcnd.MarkError(vmCtx.VM, vmopv1.VirtualMachineConditionStorageReady, "StorageClassRequired", err)
			return err
		}

		// Testing only for standalone gce2e.
		if cfg.Datastore == "" {
			err := fmt.Errorf("no Datastore provided in configuration")
			pkgcnd.MarkError(vmCtx.VM, vmopv1.VirtualMachineConditionStorageReady, "DatastoreNotFound", err)
			return err
		}

		datastore, err := vcClient.Finder().Datastore(vmCtx, cfg.Datastore)
		if err != nil {
			pkgcnd.MarkError(vmCtx.VM, vmopv1.VirtualMachineConditionStorageReady, "DatastoreNotFound", err)
			return fmt.Errorf("failed to find Datastore %s: %w", cfg.Datastore, err)
		}

		createArgs.DatastoreMoID = datastore.Reference().Value
	}

	vmStorage, err := storage.GetVMStorageData(vmCtx, vs.k8sClient)
	if err != nil {
		reason, msg := errToConditionReasonAndMessage(err)
		pkgcnd.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionStorageReady, reason, "%s", msg)
		return err
	}

	vmStorageProfileID := vmStorage.StorageClassToPolicyID[vmStorageClass]
	provisioningType, err := virtualmachine.GetDefaultDiskProvisioningType(vmCtx, vcClient, vmStorageProfileID)
	if err != nil {
		reason, msg := errToConditionReasonAndMessage(err)
		pkgcnd.MarkFalse(vmCtx.VM, vmopv1.VirtualMachineConditionStorageReady, reason, "%s", msg)
		return err
	}

	createArgs.Storage = vmStorage
	createArgs.StorageProvisioning = provisioningType
	createArgs.StorageProfileID = vmStorageProfileID
	pkgcnd.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionStorageReady)

	return nil
}

func (vs *vSphereVMProvider) vmCreateDoNetworking(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) error {

	networkSpec := vmCtx.VM.Spec.Network
	if networkSpec == nil || networkSpec.Disabled {
		pkgcnd.Delete(vmCtx.VM, vmopv1.VirtualMachineConditionNetworkReady)
		return nil
	}

	results, err := network.CreateAndWaitForNetworkInterfaces(
		vmCtx,
		vs.k8sClient,
		vcClient.VimClient(),
		vcClient.Finder(),
		nil, // Don't know the CCR yet (needed to resolve backings for NSX-T)
		networkSpec)
	if err != nil {
		pkgcnd.MarkError(vmCtx.VM, vmopv1.VirtualMachineConditionNetworkReady, "NotReady", err)
		return err
	}

	createArgs.NetworkResults = results
	pkgcnd.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionNetworkReady)

	return nil
}

func (vs *vSphereVMProvider) vmCreateGenConfigSpec(
	vmCtx pkgctx.VirtualMachineContext,
	createArgs *VMCreateArgs) error {

	// TODO: This is a partial dupe of what's done in the update path in the remaining Session code. I got
	// tired of trying to keep that in sync so we get to live with a frankenstein thing longer.

	var configSpec vimtypes.VirtualMachineConfigSpec
	if rawConfigSpec := createArgs.VMClass.Spec.ConfigSpec; len(rawConfigSpec) > 0 {
		vmClassConfigSpec, err := GetVMClassConfigSpec(vmCtx, rawConfigSpec)
		if err != nil {
			return err
		}
		configSpec = vmClassConfigSpec
	} else {
		configSpec = virtualmachine.ConfigSpecFromVMClassDevices(&createArgs.VMClass.Spec)
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
		configSpec,
		createArgs.VMClass.Spec,
		createArgs.ImageStatus,
		minCPUFreq)

	if pkgcfg.FromContext(vmCtx).Features.FastDeploy {
		if err := vs.vmCreateGenConfigSpecImage(vmCtx, createArgs); err != nil {
			return err
		}
		createArgs.ConfigSpec.VmProfile = []vimtypes.BaseVirtualMachineProfileSpec{
			&vimtypes.VirtualMachineDefinedProfileSpec{
				ProfileId: createArgs.StorageProfileID,
			},
		}
	}

	// Get the encryption class details for the VM.
	if pkgcfg.FromContext(vmCtx).Features.BringYourOwnEncryptionKey {
		for _, r := range vmconfig.FromContext(vmCtx) {
			if err := r.Reconcile(
				vmCtx,
				vs.k8sClient,
				vs.vcClient.VimClient(),
				vmCtx.VM,
				vmCtx.MoVM,
				&createArgs.ConfigSpec); err != nil {

				return err
			}
		}
	}

	if err := vs.vmCreateGenConfigSpecExtraConfig(vmCtx, createArgs); err != nil {
		return err
	}

	if err := vs.vmCreateGenConfigSpecChangeBootDiskSize(vmCtx, createArgs); err != nil {
		return err
	}

	if err := vs.vmCreateGenConfigSpecZipNetworkInterfaces(vmCtx, createArgs); err != nil {
		return err
	}

	return nil
}

func (vs *vSphereVMProvider) vmCreateGenConfigSpecImage(
	vmCtx pkgctx.VirtualMachineContext,
	createArgs *VMCreateArgs) (retErr error) {

	if createArgs.ImageStatus.Type != "OVF" {
		return nil
	}

	var (
		itemID      = createArgs.ImageStatus.ProviderItemID
		itemVersion = createArgs.ImageStatus.ProviderContentVersion
	)

	if itemID == "" {
		return errors.New("empty image provider item id")
	}
	if itemVersion == "" {
		return errors.New("empty image provider content version")
	}

	var (
		vmiCache    vmopv1.VirtualMachineImageCache
		vmiCacheKey = ctrlclient.ObjectKey{
			Namespace: pkgcfg.FromContext(vmCtx).PodNamespace,
			Name:      pkgutil.VMIName(itemID),
		}
	)

	// Get the VirtualMachineImageCache resource.
	if err := vs.k8sClient.Get(
		vmCtx,
		vmiCacheKey,
		&vmiCache); err != nil {

		return fmt.Errorf(
			"failed to get vmi cache object: %w, %w",
			err,
			pkgerr.VMICacheNotReadyError{Name: vmiCacheKey.Name})
	}

	// Check if the OVF data is ready.
	c := pkgcnd.Get(vmiCache, vmopv1.VirtualMachineImageCacheConditionOVFReady)
	switch {
	case c != nil && c.Status == metav1.ConditionFalse:

		return fmt.Errorf(
			"failed to get ovf: %s: %w",
			c.Message,
			pkgerr.VMICacheNotReadyError{Name: vmiCacheKey.Name})

	case c == nil,
		c.Status != metav1.ConditionTrue,
		vmiCache.Status.OVF == nil,
		vmiCache.Status.OVF.ProviderVersion != itemVersion:

		return pkgerr.VMICacheNotReadyError{
			Message: "cached ovf not ready",
			Name:    vmiCacheKey.Name,
		}
	}

	// Get the OVF data.
	var ovfConfigMap corev1.ConfigMap
	if err := vs.k8sClient.Get(
		vmCtx,
		ctrlclient.ObjectKey{
			Namespace: vmiCache.Namespace,
			Name:      vmiCache.Status.OVF.ConfigMapName,
		},
		&ovfConfigMap); err != nil {

		return fmt.Errorf(
			"failed to get ovf configmap: %w, %w",
			err,
			pkgerr.VMICacheNotReadyError{Name: vmiCacheKey.Name})
	}

	var ovfEnvelope ovf.Envelope
	if err := yaml.Unmarshal(
		[]byte(ovfConfigMap.Data["value"]), &ovfEnvelope); err != nil {

		return fmt.Errorf("failed to unmarshal ovf yaml into envelope: %w", err)
	}

	ovfConfigSpec, err := ovfEnvelope.ToConfigSpec()
	if err != nil {
		return fmt.Errorf("failed to transform ovf to config spec: %w", err)
	}

	if createArgs.ConfigSpec.GuestId == "" {
		createArgs.ConfigSpec.GuestId = ovfConfigSpec.GuestId
	}

	// Inherit the image's vAppConfig.
	createArgs.ConfigSpec.VAppConfig = ovfConfigSpec.VAppConfig

	// Merge the image's extra config.
	if srcEC := ovfConfigSpec.ExtraConfig; len(srcEC) > 0 {
		if dstEC := createArgs.ConfigSpec.ExtraConfig; len(dstEC) == 0 {
			// The current config spec doesn't have any extra config, so just
			// set it to use the extra config from the image.
			createArgs.ConfigSpec.ExtraConfig = srcEC
		} else {
			// The current config spec already has extra config, so merge the
			// extra config from the image, but do not overwrite any keys that
			// already exist in the current config spec.
			createArgs.ConfigSpec.ExtraConfig = object.OptionValueList(dstEC).
				Join(srcEC...)
		}
	}

	// Inherit the image's disks and their controllers.
	pkgutil.CopyStorageControllersAndDisks(
		&createArgs.ConfigSpec,
		ovfConfigSpec,
		createArgs.StorageProfileID)

	return nil
}

func (vs *vSphereVMProvider) vmCreateGenConfigSpecExtraConfig(
	vmCtx pkgctx.VirtualMachineContext,
	createArgs *VMCreateArgs) error {

	ecMap := maps.Clone(vs.globalExtraConfig)

	if v, exists := ecMap[constants.ExtraConfigRunContainerKey]; exists {
		// The local-vcsim config sets the JSON_EXTRA_CONFIG with RUN.container so vcsim
		// creates a container for the VM. The only current use of this template function
		// is to fill in the {{.Name}} from the images status.
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
		k := constants.ExtraConfigRunContainerKey
		ecMap[k] = renderTemplateFn(k, v)
	}

	if pkgutil.HasVirtualPCIPassthroughDeviceChange(createArgs.ConfigSpec.DeviceChange) {
		mmioSize := vmCtx.VM.Annotations[constants.PCIPassthruMMIOOverrideAnnotation]
		if mmioSize == "" {
			mmioSize = constants.PCIPassthruMMIOSizeDefault
		}
		if mmioSize != "0" {
			ecMap[constants.PCIPassthruMMIOExtraConfigKey] = constants.ExtraConfigTrue
			ecMap[constants.PCIPassthruMMIOSizeExtraConfigKey] = mmioSize
		}
	}

	// The ConfigSpec's current ExtraConfig values (that came from the class)
	// take precedence over what was set here.
	createArgs.ConfigSpec.ExtraConfig = pkgutil.OptionValues(
		createArgs.ConfigSpec.ExtraConfig).
		Append(pkgutil.OptionValuesFromMap(ecMap)...)

	// Leave constants.VMOperatorV1Alpha1ExtraConfigKey for the update path (if that's still even needed)

	return nil
}

func (vs *vSphereVMProvider) vmCreateGenConfigSpecChangeBootDiskSize(
	vmCtx pkgctx.VirtualMachineContext,
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
	vmCtx pkgctx.VirtualMachineContext,
	createArgs *VMCreateArgs) error {

	if vmCtx.VM.Spec.Network == nil || vmCtx.VM.Spec.Network.Disabled {
		pkgutil.RemoveDevicesFromConfigSpec(&createArgs.ConfigSpec, pkgutil.IsEthernetCard)
		return nil
	}

	resultsIdx := 0
	var unmatchedEthDevices []int

	for idx := range createArgs.ConfigSpec.DeviceChange {
		spec := createArgs.ConfigSpec.DeviceChange[idx].GetVirtualDeviceConfigSpec()
		if spec == nil || !pkgutil.IsEthernetCard(spec.Device) {
			continue
		}

		device := spec.Device
		ethCard := device.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()

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

		createArgs.ConfigSpec.DeviceChange = append(createArgs.ConfigSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    ethCardDev,
		})
	}

	return nil
}

func (vs *vSphereVMProvider) vmUpdateGetArgs(
	vmCtx pkgctx.VirtualMachineContext) (*vmUpdateArgs, error) {

	vmClass, err := GetVirtualMachineClass(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	var resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
	if vmCtx.VM.Annotations[pkgconst.ClusterModuleNameAnnotationKey] != "" {
		// Post create the resource policy is only needed to set the cluster module.
		resourcePolicy, err = GetVMSetResourcePolicy(vmCtx, vs.k8sClient)
		if err != nil {
			return nil, err
		}
		if resourcePolicy == nil {
			return nil, fmt.Errorf("cannot set cluster module without resource policy")
		}
	}

	bsData, err := GetVirtualMachineBootstrap(vmCtx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	updateArgs := &vmUpdateArgs{
		VMClass:        vmClass,
		ResourcePolicy: resourcePolicy,
		BootstrapData:  bsData,
	}

	ecMap := maps.Clone(vs.globalExtraConfig)
	maps.DeleteFunc(ecMap, func(k string, v string) bool {
		// Remove keys that we only want set on create.
		return k == constants.ExtraConfigRunContainerKey
	})
	updateArgs.ExtraConfig = ecMap

	if res := vmClass.Spec.Policies.Resources; !res.Requests.Cpu.IsZero() || !res.Limits.Cpu.IsZero() {
		freq, err := vs.getOrComputeCPUMinFrequency(vmCtx)
		if err != nil {
			return nil, err
		}
		updateArgs.MinCPUFreq = freq
	}

	var configSpec vimtypes.VirtualMachineConfigSpec
	if rawConfigSpec := updateArgs.VMClass.Spec.ConfigSpec; len(rawConfigSpec) > 0 {
		vmClassConfigSpec, err := GetVMClassConfigSpec(vmCtx, rawConfigSpec)
		if err != nil {
			return nil, err
		}
		configSpec = vmClassConfigSpec
	}

	updateArgs.ConfigSpec = virtualmachine.CreateConfigSpec(
		vmCtx,
		configSpec,
		updateArgs.VMClass.Spec,
		vmopv1.VirtualMachineImageStatus{},
		updateArgs.MinCPUFreq)

	return updateArgs, nil
}

func (vs *vSphereVMProvider) vmResizeGetArgs(
	vmCtx pkgctx.VirtualMachineContext) (*vmResizeArgs, error) {

	resizeArgs := &vmResizeArgs{}

	if !vmopv1util.IsClasslessVM(*vmCtx.VM) {
		vmClass, err := GetVirtualMachineClass(vmCtx, vs.k8sClient)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
		} else {
			resizeArgs.VMClass = &vmClass
		}
	}

	if resizeArgs.VMClass != nil {
		var minCPUFreq uint64

		if res := resizeArgs.VMClass.Spec.Policies.Resources; !res.Requests.Cpu.IsZero() || !res.Limits.Cpu.IsZero() {
			freq, err := vs.getOrComputeCPUMinFrequency(vmCtx)
			if err != nil {
				return nil, err
			}
			minCPUFreq = freq
		}

		var configSpec vimtypes.VirtualMachineConfigSpec
		if rawConfigSpec := resizeArgs.VMClass.Spec.ConfigSpec; len(rawConfigSpec) > 0 {
			vmClassConfigSpec, err := GetVMClassConfigSpec(vmCtx, rawConfigSpec)
			if err != nil {
				return nil, err
			}
			configSpec = vmClassConfigSpec
		}

		resizeArgs.ConfigSpec = virtualmachine.CreateConfigSpec(
			vmCtx,
			configSpec,
			resizeArgs.VMClass.Spec,
			vmopv1.VirtualMachineImageStatus{},
			minCPUFreq)

	}

	return resizeArgs, nil
}
