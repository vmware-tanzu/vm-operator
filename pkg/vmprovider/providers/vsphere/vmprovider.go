// Copyright (c) 2018-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	goctx "context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	vcconfig "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/vcenter"
)

const (
	VsphereVMProviderName = "vsphere"

	// taskHistoryCollectorPageSize represents the max count to read from task manager in one iteration.
	taskHistoryCollectorPageSize = 10

	ovfCacheMaxItem                 = 100
	ovfCacheItemExpiration          = 30 * time.Minute
	ovfCacheExpirationCheckInterval = 5 * time.Minute
)

var log = logf.Log.WithName(VsphereVMProviderName)

type VersionedOVFEnvelope struct {
	OvfEnvelope    *ovf.Envelope
	ContentVersion string
}

type vSphereVMProvider struct {
	k8sClient         ctrlruntime.Client
	eventRecorder     record.Recorder
	globalExtraConfig map[string]string
	minCPUFreq        uint64
	ovfCache          *util.Cache[VersionedOVFEnvelope]
	ovfCacheLockPool  *util.LockPool[string, *sync.RWMutex]

	vcClientLock sync.Mutex
	vcClient     *vcclient.Client
}

func NewVSphereVMProviderFromClient(
	client ctrlruntime.Client,
	recorder record.Recorder) vmprovider.VirtualMachineProviderInterface {
	ovfCache, ovfLockPool := InitOvfCacheAndLockPool(
		ovfCacheItemExpiration, ovfCacheExpirationCheckInterval, ovfCacheMaxItem)

	return &vSphereVMProvider{
		k8sClient:         client,
		eventRecorder:     recorder,
		globalExtraConfig: getExtraConfig(),
		ovfCache:          ovfCache,
		ovfCacheLockPool:  ovfLockPool,
	}
}

// InitOvfCacheAndLockPool initializes the ovf cache and lock pool that are used
// to cache the ovf envelope and lock the ovf envelope when it is being downloaded.
func InitOvfCacheAndLockPool(expireAfter, checkExpireInterval time.Duration, maxItems int) (
	*util.Cache[VersionedOVFEnvelope], *util.LockPool[string, *sync.RWMutex]) {
	ovfCache := util.NewCache[VersionedOVFEnvelope](expireAfter, checkExpireInterval, maxItems)
	ovfLockPool := &util.LockPool[string, *sync.RWMutex]{}

	// Clean up the lock pool when the ovf cache item expires.
	expiredChan := ovfCache.ExpiredChan()
	go func() {
		for k := range expiredChan {
			l := ovfLockPool.Get(k)
			// This could still delete an in-use lock if it's retrieved from the pool but not locked yet.
			// If it's already locked, this will wait until it's unlocked to delete it from the pool.
			l.Lock()
			ovfLockPool.Delete(k)
			l.Unlock()
		}
	}()

	return ovfCache, ovfLockPool
}

