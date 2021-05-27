// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/cluster"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

var DefaultExtraConfig = map[string]string{
	EnableDiskUUIDExtraConfigKey:       ExtraConfigTrue,
	GOSCIgnoreToolsCheckExtraConfigKey: ExtraConfigTrue,
}

type Session struct {
	Client    *Client
	k8sClient ctrlruntime.Client
	scheme    *runtime.Scheme

	Finder       *find.Finder
	datacenter   *object.Datacenter
	cluster      *object.ClusterComputeResource
	folder       *object.Folder
	resourcePool *object.ResourcePool
	network      object.NetworkReference // BMV: Dead? (never set in ConfigMap)
	datastore    *object.Datastore

	networkProvider    NetworkProvider
	contentLibProvider ContentLibraryProvider

	extraConfig           map[string]string
	storageClassRequired  bool
	useInventoryForImages bool
	tagInfo               map[string]string

	mutex              sync.Mutex
	cpuMinMHzInCluster uint64 // CPU Min Frequency across all Hosts in the cluster
}

func NewSessionAndConfigure(ctx context.Context, client *Client, config *VSphereVmProviderConfig,
	k8sClient ctrlruntime.Client, scheme *runtime.Scheme) (*Session, error) {

	if log.V(4).Enabled() {
		configCopy := *config
		configCopy.VcCreds = nil
		log.V(4).Info("Creating new Session", "config", &configCopy)
	}

	s := &Session{
		Client:                client,
		k8sClient:             k8sClient,
		scheme:                scheme,
		storageClassRequired:  config.StorageClassRequired,
		useInventoryForImages: config.UseInventoryAsContentSource,
	}

	if err := s.initSession(ctx, config); err != nil {
		return nil, err
	}

	log.V(4).Info("New session created and configured", "session", s.String())
	return s, nil
}

//nolint:gocyclo
func (s *Session) initSession(ctx context.Context, config *VSphereVmProviderConfig) error {
	s.Finder = find.NewFinder(s.Client.VimClient(), false)

	ref := types.ManagedObjectReference{Type: "Datacenter", Value: config.Datacenter}
	o, err := s.Finder.ObjectReference(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "failed to init Datacenter %q", config.Datacenter)
	}
	s.datacenter = o.(*object.Datacenter)
	s.Finder.SetDatacenter(s.datacenter)

	if config.Cluster != "" {
		s.cluster, err = s.GetClusterByMoID(ctx, config.Cluster)
		if err != nil {
			return errors.Wrapf(err, "failed to init Cluster %q", config.Cluster)
		}
	}

	// On WCP, the RP is extracted from an annotation on the namespace.
	if config.ResourcePool != "" {
		s.resourcePool, err = s.GetResourcePoolByMoID(ctx, config.ResourcePool)
		if err != nil {
			return errors.Wrapf(err, "failed to init Resource Pool %q", config.ResourcePool)
		}

		// TODO: Remove this and fetch from config.Cluster once we populate the value from wcpsvc
		if s.cluster == nil {
			s.cluster, err = GetResourcePoolOwner(ctx, s.resourcePool)
			if err != nil {
				return errors.Wrapf(err, "failed to init cluster from Resource Pool %q", config.ResourcePool)
			}
		}
	}

	// Initialize fields that require a cluster.
	if s.cluster != nil {
		minFreq, err := s.computeCPUInfo(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to init minimum CPU frequency")
		}
		s.SetCpuMinMHzInCluster(minFreq)
	}

	// On WCP, the Folder is extracted from an annotation on the namespace.
	if config.Folder != "" {
		s.folder, err = s.GetFolderByMoID(ctx, config.Folder)
		if err != nil {
			return errors.Wrapf(err, "failed to init folder %q", config.Folder)
		}
	}

	// Network setting is optional. This is only supported for test env, if that.
	if config.Network != "" {
		s.network, err = s.Finder.Network(ctx, config.Network)
		if err != nil {
			return errors.Wrapf(err, "failed to init Network %q", config.Network)
		}
	}

	if s.storageClassRequired {
		if config.Datastore != "" {
			log.V(4).Info("Ignoring configured datastore since storage class is required")
		}
	} else {
		if config.Datastore != "" {
			s.datastore, err = s.Finder.Datastore(ctx, config.Datastore)
			if err != nil {
				return errors.Wrapf(err, "failed to init Datastore %q", config.Datastore)
			}
		}
	}

	// Apply default extra config values. Allow for the option to specify extraConfig to be applied to all VMs
	s.extraConfig = DefaultExtraConfig
	if jsonExtraConfig := os.Getenv("JSON_EXTRA_CONFIG"); jsonExtraConfig != "" {
		extraConfig := make(map[string]string)
		if err := json.Unmarshal([]byte(jsonExtraConfig), &extraConfig); err != nil {
			return errors.Wrap(err, "Unable to parse value of 'JSON_EXTRA_CONFIG' environment variable")
		}
		log.V(4).Info("Using Json extraConfig", "extraConfig", extraConfig)
		// Over-write the default extra config values
		for k, v := range extraConfig {
			s.extraConfig[k] = v
		}
	}

	s.networkProvider = NewNetworkProvider(s.k8sClient, s.Client.VimClient(), s.Finder, s.cluster, s.scheme)
	s.contentLibProvider = NewContentLibraryProvider(s.Client.RestClient())

	// Initialize tagging information
	s.tagInfo = make(map[string]string)
	s.tagInfo[CtrlVmVmAntiAffinityTagKey] = config.CtrlVmVmAntiAffinityTag
	s.tagInfo[WorkerVmVmAntiAffinityTagKey] = config.WorkerVmVmAntiAffinityTag
	s.tagInfo[ProviderTagCategoryNameKey] = config.TagCategoryName

	return nil
}

