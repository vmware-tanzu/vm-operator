// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vapi/library"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/client"
	vcconfig "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	vsclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
)

const (
	VsphereVMProviderName = "vsphere"

	// taskHistoryCollectorPageSize represents the max count to read from task manager in one iteration.
	taskHistoryCollectorPageSize = 10
)

var log = logf.Log.WithName(VsphereVMProviderName)

type vSphereVMProvider struct {
	k8sClient         ctrlclient.Client
	eventRecorder     record.Recorder
	globalExtraConfig map[string]string
	minCPUFreq        uint64

	vcClientLock sync.Mutex
	vcClient     *vcclient.Client
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

func (vs *vSphereVMProvider) getVcClient(ctx context.Context) (*vcclient.Client, error) {
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

func (vs *vSphereVMProvider) UpdateVcPNID(ctx context.Context, vcPNID, vcPort string) error {
	updated, err := vcconfig.UpdateVcInConfigMap(ctx, vs.k8sClient, vcPNID, vcPort)
	if err != nil || !updated {
		return err
	}

	// Our controller-runtime client does not cache ConfigMaps & Secrets, so the next time
	// getVcClient() is called, it will fetch newly updated CM.
	vs.clearAndLogoutVcClient(ctx)
	return nil
}

func (vs *vSphereVMProvider) ResetVcClient(ctx context.Context) {
	vs.clearAndLogoutVcClient(ctx)
}

func (vs *vSphereVMProvider) clearAndLogoutVcClient(ctx context.Context) {
	vs.vcClientLock.Lock()
	vcClient := vs.vcClient
	vs.vcClient = nil
	vs.vcClientLock.Unlock()

	if vcClient != nil {
		vcClient.Logout(ctx)
	}
}

// SyncVirtualMachineImage syncs the vmi object with the OVF Envelope retrieved from the cli object.
func (vs *vSphereVMProvider) SyncVirtualMachineImage(ctx context.Context, cli, vmi ctrlclient.Object) error {
	var itemID, contentVersion string
	var itemType imgregv1a1.ContentLibraryItemType

	switch cli := cli.(type) {
	case *imgregv1a1.ContentLibraryItem:
		itemID = string(cli.Spec.UUID)
		contentVersion = cli.Status.ContentVersion
		itemType = cli.Status.Type
	case *imgregv1a1.ClusterContentLibraryItem:
		itemID = string(cli.Spec.UUID)
		contentVersion = cli.Status.ContentVersion
		itemType = cli.Status.Type
	default:
		return fmt.Errorf("unexpected content library item K8s object type %T", cli)
	}

	logger := log.V(4).WithValues("vmiName", vmi.GetName(), "cliName", cli.GetName())

	// Exit early if the library item type is not an OVF.
	if itemType != imgregv1a1.ContentLibraryItemTypeOvf {
		logger.Info("Skip syncing VMI content as the library item is not OVF",
			"libraryItemType", itemType)
		return nil
	}

	ovfEnvelope, err := ovfcache.GetOVFEnvelope(ctx, itemID, contentVersion)
	if err != nil {
		return fmt.Errorf("failed to get OVF envelope for library item %q: %w", itemID, err)
	}

	if ovfEnvelope == nil {
		return fmt.Errorf("OVF envelope is nil for library item %q", itemID)
	}

	contentlibrary.UpdateVmiWithOvfEnvelope(vmi, *ovfEnvelope)
	return nil
}

func (vs *vSphereVMProvider) getOvfEnvelope(
	ctx context.Context, itemID string) (*ovf.Envelope, error) {

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}

	p := contentlibrary.NewProvider(ctx, client.RestClient())
	return p.RetrieveOvfEnvelopeByLibraryItemID(ctx, itemID)
}

// GetItemFromLibraryByName get the library item from specified content library by its name.
// Do not return error if the item doesn't exist in the content library.
func (vs *vSphereVMProvider) GetItemFromLibraryByName(ctx context.Context,
	contentLibrary, itemName string) (*library.Item, error) {
	log.V(4).Info("Get item from ContentLibrary",
		"UUID", contentLibrary, "item name", itemName)

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}

	contentLibraryProvider := contentlibrary.NewProvider(ctx, client.RestClient())
	return contentLibraryProvider.GetLibraryItem(ctx, contentLibrary, itemName, false)
}

func (vs *vSphereVMProvider) UpdateContentLibraryItem(ctx context.Context, itemID, newName string, newDescription *string) error {
	log.V(4).Info("Update Content Library Item", "itemID", itemID)

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return err
	}

	contentLibraryProvider := contentlibrary.NewProvider(ctx, client.RestClient())
	return contentLibraryProvider.UpdateLibraryItem(ctx, itemID, newName, newDescription)
}

func (vs *vSphereVMProvider) getOpID(vm *vmopv1.VirtualMachine, operation string) string {
	const charset = "0123456789abcdef"

	id := make([]byte, 8)
	for i := range id {
		idx := rand.Intn(len(charset)) //nolint:gosec
		id[i] = charset[idx]
	}

	return strings.Join([]string{"vmoperator", vm.Name, operation, string(id)}, "-")
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

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return 0, err
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

func (vs *vSphereVMProvider) GetTasksByActID(ctx context.Context, actID string) (_ []vimtypes.TaskInfo, retErr error) {
	vcClient, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}

	taskManager := task.NewManager(vcClient.VimClient())
	filterSpec := vimtypes.TaskFilterSpec{
		ActivationId: []string{actID},
	}

	collector, err := taskManager.CreateCollectorForTasks(ctx, filterSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to create collector for tasks: %w", err)
	}
	defer func() {
		err = collector.Destroy(ctx)
		if retErr == nil {
			retErr = err
		}
	}()

	taskList := make([]vimtypes.TaskInfo, 0)
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

func (vs *vSphereVMProvider) DoesProfileSupportEncryption(
	ctx context.Context,
	profileID string) (bool, error) {

	c, err := vs.getVcClient(ctx)
	if err != nil {
		return false, err
	}

	return c.PbmClient().SupportsEncryption(ctx, profileID)
}

func (vs *vSphereVMProvider) VSphereClient(
	ctx context.Context) (*vsclient.Client, error) {

	c, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}
	return c.Client, nil
}