func getExtraConfig() map[string]string {
	ec := map[string]string{
		constants.EnableDiskUUIDExtraConfigKey:       constants.ExtraConfigTrue,
		constants.GOSCIgnoreToolsCheckExtraConfigKey: constants.ExtraConfigTrue,
	}

	if jsonEC := os.Getenv("JSON_EXTRA_CONFIG"); jsonEC != "" {
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

func (vs *vSphereVMProvider) getVcClient(ctx goctx.Context) (*vcclient.Client, error) {
	vs.vcClientLock.Lock()
	defer vs.vcClientLock.Unlock()

	if vs.vcClient != nil {
		return vs.vcClient, nil
	}

	config, err := vcconfig.GetProviderConfig(ctx, vs.k8sClient)
	if err != nil {
		return nil, err
	}

	vcClient, err := vcclient.NewClient(ctx, config)
	if err != nil {
		return nil, err
	}

	vs.vcClient = vcClient
	return vcClient, nil
}

func (vs *vSphereVMProvider) UpdateVcPNID(ctx goctx.Context, vcPNID, vcPort string) error {
	updated, err := vcconfig.UpdateVcInConfigMap(ctx, vs.k8sClient, vcPNID, vcPort)
	if err != nil || !updated {
		return err
	}

	// Our controller-runtime client does not cache ConfigMaps & Secrets, so the next time
	// getVcClient() is called, it will fetch newly updated CM.
	vs.clearAndLogoutVcClient(ctx)
	return nil
}

func (vs *vSphereVMProvider) ResetVcClient(ctx goctx.Context) {
	vs.clearAndLogoutVcClient(ctx)
}

func (vs *vSphereVMProvider) clearAndLogoutVcClient(ctx goctx.Context) {
	vs.vcClientLock.Lock()
	vcClient := vs.vcClient
	vs.vcClient = nil
	vs.vcClientLock.Unlock()

	if vcClient != nil {
		vcClient.Logout(ctx)
	}
}

// ListItemsFromContentLibrary list items from a content library.
func (vs *vSphereVMProvider) ListItemsFromContentLibrary(
	ctx goctx.Context,
	contentLibrary *vmopv1.ContentLibraryProvider) ([]string, error) {

	log.V(4).Info("Listing VirtualMachineImages from ContentLibrary",
		"name", contentLibrary.Name,
		"UUID", contentLibrary.Spec.UUID)

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}

	return client.ContentLibClient().ListLibraryItems(ctx, contentLibrary.Spec.UUID)
}

// GetVirtualMachineImageFromContentLibrary gets a VM image from a ContentLibrary.
func (vs *vSphereVMProvider) GetVirtualMachineImageFromContentLibrary(
	ctx goctx.Context,
	contentLibrary *vmopv1.ContentLibraryProvider,
	itemID string,
	currentCLImages map[string]vmopv1.VirtualMachineImage) (*vmopv1.VirtualMachineImage, error) {

	log.V(4).Info("Getting VirtualMachineImage from ContentLibrary",
		"name", contentLibrary.Name,
		"UUID", contentLibrary.Spec.UUID)

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}

	return client.ContentLibClient().VirtualMachineImageResourceForLibrary(
		ctx,
		itemID,
		contentLibrary.Spec.UUID,
		currentCLImages)
}

// SyncVirtualMachineImage syncs the vmi object with the OVF Envelope retrieved from the cli object.
func (vs *vSphereVMProvider) SyncVirtualMachineImage(ctx goctx.Context, cli, vmi ctrlruntime.Object) error {
	var itemID, contentVersion string
	switch cli := cli.(type) {
	case *imgregv1a1.ContentLibraryItem:
		itemID = string(cli.Spec.UUID)
		contentVersion = cli.Status.ContentVersion
	case *imgregv1a1.ClusterContentLibraryItem:
		itemID = string(cli.Spec.UUID)
		contentVersion = cli.Status.ContentVersion
	default:
		return errors.Errorf("unexpected content library item type %T", cli)
	}

	ovfEnvelope, err := vs.getOvfEnvelope(ctx, itemID, contentVersion)
	if err != nil {
		return err
	}

	logger := log.V(4).WithValues("vmiName", vmi.GetName(), "cliName", cli.GetName())
	if ovfEnvelope == nil {
		logger.Error(nil, "skip syncing VMI as corresponding OVF envelope is nil")
		return nil
	}

	contentlibrary.UpdateVmiWithOvfEnvelope(vmi, *ovfEnvelope)
	return nil
}