func (s *Session) CreateLibrary(ctx context.Context, name, datastoreID string) (string, error) {
	return s.contentLibProvider.CreateLibrary(ctx, name, datastoreID)
}

func (s *Session) CreateLibraryItem(ctx context.Context, libraryItem library.Item, path string) error {
	return s.contentLibProvider.CreateLibraryItem(ctx, libraryItem, path)
}

func IsSupportedDeployType(t string) bool {
	switch t {
	case library.ItemTypeVMTX, library.ItemTypeOVF:
		// Keep in sync with what cloneVMFromContentLibrary() handles.
		return true
	default:
		return false
	}
}

// Lists all the VirtualMachineImages from a CL by a given UUID.
func (s *Session) ListVirtualMachineImagesFromCL(ctx context.Context, clUUID string,
	currentCLImages map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error) {

	log.V(4).Info("Listing VirtualMachineImages from ContentLibrary", "contentLibraryUUID", clUUID)

	items, err := s.contentLibProvider.GetLibraryItems(ctx, clUUID)
	if err != nil {
		return nil, err
	}

	var images []*v1alpha1.VirtualMachineImage
	for i := range items {
		var ovfEnvelope *ovf.Envelope
		item := items[i]

		if curImage, ok := currentCLImages[item.Name]; ok {
			// If there is already an VMImage for this item, and it is the same - as determined by _just_ the
			// annotation - reuse the existing VMImage. This is to avoid repeated CL fetch tasks that would
			// otherwise be created, spamming the UI. It would be nice if CL provided an external API that
			// allowed us to silently fetch the OVF.
			annotations := curImage.GetAnnotations()
			if ver := annotations[VMImageCLVersionAnnotation]; ver == libItemVersionAnnotation(&item) {
				images = append(images, &curImage)
				continue
			}
		}

		switch item.Type {
		case library.ItemTypeOVF:
			if ovfEnvelope, err = s.contentLibProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, &item); err != nil {
				return nil, err
			}
		case library.ItemTypeVMTX:
			// Do not try to populate VMTX types, but resVm.GetOvfProperties() should return an
			// OvfEnvelope.
		default:
			// Not a supported type. Keep this in sync with cloneVMFromContentLibrary().
			continue
		}

		images = append(images, LibItemToVirtualMachineImage(&item, ovfEnvelope))
	}

	return images, nil
}

