// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/waitgroup"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	imgregv1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha2"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/client"
	vcconfig "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/contentlibrary"
	vccreds "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/credentials"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	vsclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	storutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/storage"
)

const (
	// taskHistoryCollectorPageSize represents the max count to read from task manager in one iteration.
	taskHistoryCollectorPageSize = 10
)

type vSphereVMProvider struct {
	k8sClient         ctrlclient.Client
	eventRecorder     record.Recorder
	globalExtraConfig map[string]string
	minCPUFreq        uint64

	vcClientLock sync.Mutex
	vcClientGen  *vcClientGeneration
}

// vcClientGeneration pairs a vSphere client with a count of callers
// currently using it, so a superseded client is not logged out while it
// may still be in active use.
type vcClientGeneration struct {
	client *vcclient.Client
	wg     waitgroup.SafeWaitGroup
}

// vcClientRelease guards a single release of an acquired vcClientGeneration
// reference. The returned func is safe to call more than once; only the
// first call has an effect. If a caller loses the returned func without
// calling it, a cleanup runs as a backstop so the generation's WaitGroup is
// not held indefinitely.
type vcClientRelease struct {
	once  sync.Once
	done  func()
	clean runtime.Cleanup
}

func (r *vcClientRelease) release() {
	r.once.Do(func() {
		r.clean.Stop()
		r.done()
	})
}

func newVcClientRelease(done func()) func() {
	r := &vcClientRelease{done: done}
	r.clean = runtime.AddCleanup(r, func(fn func()) { fn() }, done)
	return r.release
}

func NewVSphereVMProviderFromClient(
	ctx context.Context,
	client ctrlclient.Client,
	recorder record.Recorder) providers.VirtualMachineProviderInterface {

	p := &vSphereVMProvider{
		k8sClient:         client,
		eventRecorder:     recorder,
		globalExtraConfig: getExtraConfig(ctx),
	}

	ovfcache.SetGetter(ctx, p.getOvfEnvelope)

	return p
}

func getExtraConfig(ctx context.Context) map[string]string {
	ec := map[string]string{
		constants.EnableDiskUUIDExtraConfigKey:       constants.ExtraConfigTrue,
		constants.GOSCIgnoreToolsCheckExtraConfigKey: constants.ExtraConfigTrue,
	}

	if jsonEC := pkgcfg.FromContext(ctx).JSONExtraConfig; jsonEC != "" {
		extraConfig := make(map[string]string)

		if err := json.Unmarshal([]byte(jsonEC), &extraConfig); err != nil {
			// This is only set in testing so make errors fatal.
			panic(fmt.Sprintf("invalid JSON_EXTRA_CONFIG envvar: %q %v", jsonEC, err))
		}

		for k, v := range extraConfig {
			ec[k] = v
		}
	}

	return ec
}

// getVcClient returns the current vSphere client along with a release func
// that the caller MUST call (typically via defer) once it is done using the
// client. Until release is called, the client is guaranteed not to be
// logged out from under the caller.
func (vs *vSphereVMProvider) getVcClient(
	ctx context.Context) (*vcclient.Client, func(), error) {
	vs.vcClientLock.Lock()
	defer vs.vcClientLock.Unlock()

	if gen := vs.vcClientGen; gen != nil {
		if err := gen.wg.Add(1); err != nil {
			return nil, nil, fmt.Errorf("failed to acquire vc client: %w", err)
		}
		return gen.client, newVcClientRelease(gen.wg.Done), nil
	}

	config, err := vcconfig.GetProviderConfig(ctx, vs.k8sClient)
	if err != nil {
		return nil, nil, err
	}

	vcClient, err := vcclient.NewClient(ctx, config)
	if err != nil {
		return nil, nil, err
	}

	gen := &vcClientGeneration{client: vcClient}
	vs.vcClientGen = gen

	if err := gen.wg.Add(1); err != nil {
		return nil, nil, fmt.Errorf("failed to acquire vc client: %w", err)
	}

	return gen.client, newVcClientRelease(gen.wg.Done), nil
}

