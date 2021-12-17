// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	goctx "context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/pool"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

var log = logf.Log.WithName("vsphere").WithName("session")

var DefaultExtraConfig = map[string]string{
	constants.EnableDiskUUIDExtraConfigKey:       constants.ExtraConfigTrue,
	constants.GOSCIgnoreToolsCheckExtraConfigKey: constants.ExtraConfigTrue,
}

type Session struct {
	Client    *client.Client
	k8sClient ctrlruntime.Client

	Finder       *find.Finder
	datacenter   *object.Datacenter
	cluster      *object.ClusterComputeResource
	folder       *object.Folder
	resourcePool *object.ResourcePool
	network      object.NetworkReference // BMV: Dead? (never set in ConfigMap)
	datastore    *object.Datastore

	networkProvider network.Provider

	extraConfig           map[string]string
	storageClassRequired  bool
	useInventoryForImages bool
	tagInfo               map[string]string

	mutex              sync.Mutex
	cpuMinMHzInCluster uint64 // CPU Min Frequency across all Hosts in the cluster
}

func NewSessionAndConfigure(
	ctx goctx.Context,
	client *client.Client,
	config *config.VSphereVMProviderConfig,
	k8sClient ctrlruntime.Client) (*Session, error) {

	if log.V(4).Enabled() {
		configCopy := *config
		configCopy.VcCreds = nil
		log.V(4).Info("Creating new Session", "config", &configCopy)
	}

	s := &Session{
		Client:                client,
		k8sClient:             k8sClient,
		storageClassRequired:  config.StorageClassRequired,
		useInventoryForImages: config.UseInventoryAsContentSource,
	}

	if err := s.initSession(ctx, config); err != nil {
		return nil, err
	}

	log.V(4).Info("New session created and configured", "session", s.String())
	return s, nil
}

func (s *Session) Cluster() *object.ClusterComputeResource {
	return s.cluster
}