// getOvfEnvelope gets the OVF envelope from the cache if it exists and matches version.
// If not, it downloads the OVF envelope from vCenter and stores it in the cache.
func (vs *vSphereVMProvider) getOvfEnvelope(
	ctx goctx.Context, itemID, contentVersion string) (*ovf.Envelope, error) {
	logger := log.V(4).WithValues("itemID", itemID, "contentVersion", contentVersion)

	// Lock the current item to prevent concurrent downloads of the same OVF.
	// This is done before the get from cache below to prevent stale result.
	curItemLock := vs.ovfCacheLockPool.Get(itemID)
	curItemLock.Lock()
	defer curItemLock.Unlock()

	isHitFunc := func(cacheItem VersionedOVFEnvelope) bool {
		return cacheItem.ContentVersion == contentVersion
	}
	cacheItem, found := vs.ovfCache.Get(itemID, isHitFunc)
	if found {
		logger.Info("Cache item hit, using cached OVF")
	} else {
		logger.Info("Cache item miss, downloading OVF from vCenter")
		client, err := vs.getVcClient(ctx)
		if err != nil {
			return nil, err
		}

		ovfEnvelope, err := client.ContentLibClient().RetrieveOvfEnvelopeByLibraryItemID(ctx, itemID)
		if err != nil || ovfEnvelope == nil {
			return nil, err
		}

		cacheItem = VersionedOVFEnvelope{
			ContentVersion: contentVersion,
			OvfEnvelope:    ovfEnvelope,
		}
		putResult := vs.ovfCache.Put(itemID, cacheItem)
		logger.Info("Cache item put", "itemID", itemID, "putResult", putResult)
	}

	return cacheItem.OvfEnvelope, nil
}

// GetItemFromLibraryByName get the library item from specified content library by its name.
// Do not return error if the item doesn't exist in the content library.
func (vs *vSphereVMProvider) GetItemFromLibraryByName(ctx goctx.Context,
	contentLibrary, itemName string) (*library.Item, error) {
	log.V(4).Info("Get item from ContentLibrary",
		"UUID", contentLibrary, "item name", itemName)

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}

	return client.ContentLibClient().GetLibraryItem(ctx, contentLibrary, itemName, false)
}

func (vs *vSphereVMProvider) UpdateContentLibraryItem(ctx goctx.Context, itemID, newName string, newDescription *string) error {
	log.V(4).Info("Update Content Library Item", "itemID", itemID)

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return err
	}

	return client.ContentLibClient().UpdateLibraryItem(ctx, itemID, newName, newDescription)
}

func (vs *vSphereVMProvider) getOpID(vm *vmopv1.VirtualMachine, operation string) string {
	const charset = "0123456789abcdef"

	// TODO: Is this actually useful?
	id := make([]byte, 8)
	for i := range id {
		idx := rand.Intn(len(charset)) //nolint:gosec
		id[i] = charset[idx]
	}

	return strings.Join([]string{"vmoperator", vm.Name, operation, string(id)}, "-")
}

func (vs *vSphereVMProvider) getVM(
	vmCtx context.VirtualMachineContext,
	client *vcclient.Client,
	notFoundReturnErr bool) (*object.VirtualMachine, error) {

	vcVM, err := vcenter.GetVirtualMachine(vmCtx, vs.k8sClient, client.VimClient(), client.Datacenter(), client.Finder())
	if err != nil {
		return nil, err
	}

	if vcVM == nil && notFoundReturnErr {
		return nil, fmt.Errorf("VirtualMachine %q was not found on VC", vmCtx.VM.Name)
	}

	return vcVM, nil
}