// rotateVcClientGen retires the current client generation, if one exists and
// isStale returns true for it, so the next call to getVcClient creates a new
// client. It then waits, without blocking the caller, for existing callers
// of the retired generation to finish before logging it out.
func (vs *vSphereVMProvider) rotateVcClientGen(
	ctx context.Context, isStale func(*vcClientGeneration) bool) {

	oldGen := func() *vcClientGeneration {
		vs.vcClientLock.Lock()
		defer vs.vcClientLock.Unlock()

		gen := vs.vcClientGen
		if gen == nil || !isStale(gen) {
			return nil
		}

		vs.vcClientGen = nil
		return gen
	}()

	if oldGen == nil {
		return
	}

	// Use a context that is not cancelled when ctx is, so the logout
	// below still runs even though this call returns immediately and
	// ctx may be cancelled long before the old generation's existing
	// callers finish using it.
	logoutCtx := context.WithoutCancel(ctx)
	go func() {
		oldGen.wg.Wait()
		oldGen.client.Logout(logoutCtx)
	}()
}

func (vs *vSphereVMProvider) UpdateVcPNID(
	ctx context.Context, vcPNID, vcPort string) error {
	updated, err := vcconfig.UpdateVcInConfigMap(
		ctx, vs.k8sClient, vcPNID, vcPort)
	if err != nil || !updated {
		return err
	}

	vs.rotateVcClientGen(ctx, func(*vcClientGeneration) bool { return true })

	return nil
}

func (vs *vSphereVMProvider) UpdateVcCreds(
	ctx context.Context, data map[string][]byte) error {
	newVcCreds, err := vccreds.ExtractVCCredentials(data)
	if err != nil {
		return err
	}

	vs.rotateVcClientGen(ctx, func(gen *vcClientGeneration) bool {
		return gen.client.Config().VcCreds != newVcCreds
	})

	return nil
}

// SyncVirtualMachineImage syncs the vmi object with the OVF Envelope retrieved
// from the content library item object.
func (vs *vSphereVMProvider) SyncVirtualMachineImage(
	ctx context.Context,
	cli, vmi ctrlclient.Object) error {

	var (
		itemID      string
		itemVersion string
		itemType    imgregv1a1.ContentLibraryItemType
	)

	switch cli := cli.(type) {
	case *imgregv1a1.ContentLibraryItem:
		itemID = string(cli.Spec.UUID)
		itemVersion = cli.Status.ContentVersion
		itemType = cli.Status.Type
	case *imgregv1a1.ClusterContentLibraryItem:
		itemID = string(cli.Spec.UUID)
		itemVersion = cli.Status.ContentVersion
		itemType = cli.Status.Type
	case *imgregv1.ContentLibraryItem:
		itemID = cli.Spec.ID
		itemVersion = cli.Status.ContentVersion
		itemType = imgregv1a1.ContentLibraryItemType(cli.Status.Type)
	case *imgregv1.ClusterContentLibraryItem:
		itemID = cli.Spec.ID
		itemVersion = cli.Status.ContentVersion
		itemType = imgregv1a1.ContentLibraryItemType(cli.Status.Type)
	default:
		return fmt.Errorf("unexpected content library item K8s object type %T", cli)
	}

	logger := pkglog.FromContextOrDefault(ctx).V(4).WithValues(
		"vmiName", vmi.GetName(), "cliName", cli.GetName())

	var allowed bool
	switch itemType {
	case imgregv1a1.ContentLibraryItemTypeOvf:
		allowed = true
	case imgregv1a1.ContentLibraryItemType("VM"),
		imgregv1a1.ContentLibraryItemType("vm"):
		allowed = pkgcfg.FromContext(ctx).Features.InventoryContentLibrary
	default:
		allowed = false
	}

	if !allowed {
		logger.Info(
			"Skip syncing VMI content as the library item is not supported",
			"itemType", itemType)
		return nil
	}

	if pkgcfg.FromContext(ctx).Features.FastDeploy {
		return vs.syncVirtualMachineImageFastDeploy(ctx, vmi, logger, itemID, itemVersion)
	}
	return vs.syncVirtualMachineImage(ctx, vmi, itemID, itemVersion)
}