// findChildEntity finds a child entity by a given name under a parent object
func (s *Session) findChildEntity(ctx context.Context, parent object.Reference, childName string) (object.Reference, error) {
	si := object.NewSearchIndex(s.Client.VimClient())
	ref, err := si.FindChild(ctx, parent, childName)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		// SearchIndex returns nil when child name is not found
		log.Error(fmt.Errorf("entity not found"), "child entity not found on vSphere", "name", childName)
		return nil, &find.NotFoundError{}
	}

	// We have found a child entity with the given name. Populate the inventory path before returning.
	child, err := s.Finder.ObjectReference(ctx, ref.Reference())
	if err != nil {
		log.Error(err, "error when setting inventory path for the object", "moRef", ref.Reference().Value)
		return nil, err
	}

	return child, nil
}

// ChildResourcePool returns a child resource pool by a given name under the session's parent
// resource pool, returns error if no child resource pool exists with a given name.
func (s *Session) ChildResourcePool(ctx context.Context, resourcePoolName string) (*object.ResourcePool, error) {
	resourcePool, err := s.findChildEntity(ctx, s.resourcePool, resourcePoolName)
	if err != nil {
		return nil, err
	}

	rp, ok := resourcePool.(*object.ResourcePool)
	if !ok {
		return nil, fmt.Errorf("ResourcePool %q is not expected ResourcePool type but a %T", resourcePoolName, resourcePool)
	}
	return rp, nil
}

// ChildFolder returns a child resource pool by a given name under the session's parent
// resource pool, returns error if no child resource pool exists with a given name.
func (s *Session) ChildFolder(ctx context.Context, folderName string) (*object.Folder, error) {
	folder, err := s.findChildEntity(ctx, s.folder, folderName)
	if err != nil {
		return nil, err
	}

	f, ok := folder.(*object.Folder)
	if !ok {
		return nil, fmt.Errorf("Folder %q is not expected Folder type but a %T", folderName, folder)
	}
	return f, nil
}