func (vs *vSphereVMProvider) getOrComputeCPUMinFrequency(ctx goctx.Context) (uint64, error) {
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

func (vs *vSphereVMProvider) ComputeCPUMinFrequency(ctx goctx.Context) error {
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

func (vs *vSphereVMProvider) computeCPUMinFrequency(ctx goctx.Context) (uint64, error) {
	// Get all the availability zones in order to calculate the minimum
	// CPU frequencies for each of the zones' vSphere clusters.
	availabilityZones, err := topology.GetAvailabilityZones(ctx, vs.k8sClient)
	if err != nil {
		return 0, err
	}

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return 0, err
	}

	if !lib.IsWcpFaultDomainsFSSEnabled() {
		ccr, err := vcenter.GetResourcePoolOwnerMoRef(ctx, client.VimClient(), client.Config().ResourcePool)
		if err != nil {
			return 0, err
		}

		// Only expect 1 AZ in this case.
		for i := range availabilityZones {
			availabilityZones[i].Spec.ClusterComputeResourceMoIDs = []string{ccr.Value}
		}
	}

	var errs []error

	var minFreq uint64
	for _, az := range availabilityZones {
		moIDs := az.Spec.ClusterComputeResourceMoIDs
		if len(moIDs) == 0 {
			moIDs = []string{az.Spec.ClusterComputeResourceMoId} // HA TEMP
		}

		for _, moID := range moIDs {
			ccr := object.NewClusterComputeResource(client.VimClient(),
				types.ManagedObjectReference{Type: "ClusterComputeResource", Value: moID})

			freq, err := vcenter.ClusterMinCPUFreq(ctx, ccr)
			if err != nil {
				errs = append(errs, err)
			} else if minFreq == 0 || freq < minFreq {
				minFreq = freq
			}
		}
	}

	return minFreq, k8serrors.NewAggregate(errs)
}

// ResVMToVirtualMachineImage isn't currently used.
func ResVMToVirtualMachineImage(ctx goctx.Context, vm *object.VirtualMachine) (*vmopv1.VirtualMachineImage, error) {
	var o mo.VirtualMachine
	err := vm.Properties(ctx, vm.Reference(), []string{"summary", "config.createDate", "config.vAppConfig"}, &o)
	if err != nil {
		return nil, err
	}

	var createTimestamp metav1.Time
	ovfProps := make(map[string]string)

	if o.Config != nil {
		if o.Config.CreateDate != nil {
			createTimestamp = metav1.NewTime(*o.Config.CreateDate)
		}

		if o.Config.VAppConfig != nil {
			if vAppConfig := o.Config.VAppConfig.GetVmConfigInfo(); vAppConfig != nil {
				for _, prop := range vAppConfig.Property {
					if strings.HasPrefix(prop.Id, "vmware-system") {
						if prop.Value != "" {
							ovfProps[prop.Id] = prop.Value
						} else {
							ovfProps[prop.Id] = prop.DefaultValue
						}
					}
				}
			}
		}
	}

	return &vmopv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:              o.Summary.Config.Name,
			Annotations:       ovfProps,
			CreationTimestamp: createTimestamp,
		},
		Spec: vmopv1.VirtualMachineImageSpec{
			Type:            "VM",
			ImageSourceType: "Inventory",
		},
		Status: vmopv1.VirtualMachineImageStatus{
			Uuid:       o.Summary.Config.Uuid,
			InternalId: vm.Reference().Value,
			PowerState: string(o.Summary.Runtime.PowerState),
		},
	}, nil
}

func (vs *vSphereVMProvider) GetTasksByActID(ctx goctx.Context, actID string) (tasksInfo []types.TaskInfo, retErr error) {
	vcClient, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}

	taskManager := task.NewManager(vcClient.VimClient())
	filterSpec := types.TaskFilterSpec{
		ActivationId: []string{actID},
	}

	collector, err := taskManager.CreateCollectorForTasks(ctx, filterSpec)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create collector for tasks")
	}
	defer func() {
		err = collector.Destroy(ctx)
		if retErr == nil {
			retErr = err
		}
	}()

	taskList := make([]types.TaskInfo, 0)
	for {
		nextTasks, err := collector.ReadNextTasks(ctx, taskHistoryCollectorPageSize)
		if err != nil {
			log.Error(err, "failed to read next tasks")
			return nil, err
		}
		if len(nextTasks) == 0 {
			break
		}
		taskList = append(taskList, nextTasks...)
	}

	log.V(5).Info("found tasks", "actID", actID, "tasks", taskList)
	return taskList, nil
}