func (vs *vSphereVMProvider) syncVirtualMachineImageFastDeploy(
	ctx context.Context,
	vmi ctrlclient.Object,
	logger logr.Logger,
	itemID,
	itemVersion string) error {

	// Create or patch the VMICache object.
	vmiCache := vmopv1.VirtualMachineImageCache{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pkgcfg.FromContext(ctx).PodNamespace,
			Name:      pkgutil.VMIName(itemID),
		},
	}
	if _, err := controllerutil.CreateOrPatch(
		ctx,
		vs.k8sClient,
		&vmiCache,
		func() error {
			vmiCache.Spec.ProviderID = itemID
			vmiCache.Spec.ProviderVersion = itemVersion
			return nil
		}); err != nil {
		return fmt.Errorf(
			"failed to createOrPatch image cache resource: %w", err)
	}

	hardwareReadyCondition := pkgcond.Get(
		vmiCache,
		vmopv1.VirtualMachineImageCacheConditionHardwareReady)

	switch {
	case hardwareReadyCondition != nil &&
		hardwareReadyCondition.Status == metav1.ConditionFalse:

		return fmt.Errorf(
			"failed to get hardware: %s: %w",
			hardwareReadyCondition.Message,
			pkgerr.VMICacheNotReadyError{Name: vmiCache.Name})

	case hardwareReadyCondition == nil,
		hardwareReadyCondition.Status != metav1.ConditionTrue:

		logger.V(4).Info(
			"Skip sync VMI",
			"vmiCache.hardwareReadyCondition", hardwareReadyCondition)

		return pkgerr.VMICacheNotReadyError{Name: vmiCache.Name}
	}

	if strings.HasPrefix(itemID, "vm-") {
		return vs.syncVirtualMachineImageFastDeployVM(
			ctx,
			vmi,
			itemID)
	}

	return vs.syncVirtualMachineImageFastDeployOVF(
		ctx,
		vmiCache,
		itemVersion,
		logger,
		vmi)
}

func (vs *vSphereVMProvider) syncVirtualMachineImageFastDeployOVF(
	ctx context.Context,
	vmiCache vmopv1.VirtualMachineImageCache,
	itemVersion string,
	logger logr.Logger,
	vmi ctrlclient.Object) error {

	switch {
	case vmiCache.Status.OVF == nil,
		vmiCache.Status.OVF.ProviderVersion != itemVersion:

		logger.V(4).Info(
			"Skip sync VMI",
			"vmiCache.status.ovf", vmiCache.Status.OVF,
			"expectedContentVersion", itemVersion)

		return pkgerr.VMICacheNotReadyError{Name: vmiCache.Name}
	}

	// Get the OVF data.
	var (
		ovfConfigMap    corev1.ConfigMap
		ovfConfigMapKey = ctrlclient.ObjectKey{
			Namespace: vmiCache.Namespace,
			Name:      vmiCache.Status.OVF.ConfigMapName,
		}
	)
	if err := vs.k8sClient.Get(
		ctx,
		ovfConfigMapKey,
		&ovfConfigMap); err != nil {

		return fmt.Errorf(
			"failed to get ovf configmap: %w, %w",
			err,
			pkgerr.VMICacheNotReadyError{Name: vmiCache.Name})
	}

	var ovfEnvelope ovf.Envelope
	if err := yaml.Unmarshal(
		[]byte(ovfConfigMap.Data["value"]), &ovfEnvelope); err != nil {

		return fmt.Errorf(
			"failed to unmarshal ovf yaml into envelope: %w", err)
	}

	if err := contentlibrary.UpdateVmiWithOvfEnvelope(
		vmi,
		ovfEnvelope); err != nil {

		return fmt.Errorf("failed to update vmi from ovf: %w", err)
	}

	return nil
}