// DoesResourcePoolExist checks if a ResourcePool with the given name exists.
func (s *Session) DoesResourcePoolExist(ctx context.Context, resourcePoolName string) (bool, error) {
	log.V(4).Info("Checking if ResourcePool exists", "resourcePoolName", resourcePoolName)
	_, err := s.ChildResourcePool(ctx, resourcePoolName)
	if err != nil {
		switch err.(type) {
		case *find.NotFoundError:
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}

// CreateResourcePool creates a ResourcePool under the parent ResourcePool (session.resourcePool).
func (s *Session) CreateResourcePool(ctx context.Context, rpSpec *v1alpha1.ResourcePoolSpec) (string, error) {
	log.Info("Creating ResourcePool with session", "name", rpSpec.Name)

	// CreateResourcePool is invoked during a ResourcePolicy reconciliation to create a ResourcePool for a set of
	// VirtualMachines. The new RP is created under the RP corresponding to the session.
	// For a Supervisor Cluster deployment, the session's RP is the supervisor cluster namespace's RP.
	// For IAAS deployments, the session's RP correspond to RP in provider ConfigMap.
	resourcePool, err := s.resourcePool.Create(ctx, rpSpec.Name, types.DefaultResourceConfigSpec())
	if err != nil {
		return "", err
	}

	log.Info("Created ResourcePool", "name", resourcePool.Name(), "path", resourcePool.InventoryPath)

	return resourcePool.Reference().Value, nil
}

// UpdateResourcePool updates a ResourcePool with the given spec.
func (s *Session) UpdateResourcePool(ctx context.Context, rpSpec *v1alpha1.ResourcePoolSpec) error {
	// Nothing to do if no reservation and limits set.
	hasReservations := !rpSpec.Reservations.Cpu.IsZero() || !rpSpec.Reservations.Memory.IsZero()
	hasLimits := !rpSpec.Limits.Cpu.IsZero() || !rpSpec.Limits.Memory.IsZero()
	if !hasReservations || !hasLimits {
		return nil
	}

	log.V(4).Info("Updating the ResourcePool", "name", rpSpec.Name)
	// TODO https://jira.eng.vmware.com/browse/GCM-1607

	return nil
}

func (s *Session) DeleteResourcePool(ctx context.Context, resourcePoolName string) error {
	log.Info("Deleting the ResourcePool", "name", resourcePoolName)

	resourcePool, err := s.ChildResourcePool(ctx, resourcePoolName)
	if err != nil {
		switch err.(type) {
		case *find.NotFoundError, *find.DefaultNotFoundError:
			return nil
		default:
			log.Error(err, "Error getting the ResourcePool to be deleted", "name", resourcePoolName)
			return err
		}
	}

	task, err := resourcePool.Destroy(ctx)
	if err != nil {
		log.Error(err, "Failed to invoke destroy for ResourcePool", "name", resourcePoolName)
		return err
	}

	if taskResult, err := task.WaitForResult(ctx, nil); err != nil {
		msg := ""
		if taskResult != nil && taskResult.Error != nil {
			msg = taskResult.Error.LocalizedMessage
		}
		log.Error(err, "Error in deleting ResourcePool", "name", resourcePoolName, "msg", msg)
		return err
	}

	return nil
}

// DoesFolderExist checks if a Folder with the given name exists.
func (s *Session) DoesFolderExist(ctx context.Context, folderName string) (bool, error) {
	_, err := s.ChildFolder(ctx, folderName)
	if err != nil {
		switch err.(type) {
		case *find.NotFoundError:
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}

// CreateFolder creates a folder under the parent Folder (session.folder).
func (s *Session) CreateFolder(ctx context.Context, folderSpec *v1alpha1.FolderSpec) (string, error) {
	log.Info("Creating a new Folder", "name", folderSpec.Name)

	// CreateFolder is invoked during a ResourcePolicy reconciliation to create a Folder for a set of VirtualMachines.
	// The new Folder is created under the Folder corresponding to the session.
	// For a Supervisor Cluster deployment, the session's Folder is the supervisor cluster namespace's Folder.
	// For IAAS deployments, the session's Folder corresponds to Folder in provider ConfigMap.
	folder, err := s.folder.CreateFolder(ctx, folderSpec.Name)
	if err != nil {
		return "", err
	}

	log.Info("Created Folder", "name", folder.Name(), "path", folder.InventoryPath)

	return folder.Reference().Value, nil
}

// DeleteFolder deletes the folder under the parent Folder (session.folder).
func (s *Session) DeleteFolder(ctx context.Context, folderName string) error {
	log.Info("Deleting the Folder", "name", folderName)

	folder, err := s.ChildFolder(ctx, folderName)
	if err != nil {
		switch err.(type) {
		case *find.NotFoundError, *find.DefaultNotFoundError:
			return nil
		default:
			log.Error(err, "Error finding the VM folder to delete", "name", folderName)
			return err
		}
	}

	task, err := folder.Destroy(ctx)
	if err != nil {
		return err
	}

	if taskResult, err := task.WaitForResult(ctx, nil); err != nil {
		msg := ""
		if taskResult != nil && taskResult.Error != nil {
			msg = taskResult.Error.LocalizedMessage
		}
		log.Error(err, "Error deleting folder", "name", folderName, "message", msg)
		return err
	}

	log.Info("Successfully deleted folder", "name", folderName)

	return nil
}

// getResourcePoolAndFolder gets the ResourcePool and Folder from the Resource Policy. If no policy
// is specified, the session's ResourcePool and Folder is returned instead.
func (s *Session) getResourcePoolAndFolder(vmCtx VMContext,
	resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (*object.ResourcePool, *object.Folder, error) {

	if resourcePolicy == nil {
		return s.resourcePool, s.folder, nil
	}

	resourcePoolName := resourcePolicy.Spec.ResourcePool.Name
	resourcePool, err := s.ChildResourcePool(vmCtx, resourcePoolName)
	if err != nil {
		vmCtx.Logger.Error(err, "Unable to find ResourcePool", "name", resourcePoolName)
		return nil, nil, err
	}

	vmCtx.Logger.V(4).Info("Found ResourcePool",
		"name", resourcePoolName, "moRef", resourcePool.Reference().Value)

	folderName := resourcePolicy.Spec.Folder.Name
	folder, err := s.ChildFolder(vmCtx, folderName)
	if err != nil {
		vmCtx.Logger.Error(err, "Unable to find Folder", "name", folderName)
		return nil, nil, err
	}

	vmCtx.Logger.V(4).Info("Found Folder",
		"name", folderName, "moRef", folder.Reference().Value)

	return resourcePool, folder, nil
}

func (s *Session) lookupVMByName(ctx context.Context, name string) (*res.VirtualMachine, error) {
	vm, err := s.Finder.VirtualMachine(ctx, name)
	if err != nil {
		return nil, err
	}
	return res.NewVMFromObject(vm)
}

func (s *Session) GetVirtualMachine(vmCtx VMContext) (*res.VirtualMachine, error) {
	if uniqueID := vmCtx.VM.Status.UniqueID; uniqueID != "" {
		vm, err := s.lookupVMByMoID(vmCtx, uniqueID)
		if err == nil {
			return vm, nil
		}
		vmCtx.Logger.V(4).Info("Failed to lookup VM by MoID, falling back to path",
			"moID", uniqueID, "error", err)
	}

	var folder *object.Folder

	if policyName := vmCtx.VM.Spec.ResourcePolicyName; policyName != "" {
		// Lookup the VM by name using the full inventory path to the VM. To do so, we need the
		// resource policy to get the VM's Folder.
		rp := &v1alpha1.VirtualMachineSetResourcePolicy{}
		rpKey := ctrlruntime.ObjectKey{Name: policyName, Namespace: vmCtx.VM.Namespace}
		if err := s.k8sClient.Get(vmCtx, rpKey, rp); err != nil {
			vmCtx.Logger.Error(err, "Failed to get resource policy", "name", rpKey)
			return nil, err
		}

		var err error
		folder, err = s.ChildFolder(vmCtx, rp.Spec.Folder.Name)
		if err != nil {
			vmCtx.Logger.Error(err, "Failed to find child Folder", "name", rp.Spec.Folder.Name, "rpName", rpKey)
			return nil, err
		}
	} else {
		// Developer enablement path: use the default folder for the session.
		// TODO: AKP: If any of the parent objects have been renamed, the cached inventory
		// path will be stale: https://jira.eng.vmware.com/browse/GCM-3124.
		folder = s.folder
	}

	path := folder.InventoryPath + "/" + vmCtx.VM.Name
	vm, err := s.Finder.VirtualMachine(vmCtx, path)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed lookup VM by path", "path", path)
		return nil, err
	}

	vmCtx.Logger.V(4).Info("Found VM via path", "vmRef", vm.Reference(), "path", path)
	return res.NewVMFromObject(vm)
}

func (s *Session) lookupVMByMoID(ctx context.Context, moId string) (*res.VirtualMachine, error) {
	ref, err := s.Finder.ObjectReference(ctx, types.ManagedObjectReference{Type: "VirtualMachine", Value: moId})
	if err != nil {
		return nil, err
	}

	vm := ref.(*object.VirtualMachine)
	log.V(4).Info("Found VM", "name", vm.Name(), "path", vm.InventoryPath, "moRef", vm.Reference())

	return res.NewVMFromObject(vm)
}

func (s *Session) invokeFsrVirtualMachine(vmCtx VMContext, resVM *res.VirtualMachine) error {
	vmCtx.Logger.Info("Invoking FSR on VM")

	task, err := VirtualMachineFSR(vmCtx, resVM.MoRef(), s.Client.vimClient)
	if err != nil {
		vmCtx.Logger.Error(err, "InvokeFSR call failed")
		return err
	}

	if err = task.Wait(vmCtx); err != nil {
		vmCtx.Logger.Error(err, "InvokeFSR task failed")
		return err
	}

	return nil
}

// GetClusterByMoID returns resource pool for a given a moref
func (s *Session) GetClusterByMoID(ctx context.Context, moID string) (*object.ClusterComputeResource, error) {
	ref := types.ManagedObjectReference{Type: "ClusterComputeResource", Value: moID}
	o, err := s.Finder.ObjectReference(ctx, ref)
	if err != nil {
		return nil, err
	}
	return o.(*object.ClusterComputeResource), nil
}

// GetResourcePoolByMoID returns resource pool for a given a moref
func (s *Session) GetResourcePoolByMoID(ctx context.Context, moID string) (*object.ResourcePool, error) {
	ref := types.ManagedObjectReference{Type: "ResourcePool", Value: moID}
	o, err := s.Finder.ObjectReference(ctx, ref)
	if err != nil {
		return nil, err
	}
	return o.(*object.ResourcePool), nil
}

// GetFolderByMoID returns a folder for a given moref
func (s *Session) GetFolderByMoID(ctx context.Context, moID string) (*object.Folder, error) {
	ref := types.ManagedObjectReference{Type: "Folder", Value: moID}
	o, err := s.Finder.ObjectReference(ctx, ref)
	if err != nil {
		return nil, err
	}
	return o.(*object.Folder), nil
}

func (s *Session) GetCpuMinMHzInCluster() uint64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.cpuMinMHzInCluster
}

func (s *Session) SetCpuMinMHzInCluster(minFreq uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.cpuMinMHzInCluster != minFreq {
		prevFreq := s.cpuMinMHzInCluster
		s.cpuMinMHzInCluster = minFreq
		log.V(4).Info("Successfully set CPU min frequency", "prevFreq", prevFreq, "newFreq", minFreq)
	}
}

func (s *Session) computeCPUInfo(ctx context.Context) (uint64, error) {
	return ComputeCPUInfo(ctx, s.cluster)
}

// ComputeCPUInfo computes the minimum frequency across all the hosts in the cluster. This is needed to convert the CPU
// requirements specified in cores to MHz. vSphere core is assumed to be equivalent to the value of min frequency.
// This function is adapted from wcp schedext
func ComputeCPUInfo(ctx context.Context, cluster *object.ClusterComputeResource) (uint64, error) {
	var cr mo.ComputeResource
	var hosts []mo.HostSystem
	var minFreq uint64

	if cluster == nil {
		return 0, errors.New("Must have a valid cluster reference to compute the cpu info")
	}

	err := cluster.Properties(ctx, cluster.Reference(), nil, &cr)
	if err != nil {
		return 0, err
	}

	if len(cr.Host) == 0 {
		return 0, errors.New("No hosts found in the cluster")
	}

	pc := property.DefaultCollector(cluster.Client())
	err = pc.Retrieve(ctx, cr.Host, []string{"summary"}, &hosts)
	if err != nil {
		return 0, err
	}

	for _, h := range hosts {
		if h.Summary.Hardware == nil {
			continue
		}
		hostCpuMHz := uint64(h.Summary.Hardware.CpuMhz)
		if hostCpuMHz < minFreq || minFreq == 0 {
			minFreq = hostCpuMHz
		}
	}

	return minFreq, nil
}

func (s *Session) String() string {
	var sb strings.Builder
	sb.WriteString("{")
	if s.datacenter != nil {
		sb.WriteString(fmt.Sprintf("datacenter: %s, ", s.datacenter.Reference().Value))
	}
	if s.folder != nil {
		sb.WriteString(fmt.Sprintf("folder: %s, ", s.folder.Reference().Value))
	}
	if s.network != nil {
		sb.WriteString(fmt.Sprintf("network: %s, ", s.network.Reference().Value))
	}
	if s.resourcePool != nil {
		sb.WriteString(fmt.Sprintf("resourcePool: %s, ", s.resourcePool.Reference().Value))
	}
	if s.cluster != nil {
		sb.WriteString(fmt.Sprintf("cluster: %s, ", s.cluster.Reference().Value))
	}
	if s.datastore != nil {
		sb.WriteString(fmt.Sprintf("datastore: %s, ", s.datastore.Reference().Value))
	}
	sb.WriteString(fmt.Sprintf("cpuMinMHzInCluster: %v, ", s.GetCpuMinMHzInCluster()))
	sb.WriteString(fmt.Sprintf("tagInfo: %v ", s.tagInfo))
	sb.WriteString("}")
	return sb.String()
}

// CreateClusterModule creates a clusterModule in vc and returns its id.
func (s *Session) CreateClusterModule(ctx context.Context) (string, error) {
	log.Info("Creating clusterModule")

	restClient := s.Client.RestClient()
	moduleId, err := cluster.NewManager(restClient).CreateModule(ctx, s.cluster)
	if err != nil {
		return "", err
	}

	log.Info("Created clusterModule", "moduleId", moduleId)
	return moduleId, nil
}

// DeleteClusterModule deletes a clusterModule in vc.
func (s *Session) DeleteClusterModule(ctx context.Context, moduleId string) error {
	log.Info("Deleting clusterModule", "moduleId", moduleId)

	restClient := s.Client.RestClient()
	if err := cluster.NewManager(restClient).DeleteModule(ctx, moduleId); err != nil {
		return err
	}

	log.Info("Deleted clusterModule", "moduleId", moduleId)
	return nil
}

// DoesClusterModuleExist checks whether the module with the given spec/uuid exit in vc.
func (s *Session) DoesClusterModuleExist(ctx context.Context, moduleUuid string) (bool, error) {
	log.V(4).Info("Checking clusterModule", "moduleId", moduleUuid)

	if moduleUuid == "" {
		return false, nil
	}

	restClient := s.Client.RestClient()
	modules, err := cluster.NewManager(restClient).ListModules(ctx)
	if err != nil {
		return false, err
	}
	for _, mod := range modules {
		if mod.Module == moduleUuid {
			return true, nil
		}
	}

	log.V(4).Info("ClusterModule doesn't exist", "moduleId", moduleUuid)
	return false, nil
}

// AddVmToClusterModule associates a VM with a clusterModule.
func (s *Session) AddVmToClusterModule(ctx context.Context, moduleId string, vmRef mo.Reference) error {
	log.Info("Adding vm to clusterModule", "moduleId", moduleId, "vmId", vmRef)

	restClient := s.Client.RestClient()
	if _, err := cluster.NewManager(restClient).AddModuleMembers(ctx, moduleId, vmRef); err != nil {
		return err
	}

	log.Info("Added vm to clusterModule", "moduleId", moduleId, "vmId", vmRef)
	return nil
}

// RemoveVmFromClusterModule removes a VM from a clusterModule.
func (s *Session) RemoveVmFromClusterModule(ctx context.Context, moduleId string, vmRef mo.Reference) error {
	log.Info("Removing vm from clusterModule", "moduleId", moduleId, "vmId", vmRef)

	restClient := s.Client.RestClient()
	if _, err := cluster.NewManager(restClient).RemoveModuleMembers(ctx, moduleId, vmRef); err != nil {
		return err
	}

	log.Info("Removed vm from clusterModule", "moduleId", moduleId, "vmId", vmRef)
	return nil
}

// IsVmMemberOfClusterModule checks whether a given VM is a member of ClusterModule in VC.
func (s *Session) IsVmMemberOfClusterModule(ctx context.Context, moduleId string, vmRef mo.Reference) (bool, error) {
	restClient := s.Client.RestClient()
	moduleMembers, err := cluster.NewManager(restClient).ListModuleMembers(ctx, moduleId)
	if err != nil {
		return false, err
	}

	for _, member := range moduleMembers {
		if member.Value == vmRef.Reference().Value {
			return true, nil
		}
	}

	return false, nil
}

// AttachTagToVm attaches a tag with a given name to the vm.
func (s *Session) AttachTagToVm(ctx context.Context, tagName string, tagCatName string, vmRef mo.Reference) error {
	restClient := s.Client.RestClient()
	manager := tags.NewManager(restClient)
	tag, err := manager.GetTagForCategory(ctx, tagName, tagCatName)
	if err != nil {
		return err
	}

	return manager.AttachTag(ctx, tag.ID, vmRef)
}

// DetachTagFromVm detaches a tag with a given name from the vm.
func (s *Session) DetachTagFromVm(ctx context.Context, tagName string, tagCatName string, vmRef mo.Reference) error {
	restClient := s.Client.RestClient()
	manager := tags.NewManager(restClient)
	tag, err := manager.GetTagForCategory(ctx, tagName, tagCatName)
	if err != nil {
		return err
	}

	return manager.DetachTag(ctx, tag.ID, vmRef)
}

// RenameSessionCluster renames the cluster corresponding to this session. Used only in integration tests for now.
func (s *Session) RenameSessionCluster(ctx context.Context, name string) error {
	task, err := s.cluster.Rename(ctx, name)
	if err != nil {
		log.Error(err, "Failed to invoke rename for cluster", "clusterMoID", s.cluster.Reference().Value)
		return err
	}

	if taskResult, err := task.WaitForResult(ctx, nil); err != nil {
		msg := ""
		if taskResult != nil && taskResult.Error != nil {
			msg = taskResult.Error.LocalizedMessage
		}
		log.Error(err, "Error in renaming cluster", "clusterMoID", s.cluster.Reference().Value, "msg", msg)
		return err
	}

	return nil
}