func (s *Session) initSession(
	ctx goctx.Context,
	cfg *config.VSphereVMProviderConfig) error {

	s.Finder = find.NewFinder(s.Client.VimClient(), false)

	ref := types.ManagedObjectReference{Type: "Datacenter", Value: cfg.Datacenter}
	o, err := s.Finder.ObjectReference(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "failed to init Datacenter %q", cfg.Datacenter)
	}
	s.datacenter = o.(*object.Datacenter)
	s.Finder.SetDatacenter(s.datacenter)

	if cfg.Cluster != "" {
		s.cluster, err = s.GetClusterByMoID(ctx, cfg.Cluster)
		if err != nil {
			return errors.Wrapf(err, "failed to init Cluster %q", cfg.Cluster)
		}
	}

	// On WCP, the RP is extracted from an annotation on the namespace.
	if cfg.ResourcePool != "" {
		s.resourcePool, err = s.GetResourcePoolByMoID(ctx, cfg.ResourcePool)
		if err != nil {
			return errors.Wrapf(err, "failed to init Resource Pool %q", cfg.ResourcePool)
		}

		// TODO: Remove this and fetch from config.Cluster once we populate the value from wcpsvc
		if s.cluster == nil {
			s.cluster, err = pool.GetResourcePoolOwner(ctx, s.resourcePool)
			if err != nil {
				return errors.Wrapf(err, "failed to init cluster from Resource Pool %q", cfg.ResourcePool)
			}
		}
	}

	// Initialize fields that require a cluster.
	if s.cluster != nil {
		minFreq, err := s.computeCPUInfo(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to init minimum CPU frequency")
		}
		s.SetCPUMinMHzInCluster(minFreq)
	}

	// On WCP, the Folder is extracted from an annotation on the namespace.
	if cfg.Folder != "" {
		s.folder, err = s.GetFolderByMoID(ctx, cfg.Folder)
		if err != nil {
			return errors.Wrapf(err, "failed to init folder %q", cfg.Folder)
		}
	}

	// Network setting is optional. This is only supported for test env, if that.
	if cfg.Network != "" {
		s.network, err = s.Finder.Network(ctx, cfg.Network)
		if err != nil {
			return errors.Wrapf(err, "failed to init Network %q", cfg.Network)
		}
	}

	if s.storageClassRequired {
		if cfg.Datastore != "" {
			log.V(4).Info("Ignoring configured datastore since storage class is required")
		}
	} else {
		if cfg.Datastore != "" {
			s.datastore, err = s.Finder.Datastore(ctx, cfg.Datastore)
			if err != nil {
				return errors.Wrapf(err, "failed to init Datastore %q", cfg.Datastore)
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

	s.networkProvider = network.NewProvider(s.k8sClient, s.Client.VimClient(), s.Finder, s.cluster)

	// Initialize tagging information
	s.tagInfo = make(map[string]string)
	s.tagInfo[config.CtrlVMVMAntiAffinityTagKey] = cfg.CtrlVMVMAntiAffinityTag
	s.tagInfo[config.WorkerVMVMAntiAffinityTagKey] = cfg.WorkerVMVMAntiAffinityTag
	s.tagInfo[config.ProviderTagCategoryNameKey] = cfg.TagCategoryName

	return nil
}

// findChildEntity finds a child entity by a given name under a parent object.
func (s *Session) findChildEntity(ctx goctx.Context, parent object.Reference, childName string) (object.Reference, error) {
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
func (s *Session) ChildResourcePool(ctx goctx.Context, resourcePoolName string) (*object.ResourcePool, error) {
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
func (s *Session) ChildFolder(ctx goctx.Context, folderName string) (*object.Folder, error) {
	folder, err := s.findChildEntity(ctx, s.folder, folderName)
	if err != nil {
		return nil, err
	}

	f, ok := folder.(*object.Folder)
	if !ok {
		return nil, fmt.Errorf("folder %q is not expected Folder type but a %T", folderName, folder)
	}
	return f, nil
}

// DoesResourcePoolExist checks if a ResourcePool with the given name exists.
func (s *Session) DoesResourcePoolExist(ctx goctx.Context, resourcePoolName string) (bool, error) {
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
func (s *Session) CreateResourcePool(ctx goctx.Context, rpSpec *v1alpha1.ResourcePoolSpec) (string, error) {
	log.Info("Creating ResourcePool with session", "name", rpSpec.Name)

	// CreateResourcePool is invoked during a ResourcePolicy reconciliation to create a ResourcePool for a set of
	// VirtualMachines. This RP is created as a child of RP of the session's RP.
	resourcePool, err := s.resourcePool.Create(ctx, rpSpec.Name, types.DefaultResourceConfigSpec())
	if err != nil {
		return "", err
	}

	log.Info("Created ResourcePool", "name", resourcePool.Name(), "path", resourcePool.InventoryPath)

	return resourcePool.Reference().Value, nil
}

// UpdateResourcePool updates a ResourcePool with the given spec.
func (s *Session) UpdateResourcePool(ctx goctx.Context, rpSpec *v1alpha1.ResourcePoolSpec) error {
	// Nothing to do if no reservation and limits set.
	hasReservations := !rpSpec.Reservations.Cpu.IsZero() || !rpSpec.Reservations.Memory.IsZero()
	hasLimits := !rpSpec.Limits.Cpu.IsZero() || !rpSpec.Limits.Memory.IsZero()
	if !hasReservations || !hasLimits {
		return nil
	}

	log.V(4).Info("Updating the ResourcePool", "name", rpSpec.Name)
	// TODO 
	return nil
}

func (s *Session) DeleteResourcePool(ctx goctx.Context, resourcePoolName string) error {
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
func (s *Session) DoesFolderExist(ctx goctx.Context, folderName string) (bool, error) {
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
func (s *Session) CreateFolder(ctx goctx.Context, folderSpec *v1alpha1.FolderSpec) (string, error) {
	log.Info("Creating a new Folder", "name", folderSpec.Name)

	// CreateFolder is invoked during a ResourcePolicy reconciliation to create a Folder for a set of VirtualMachines.
	// The new Folder is created as a child of the Folder corresponding to the session.
	folder, err := s.folder.CreateFolder(ctx, folderSpec.Name)
	if err != nil {
		return "", err
	}

	log.Info("Created Folder", "name", folder.Name(), "path", folder.InventoryPath)

	return folder.Reference().Value, nil
}

// DeleteFolder deletes the folder under the parent Folder (session.folder).
func (s *Session) DeleteFolder(ctx goctx.Context, folderName string) error {
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
func (s *Session) getResourcePoolAndFolder(vmCtx context.VirtualMachineContext,
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

func (s *Session) lookupVMByName(ctx goctx.Context, name string) (*res.VirtualMachine, error) {
	vm, err := s.Finder.VirtualMachine(ctx, name)
	if err != nil {
		return nil, err
	}
	return res.NewVMFromObject(vm)
}

func (s *Session) GetVirtualMachine(vmCtx context.VirtualMachineContext) (*res.VirtualMachine, error) {
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
		// path will be stale: 
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

func (s *Session) lookupVMByMoID(ctx goctx.Context, moID string) (*res.VirtualMachine, error) {
	ref, err := s.Finder.ObjectReference(ctx, types.ManagedObjectReference{Type: "VirtualMachine", Value: moID})
	if err != nil {
		return nil, err
	}

	vm := ref.(*object.VirtualMachine)
	log.V(4).Info("Found VM", "name", vm.Name(), "path", vm.InventoryPath, "moRef", vm.Reference())

	return res.NewVMFromObject(vm)
}

func (s *Session) invokeFsrVirtualMachine(vmCtx context.VirtualMachineContext, resVM *res.VirtualMachine) error {
	vmCtx.Logger.Info("Invoking FSR on VM")

	task, err := internal.VirtualMachineFSR(vmCtx, resVM.MoRef(), s.Client.VimClient())
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

// GetClusterByMoID returns resource pool for a given a moref.
func (s *Session) GetClusterByMoID(ctx goctx.Context, moID string) (*object.ClusterComputeResource, error) {
	ref := types.ManagedObjectReference{Type: "ClusterComputeResource", Value: moID}
	o, err := s.Finder.ObjectReference(ctx, ref)
	if err != nil {
		return nil, err
	}
	return o.(*object.ClusterComputeResource), nil
}

// GetResourcePoolByMoID returns resource pool for a given a moref.
func (s *Session) GetResourcePoolByMoID(ctx goctx.Context, moID string) (*object.ResourcePool, error) {
	ref := types.ManagedObjectReference{Type: "ResourcePool", Value: moID}
	o, err := s.Finder.ObjectReference(ctx, ref)
	if err != nil {
		return nil, err
	}
	return o.(*object.ResourcePool), nil
}

// GetFolderByMoID returns a folder for a given moref.
func (s *Session) GetFolderByMoID(ctx goctx.Context, moID string) (*object.Folder, error) {
	ref := types.ManagedObjectReference{Type: "Folder", Value: moID}
	o, err := s.Finder.ObjectReference(ctx, ref)
	if err != nil {
		return nil, err
	}
	return o.(*object.Folder), nil
}

func (s *Session) GetCPUMinMHzInCluster() uint64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.cpuMinMHzInCluster
}

func (s *Session) SetCPUMinMHzInCluster(minFreq uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.cpuMinMHzInCluster != minFreq {
		prevFreq := s.cpuMinMHzInCluster
		s.cpuMinMHzInCluster = minFreq
		log.V(4).Info("Successfully set CPU min frequency", "prevFreq", prevFreq, "newFreq", minFreq)
	}
}

func (s *Session) computeCPUInfo(ctx goctx.Context) (uint64, error) {
	return ComputeCPUInfo(ctx, s.cluster)
}

// ComputeCPUInfo computes the minimum frequency across all the hosts in the cluster. This is needed to convert the CPU
// requirements specified in cores to MHz. vSphere core is assumed to be equivalent to the value of min frequency.
// This function is adapted from wcp schedext.
func ComputeCPUInfo(ctx goctx.Context, cluster *object.ClusterComputeResource) (uint64, error) {
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
		hostCPUMHz := uint64(h.Summary.Hardware.CpuMhz)
		if hostCPUMHz < minFreq || minFreq == 0 {
			minFreq = hostCPUMHz
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
	sb.WriteString(fmt.Sprintf("cpuMinMHzInCluster: %v, ", s.GetCPUMinMHzInCluster()))
	sb.WriteString(fmt.Sprintf("tagInfo: %v ", s.tagInfo))
	sb.WriteString("}")
	return sb.String()
}

// AttachTagToVM attaches a tag with a given name to the vm.
func (s *Session) AttachTagToVM(ctx goctx.Context, tagName string, tagCatName string, vmRef mo.Reference) error {
	restClient := s.Client.RestClient()
	manager := tags.NewManager(restClient)
	tag, err := manager.GetTagForCategory(ctx, tagName, tagCatName)
	if err != nil {
		return err
	}

	return manager.AttachTag(ctx, tag.ID, vmRef)
}

// DetachTagFromVM detaches a tag with a given name from the vm.
func (s *Session) DetachTagFromVM(ctx goctx.Context, tagName string, tagCatName string, vmRef mo.Reference) error {
	restClient := s.Client.RestClient()
	manager := tags.NewManager(restClient)
	tag, err := manager.GetTagForCategory(ctx, tagName, tagCatName)
	if err != nil {
		return err
	}

	return manager.DetachTag(ctx, tag.ID, vmRef)
}

// RenameSessionCluster renames the cluster corresponding to this session. Used only in integration tests for now.
func (s *Session) RenameSessionCluster(ctx goctx.Context, name string) error {
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