func (vs *vSphereVMProvider) syncVirtualMachineImageFastDeployVM(
	ctx context.Context,
	vmi ctrlclient.Object,
	vmMoID string) error {

	vcClient, release, err := vs.getVcClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get a vc client for image vm: %w", err)
	}
	defer release()

	vm := object.NewVirtualMachine(
		vcClient.VimClient(),
		vimtypes.ManagedObjectReference{
			Type:  string(vimtypes.ManagedObjectTypeVirtualMachine),
			Value: vmMoID,
		})

	var moVM mo.VirtualMachine
	if err := vm.Properties(
		ctx,
		vm.Reference(),
		[]string{"config", "guest", "layoutEx", "summary"},
		&moVM); err != nil {

		return fmt.Errorf("failed to get properties for image vm: %w", err)
	}

	contentlibrary.UpdateVmiWithVirtualMachine(ctx, vmi, moVM)
	return nil
}

func (vs *vSphereVMProvider) syncVirtualMachineImage(
	ctx context.Context,
	vmi ctrlclient.Object,
	itemID,
	itemVersion string) error {

	ovfEnvelope, err := ovfcache.GetOVFEnvelope(ctx, itemID, itemVersion)
	if err != nil {
		return fmt.Errorf("failed to get OVF envelope for library item %q: %w", itemID, err)
	}

	if ovfEnvelope == nil {
		return fmt.Errorf("OVF envelope is nil for library item %q", itemID)
	}

	if err := contentlibrary.UpdateVmiWithOvfEnvelope(
		vmi,
		*ovfEnvelope); err != nil {

		return fmt.Errorf("failed to update vmi from ovf: %w", err)
	}

	return nil
}

func (vs *vSphereVMProvider) getOvfEnvelope(
	ctx context.Context, itemID string) (*ovf.Envelope, error) {

	client, release, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	p := contentlibrary.NewProvider(ctx, client.RestClient())
	return p.RetrieveOvfEnvelopeByLibraryItemID(ctx, itemID)
}

// GetItemFromLibraryByName get the library item from specified content library by its name.
// Do not return error if the item doesn't exist in the content library.
func (vs *vSphereVMProvider) GetItemFromLibraryByName(ctx context.Context,
	contentLibrary, itemName string) (*library.Item, error) {

	pkglog.FromContextOrDefault(ctx).V(4).Info("Get item from ContentLibrary",
		"UUID", contentLibrary, "item name", itemName)

	client, release, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	contentLibraryProvider := contentlibrary.NewProvider(ctx, client.RestClient())
	return contentLibraryProvider.GetLibraryItem(ctx, contentLibrary, itemName, false)
}

func (vs *vSphereVMProvider) GetItemFromInventoryByName(
	ctx context.Context,
	contentLibrary, itemName string) (object.Reference, error) {

	client, release, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	c := client.VimClient()

	searchIndex := object.NewSearchIndex(c)

	folderRef := vimtypes.ManagedObjectReference{
		Type:  string(vimtypes.ManagedObjectTypeFolder),
		Value: contentLibrary,
	}

	vm, err := searchIndex.FindChild(ctx, folderRef, itemName)
	if err != nil {
		return nil, fmt.Errorf("failed to find child vm %s: %w", itemName, err)
	}

	return vm, nil
}

func (vs *vSphereVMProvider) ContainsExtraConfigEntry(
	ctx context.Context,
	objVM *object.VirtualMachine,
	key, value string) (bool, error) {

	var moVM mo.VirtualMachine
	if err := objVM.Properties(ctx, objVM.Reference(), []string{"config.extraConfig"}, &moVM); err != nil {
		return false, err
	}

	if moVM.Config == nil {
		return false, fmt.Errorf("configInfo is nil for VM %q", objVM.Reference())
	}

	extraConfig := object.OptionValueList(moVM.Config.ExtraConfig).StringMap()
	if ecValue, ok := extraConfig[key]; ok {
		if ecValue == value {
			return true, nil
		}
	}

	return false, nil
}

func (vs *vSphereVMProvider) UpdateContentLibraryItem(ctx context.Context, itemID, newName string, newDescription *string) error {
	pkglog.FromContextOrDefault(ctx).V(4).Info("Update Content Library Item", "itemID", itemID)

	client, release, err := vs.getVcClient(ctx)
	if err != nil {
		return err
	}
	defer release()

	contentLibraryProvider := contentlibrary.NewProvider(ctx, client.RestClient())
	return contentLibraryProvider.UpdateLibraryItem(ctx, itemID, newName, newDescription)
}

func (vs *vSphereVMProvider) getVM(
	vmCtx pkgctx.VirtualMachineContext,
	client *vcclient.Client,
	notFoundReturnErr bool) (*object.VirtualMachine, error) {

	vcVM, err := vcenter.GetVirtualMachine(vmCtx, client.VimClient(), client.Datacenter())
	if err != nil {
		return nil, err
	}

	if vcVM == nil && notFoundReturnErr {
		return nil, fmt.Errorf("VirtualMachine %q was not found on VC", vmCtx.VM.Name)
	}

	return vcVM, nil
}

func (vs *vSphereVMProvider) getOrComputeCPUMinFrequency(ctx context.Context) (uint64, error) {
	minFreq := atomic.LoadUint64(&vs.minCPUFreq)
	if minFreq == 0 {
		// The infra controller hasn't finished ComputeCPUMinFrequency() yet, so try to
		// compute that value now.
		var err error
		minFreq, err = vs.computeCPUMinFrequency(ctx)
		if err != nil {
			// minFreq may be non-zero in case of partial success.
			return minFreq, err
		}

		// Update value if not updated already.
		atomic.CompareAndSwapUint64(&vs.minCPUFreq, 0, minFreq)
	}

	return minFreq, nil
}

func (vs *vSphereVMProvider) ComputeCPUMinFrequency(ctx context.Context) error {
	minFreq, err := vs.computeCPUMinFrequency(ctx)
	if err != nil {
		// Might have a partial success (non-zero freq): store that if we haven't updated
		// the min freq yet, and let the controller retry. This whole min CPU freq thing
		// is kind of unfortunate & busted.
		atomic.CompareAndSwapUint64(&vs.minCPUFreq, 0, minFreq)
		return err
	}

	atomic.StoreUint64(&vs.minCPUFreq, minFreq)
	return nil
}

func (vs *vSphereVMProvider) computeCPUMinFrequency(ctx context.Context) (uint64, error) {
	// Get all the availability zones in order to calculate the minimum
	// CPU frequencies for each of the zones' vSphere clusters.
	availabilityZones, err := topology.GetAvailabilityZones(ctx, vs.k8sClient)
	if err != nil {
		return 0, err
	}

	client, release, err := vs.getVcClient(ctx)
	if err != nil {
		return 0, err
	}
	defer release()

	var errs []error

	var minFreq uint64
	for _, az := range availabilityZones {
		moIDs := az.Spec.ClusterComputeResourceMoIDs
		if len(moIDs) == 0 {
			moIDs = []string{az.Spec.ClusterComputeResourceMoId} // HA TEMP
		}

		for _, moID := range moIDs {
			ccr := object.NewClusterComputeResource(client.VimClient(),
				vimtypes.ManagedObjectReference{Type: "ClusterComputeResource", Value: moID})

			freq, err := vcenter.ClusterMinCPUFreq(ctx, ccr)
			if err != nil {
				errs = append(errs, err)
			} else if minFreq == 0 || freq < minFreq {
				minFreq = freq
			}
		}
	}

	return minFreq, apierrorsutil.NewAggregate(errs)
}

func (vs *vSphereVMProvider) GetTasksByActID(ctx context.Context, vm *vmopv1.VirtualMachine, actID string) (_ []vimtypes.TaskInfo, retErr error) {
	ctx = pkgctx.WithVCOpID(ctx, vm, "getTasksByActID")
	logger := pkglog.FromContextOrDefault(ctx)

	vcClient, release, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	taskManager := task.NewManager(vcClient.VimClient())
	filterSpec := vimtypes.TaskFilterSpec{}
	if vm != nil {
		vmCtx := pkgctx.NewVirtualMachineContext(ctx, vm)
		vcVM, err := vs.getVM(vmCtx, vcClient, true)
		if err != nil {
			return nil, fmt.Errorf("error fetching VM from VC: %w", err)
		}

		filterSpec.Entity = &vimtypes.TaskFilterSpecByEntity{
			Entity:    vcVM.Reference(),
			Recursion: vimtypes.TaskFilterSpecRecursionOptionSelf,
		}
	} else {
		filterSpec.ActivationId = []string{actID}
	}

	collector, err := taskManager.CreateCollectorForTasks(ctx, filterSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to create collector for tasks: %w", err)
	}
	defer func() {
		err := collector.Destroy(ctx)
		if retErr == nil {
			retErr = err
		}
	}()

	taskList := make([]vimtypes.TaskInfo, 0)
	for {
		nextTasks, err := collector.ReadNextTasks(ctx, taskHistoryCollectorPageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to read next tasks: %w", err)
		}
		if len(nextTasks) == 0 {
			break
		}

		if vm != nil {
			for _, taskInfo := range nextTasks {
				if strings.Contains(taskInfo.ActivationId, actID) {
					taskList = append(taskList, taskInfo)
				}
			}
		} else {
			taskList = append(taskList, nextTasks...)
		}
	}

	logger.V(5).Info("found tasks", "actID", actID, "tasks", taskList)
	return taskList, nil
}

func (vs *vSphereVMProvider) DoesProfileSupportEncryption(
	ctx context.Context,
	profileID string) (bool, error) {

	c, release, err := vs.getVcClient(ctx)
	if err != nil {
		return false, err
	}
	defer release()

	return c.PbmClient().SupportsEncryption(ctx, profileID)
}

func (vs *vSphereVMProvider) GetStoragePolicyStatus(
	ctx context.Context, profileID string) (infrav1.StoragePolicyStatus, error) {

	c, release, err := vs.getVcClient(ctx)
	if err != nil {
		return infrav1.StoragePolicyStatus{}, err
	}
	defer release()

	return storutil.GetStoragePolicyStatus(
		ctx,
		vs.k8sClient,
		c.VimClient(),
		c.PbmClient(),
		profileID)
}

func (vs *vSphereVMProvider) VSphereClient(
	ctx context.Context) (*vsclient.Client, func(), error) {

	c, release, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	return c.Client, release, nil
}

// GetVirtualMachineConfigTarget returns the vSphere cluster's
// EnvironmentBrowser QueryConfigTarget and QueryConfigOptionDescriptor
// results for the cluster identified by clusterMoID.
func (vs *vSphereVMProvider) GetVirtualMachineConfigTarget(
	ctx context.Context,
	clusterMoID string) (*vimtypes.ConfigTarget, []vimtypes.VirtualMachineConfigOptionDescriptor, error) {
	vcClient, release, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	ccr := object.NewClusterComputeResource(vcClient.VimClient(),
		vimtypes.ManagedObjectReference{Type: "ClusterComputeResource", Value: clusterMoID})

	envBrowser, err := ccr.EnvironmentBrowser(ctx)
	if err != nil {
		var f *vimtypes.ManagedObjectNotFound

		if _, ok := fault.As(err, &f); ok {
			return nil, nil, fmt.Errorf("cluster %q not found: %w", clusterMoID, err)
		}

		return nil, nil, fmt.Errorf("failed to get environment browser for cluster %q: %w", clusterMoID, err)
	}

	configTarget, err := envBrowser.QueryConfigTarget(ctx, nil)
	if err != nil {
		var f *vimtypes.ManagedObjectNotFound

		if _, ok := fault.As(err, &f); ok {
			return nil, nil, fmt.Errorf("cluster %q not found: %w", clusterMoID, err)
		}

		return nil, nil, fmt.Errorf("failed to query config target for cluster %q: %w", clusterMoID, err)
	}

	configOptionDescriptors, err := envBrowser.QueryConfigOptionDescriptor(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query config option descriptor for cluster %q: %w", clusterMoID, err)
	}

	return configTarget, configOptionDescriptors, nil
}
