// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"text/template"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/pbm"
	pbmTypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vapi/vcenter"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"

	ncpcs "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/cluster"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

var DefaultExtraConfig = map[string]string{
	"disk.enableUUID": "TRUE",
}

type Session struct {
	Client    *Client
	ncpClient ncpcs.Interface
	k8sClient ctrlruntime.Client

	Finder       *find.Finder
	datacenter   *object.Datacenter
	cluster      *object.ClusterComputeResource
	folder       *object.Folder
	resourcepool *object.ResourcePool
	network      object.NetworkReference // BMV: Dead? (never set in ConfigMap)
	contentlib   *library.Library
	datastore    *object.Datastore

	userInfo              *url.Userinfo
	extraConfig           map[string]string
	storageClassRequired  bool
	useInventoryForImages bool
	tagInfo               map[string]string

	mutex              sync.Mutex
	cpuMinMHzInCluster uint64 // CPU Min Frequency across all Hosts in the cluster
}

func NewSessionAndConfigure(ctx context.Context, client *Client, config *VSphereVmProviderConfig,
	ncpClient ncpcs.Interface, k8sClient ctrlruntime.Client) (*Session, error) {

	s := &Session{
		Client:                client,
		ncpClient:             ncpClient,
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

func (s *Session) initSession(ctx context.Context, config *VSphereVmProviderConfig) error {

	s.Finder = find.NewFinder(s.Client.VimClient(), false)
	s.userInfo = url.UserPassword(config.VcCreds.Username, config.VcCreds.Password)

	ref := types.ManagedObjectReference{Type: "Datacenter", Value: config.Datacenter}
	o, err := s.Finder.ObjectReference(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "failed to init Datacenter %q", config.Datacenter)
	}
	s.datacenter = o.(*object.Datacenter)
	s.Finder.SetDatacenter(s.datacenter)

	// ResourcePool is only relevant for Development environments.  On WCP, the RP is extracted from an annotation
	// on the namespace.
	if config.ResourcePool != "" {
		s.resourcepool, err = s.GetResourcePoolByMoID(ctx, config.ResourcePool)
		if err != nil {
			return errors.Wrapf(err, "failed to init Resource Pool %q", config.ResourcePool)
		}

		s.cluster, err = GetResourcePoolOwner(ctx, s.resourcepool)
		if err != nil {
			return errors.Wrapf(err, "failed to init cluster %q", config.ResourcePool)
		}
	}

	// Folder is only relevant for Development environments.  On WCP, the folder is extracted from an annotation
	// on the namespace.
	if config.Folder != "" {
		s.folder, err = s.GetFolderByMoID(ctx, config.Folder)
		if err != nil {
			return errors.Wrapf(err, "failed to init folder %q", config.Folder)
		}
	}

	// Network setting is optional
	if config.Network != "" {
		s.network, err = s.Finder.Network(ctx, config.Network)
		if err != nil {
			return errors.Wrapf(err, "failed to init Network %q", config.Network)
		}
		log.V(4).Info("Using default network", "network", config.Network)
	}

	// Apply default extra config values
	s.extraConfig = DefaultExtraConfig
	// Allow for the option to specify extraConfig to be applied to all VMs
	if jsonExtraConfig := os.Getenv("JSON_EXTRA_CONFIG"); jsonExtraConfig != "" {
		extraConfig := make(map[string]string)
		if err := json.Unmarshal([]byte(jsonExtraConfig), &extraConfig); err != nil {
			return errors.Wrapf(err, "Unable to parse value of 'JSON_EXTRA_CONFIG' environment variable")
		}
		log.V(4).Info("Using Json extraConfig", "extraConfig", extraConfig)
		// Over-write the default extra config values
		for k, v := range extraConfig {
			s.extraConfig[k] = v
		}
	}

	// Initialize min frequency.  Only do so if the cluster reference is valid.  The resource pool and cluster info are
	// not available on all sessions.
	if s.cluster != nil {
		if err := s.initCpuMinFreq(ctx); err != nil {
			return errors.Wrapf(err, "Failed to init CPU min frequency")
		}
	}

	if err := s.ConfigureContent(ctx, config.ContentSource); err != nil {
		return err
	}

	// Initialize tagging information
	s.tagInfo = make(map[string]string)
	s.tagInfo[CtrlVmVmAntiAffinityTagKey] = config.CtrlVmVmAntiAffinityTag
	s.tagInfo[WorkerVmVmAntiAffinityTagKey] = config.WorkerVmVmAntiAffinityTag
	s.tagInfo[ProviderTagCategoryNameKey] = config.TagCategoryName

	return s.initDatastore(ctx, config.Datastore)
}

func (s *Session) ServiceContent(ctx context.Context) (vimTypes.AboutInfo, error) {
	return s.Client.VimClient().ServiceContent.About, nil
}

func (s *Session) initDatastore(ctx context.Context, datastore string) error {
	if s.storageClassRequired {
		if datastore != "" {
			log.V(4).Info("Ignoring configured datastore since storage class is required")
		}
	} else {
		if datastore != "" {
			var err error
			s.datastore, err = s.Finder.Datastore(ctx, datastore)
			if err != nil {
				return errors.Wrapf(err, "failed to init Datastore %q", datastore)
			}
			log.V(4).Info("Datastore init OK", "datastore", s.datastore.Reference().Value)
		}
	}
	return nil
}

func (s *Session) initCpuMinFreq(ctx context.Context) error {
	if s.GetCpuMinMHzInCluster() > 0 {
		return nil
	}

	minFreq, err := s.computeCPUInfo(ctx)
	if err != nil {
		return err
	}

	s.SetCpuMinMHzInCluster(minFreq)

	return nil
}

func (s *Session) ConfigureContent(ctx context.Context, contentSource string) error {
	log.V(4).Info("Configuring content library for session", "clUUID", contentSource)

	if contentSource == "" {
		log.V(4).Info("Content library configured to nothing")
		s.contentlib = nil
		return nil
	}

	restClient := s.Client.RestClient()
	lib, err := library.NewManager(restClient).GetLibraryByID(ctx, contentSource)
	if err != nil {
		return errors.Wrapf(err, "failed to init Content Library %q", contentSource)
	}

	s.contentlib = lib

	log.V(4).Info("Content library configured to", "contentSource", contentSource)
	return nil
}

// TODO: Follow up to expose this and other fields without "getters"
func (s *Session) Datastore() *object.Datastore {
	return s.datastore
}

func (s *Session) CreateLibrary(ctx context.Context, contentSource string) (string, error) {
	return NewContentLibraryProvider(s).CreateLibrary(ctx, contentSource)
}

func (s *Session) DeleteContentLibrary(ctx context.Context, libID string) error {
	restClient := s.Client.RestClient()

	libManager := library.NewManager(restClient)
	lib, err := libManager.GetLibraryByID(ctx, libID)
	if err != nil {
		return err
	}

	return libManager.DeleteLibrary(ctx, lib)
}

func (s *Session) CreateLibraryItem(ctx context.Context, libraryItem library.Item, path string) error {
	return NewContentLibraryProvider(s).CreateLibraryItem(ctx, libraryItem, path)
}

// Lists all the VirtualMachineImages from a CL by a given UUID.
func (s *Session) ListVirtualMachineImagesFromCL(ctx context.Context, clUUID string) ([]*v1alpha1.VirtualMachineImage, error) {
	log.V(4).Info("Listing VirtualMachineImages from ContentLibrary", "contentLibraryUUID", clUUID)

	restClient := s.Client.RestClient()
	items, err := library.NewManager(restClient).GetLibraryItems(ctx, clUUID)
	if err != nil {
		return nil, err
	}

	var images []*v1alpha1.VirtualMachineImage
	for i := range items {
		item := items[i]
		if IsSupportedDeployType(item.Type) {
			var ovfInfoRetriever OvfPropertyRetriever = vmOptions{}
			virtualMachineImage, err := LibItemToVirtualMachineImage(ctx, s, &item, AnnotateVmImage, ovfInfoRetriever)
			if err != nil {
				return nil, err
			}
			images = append(images, virtualMachineImage)
		}
	}

	return images, err
}

// DoesContentLibraryExist checks if a ContentLibrary by id "clID" exists on vSphere inventory.
func (s *Session) DoesContentLibraryExist(ctx context.Context, clID string) (bool, error) {
	restClient := s.Client.RestClient()
	libManager := library.NewManager(restClient)
	_, err := libManager.GetLibraryByID(ctx, clID)

	// govmomi vapi rest client doesn't expose it's error type so we cannot check and return no error for Not Found. Instead we rely
	// on the error message itself. This is not ideal since we are relying on the error message being sent by client which can change any time.
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (s *Session) GetItemIDFromCL(ctx context.Context, itemName string) (string, error) {
	restClient := s.Client.RestClient()
	itemIDs, err := library.NewManager(restClient).FindLibraryItems(ctx,
		library.FindItem{LibraryID: s.contentlib.ID, Name: itemName})
	if err != nil {
		return "", errors.Wrapf(err, "failed to find image %q", itemName)
	}

	if len(itemIDs) == 0 {
		return "", errors.Errorf("no library items named: %s", itemName)
	}

	if len(itemIDs) != 1 {
		return "", errors.Errorf("multiple library items named: %s", itemName)
	}

	return itemIDs[0], nil
}

func (s *Session) GetItemFromCL(ctx context.Context, itemName string) (*library.Item, error) {
	itemID, err := s.GetItemIDFromCL(ctx, itemName)
	if err != nil {
		return nil, err
	}

	restClient := s.Client.RestClient()
	item, err := library.NewManager(restClient).GetLibraryItem(ctx, itemID)
	if err != nil {
		return nil, err
	}

	if item == nil {
		return nil, errors.Errorf("item: %v is not found in CL", itemName)
	}

	if !IsSupportedDeployType(item.Type) {
		return nil, errors.Errorf("item: %v not a supported type: %s", item.Name, item.Type)
	}

	return item, nil
}

func (s *Session) GetVirtualMachineImageFromCL(ctx context.Context, name string) (*v1alpha1.VirtualMachineImage, error) {
	item, err := s.GetItemFromCL(ctx, name)
	if err != nil {
		return nil, err
	}

	var ovfInfoRetriever OvfPropertyRetriever = vmOptions{}

	virtualMachineImage, err := LibItemToVirtualMachineImage(ctx, s, item, AnnotateVmImage, ovfInfoRetriever)
	if err != nil {
		return nil, err
	}

	return virtualMachineImage, nil
}

func (s *Session) ListVirtualMachines(ctx context.Context, path string) ([]*res.VirtualMachine, error) {
	var vms []*res.VirtualMachine

	objVms, err := s.Finder.VirtualMachineList(ctx, path)
	if err != nil {
		switch err.(type) {
		case *find.NotFoundError, *find.DefaultNotFoundError:
			return vms, nil
		default:
			return nil, err
		}
	}

	for _, objVm := range objVms {
		if resVm, err := res.NewVMFromObject(objVm); err == nil {
			vms = append(vms, resVm)
		}
	}

	return vms, nil
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
		log.Error(fmt.Errorf("entity not found"), "name", childName)
		return nil, &find.NotFoundError{}
	}

	// We have found a child entity with the given name. Populate the inventory path before returning.
	child, err := s.Finder.ObjectReference(ctx, ref.Reference())
	if err != nil {
		log.Error(err, "error when setting inventory path for the object", "moRef", child.Reference().Value)
		return nil, err
	}
	return child, err
}

// ChildResourcePool returns a child resource pool by a given name under the session's parent
// resource pool, returns error if no child resource pool exists with a given name.
func (s *Session) ChildResourcePool(ctx context.Context, resourcePoolName string) (*object.ResourcePool, error) {
	resourcePool, err := s.findChildEntity(ctx, s.resourcepool, resourcePoolName)
	if err != nil {
		return nil, err
	}

	rp, ok := resourcePool.(*object.ResourcePool)
	if !ok {
		return nil, fmt.Errorf("ResourcePool '%s' not found. '%s' is a %T", resourcePoolName, resourcePoolName, rp)
	}
	return resourcePool.(*object.ResourcePool), err
}

// ChildFolder returns a child resource pool by a given name under the session's parent
// resource pool, returns error if no child resource pool exists with a given name.
func (s *Session) ChildFolder(ctx context.Context, folderName string) (*object.Folder, error) {
	folder, err := s.findChildEntity(ctx, s.folder, folderName)
	if err != nil {
		return nil, err
	}

	folder, ok := folder.(*object.Folder)
	if !ok {
		return nil, fmt.Errorf("Folder '%s' not found. '%s' is a %T", folderName, folderName, folder)
	}
	return folder.(*object.Folder), err
}

// DoesResourcePoolExist checks if a ResourcePool with the given name exists.
func (s *Session) DoesResourcePoolExist(ctx context.Context, namespace, resourcePoolName string) (bool, error) {
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

	// CreteResourcePool is invoked during a ResourcePolicy reconciliation to create a ResourcePool for a set of
	// VirtualMachines. The new RP is created under the RP corresponding to the session.
	// For a Supervisor Cluster deployment, the session's RP is the supervisor cluster namespace's RP.
	// For IAAS deployments, the session's RP correspond to RP in provider ConfigMap.

	resourcePool, err := s.resourcepool.Create(ctx, rpSpec.Name, types.DefaultResourceConfigSpec())
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
	// TODO 
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
func (s *Session) DoesFolderExist(ctx context.Context, namespace, folderName string) (bool, error) {
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

// GetRPAndFolderFromResourcePolicy extracts the govmomi objects for ResourcePool and Folder specified in the Resource Policy
// Returns the sessions RP and Folder if no ResourcePolicy is specified
func (s *Session) GetRPAndFolderFromResourcePolicy(ctx context.Context,
	resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (*object.ResourcePool, *object.Folder, error) {

	if resourcePolicy == nil {
		return s.resourcepool, s.folder, nil
	}

	resourcePoolName := resourcePolicy.Spec.ResourcePool.Name
	resourcePool, err := s.ChildResourcePool(ctx, resourcePoolName)
	if err != nil {
		log.Error(err, "Unable to find ResourcePool", "name", resourcePoolName)
		return nil, nil, err
	}

	log.V(4).Info("Found RP:", "name", resourcePoolName, "moRef", resourcePool.Reference().Value)

	folderName := resourcePolicy.Spec.Folder.Name
	folder, err := s.ChildFolder(ctx, folderName)
	if err != nil {
		log.Error(err, "Unable to find Folder", "name", folderName)
		return nil, nil, err
	}
	log.V(4).Info("Found Folder:", "name", folderName, "moRef", folder.Reference().Value)

	return resourcePool, folder, nil
}

func (s *Session) CloneVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) (*res.VirtualMachine, error) {
	if vmConfigArgs.StorageProfileID == "" {
		if s.storageClassRequired {
			// The storageProfileID is obtained from a StorageClass.
			return nil, fmt.Errorf("storage class is required but not specified")
		}

		if s.datastore == nil {
			return nil, fmt.Errorf("cannot clone VM when neither storage class or datastore is specified")
		}

		log.Info("Will attempt to clone virtual machine", "name", vm.Name, "datastore", s.datastore.Name())

	} else {
		log.Info("Will attempt to clone virtual machine", "name", vm.Name, "storageProfileID", vmConfigArgs.StorageProfileID)
	}

	if s.contentlib != nil {
		item, err := s.GetItemFromCL(ctx, vm.Spec.ImageName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to find image %q", vm.Spec.ImageName)
		}

		switch item.Type {
		case library.ItemTypeOVF:
			return s.cloneVirtualMachineFromLibItemInCL(ctx, vm, vmConfigArgs, item)
		case library.ItemTypeVMTX:
			return s.cloneVirtualMachineFromInventory(ctx, vm, vmConfigArgs)
		}
	}

	if s.useInventoryForImages {
		return s.cloneVirtualMachineFromInventory(ctx, vm, vmConfigArgs)
	}

	return nil, fmt.Errorf("no Content source or inventory configured to clone VM")
}

func (s *Session) cloneVirtualMachineFromInventory(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) (*res.VirtualMachine, error) {

	// Clone Virtual Machine from local.
	resSrcVm, err := s.lookupVmByName(ctx, vm.Spec.ImageName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to lookup clone source %q", vm.Spec.ImageName)
	}

	cloneSpec, err := s.getCloneSpec(ctx, vm.Name, resSrcVm, vm, vmConfigArgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create clone spec")
	}

	cloneResVm, err := s.cloneVm(ctx, resSrcVm, cloneSpec)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to clone new VM %q from %q", vm.Name, resSrcVm.Name)
	}

	return cloneResVm, nil
}

func (s *Session) cloneVirtualMachineFromLibItemInCL(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs, item *library.Item) (*res.VirtualMachine, error) {
	name := vm.Name
	resourcePolicyName := ""
	if vmConfigArgs.ResourcePolicy != nil {
		resourcePolicyName = vmConfigArgs.ResourcePolicy.Name
	}

	log.Info("Deploying CL item", "type", item.Type, "imageName", vm.Spec.ImageName, "vmName", name,
		"resourcePolicyName", resourcePolicyName, "storageProfileID", vmConfigArgs.StorageProfileID)

	deployedVm, err := s.deployOvf(ctx, item.ID, name, vmConfigArgs.ResourcePolicy, vmConfigArgs.StorageProfileID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to deploy new VM %q from %q", name, vm.Spec.ImageName)
	}

	return deployedVm, nil
}

func (s *Session) DeleteVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error {
	resVm, err := s.GetVirtualMachine(ctx, vm)
	if err != nil {
		return err
	}

	err = resVm.Delete(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to delete VM %q", vm.Name)
	}

	return nil
}

func (s *Session) lookupVmByName(ctx context.Context, name string) (*res.VirtualMachine, error) {
	vm, err := s.Finder.VirtualMachine(ctx, name)
	if err != nil {
		return nil, err
	}
	return res.NewVMFromObject(vm)
}

func (s *Session) getVirtualMachineByPath(ctx context.Context, path string) (*object.VirtualMachine, error) {
	return s.Finder.VirtualMachine(ctx, path)
}

func (s *Session) GetVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) (*res.VirtualMachine, error) {
	// Lookup by MoID first, falling back to lookup by full path
	if vm.Status.UniqueID != "" {
		resVm, err := s.lookupVirtualMachineByMoID(ctx, vm.Status.UniqueID)
		if err == nil {
			return resVm, nil
		}
		log.V(4).Info("Failed to lookup VM by MoID", "MoID", vm.Status.UniqueID, "error", err)
	}

	log.V(4).Info("Falling back to resolving VM by full path", "Namespace", vm.Namespace, "Name", vm.Name)

	var folder *object.Folder

	if vm.Spec.ResourcePolicyName != "" {
		// Lookup the VM by name using the full inventory path to the VM.  To do so, we need to acquire the resource policy
		// by name to get the VM's Folder.
		resourcePolicy := &v1alpha1.VirtualMachineSetResourcePolicy{}
		resourcePolicyKey := ctrlruntime.ObjectKey{Name: vm.Spec.ResourcePolicyName, Namespace: vm.Namespace}
		err := s.k8sClient.Get(ctx, resourcePolicyKey, resourcePolicy)
		if err != nil {
			log.Error(err, "Failed to find resource policy", "name", resourcePolicyKey)
			return nil, err
		}

		folderName := resourcePolicy.Spec.Folder.Name
		folder, err = s.ChildFolder(ctx, folderName)
		if err != nil {
			log.Error(err, "Failed to find folder", "folderName", folderName)
			return nil, err
		}
	} else {
		// Developer enablement path: Use the default folder and RP for the session.
		// TODO: AKP: If any of the parent objects have been renamed, the cached inventory path will be stale.
		// 		folder = s.folder
	}

	vmPath := folder.InventoryPath + "/" + vm.Name
	log.V(4).Info("Looking up vm path by", "path", vmPath)

	foundVm, err := s.getVirtualMachineByPath(ctx, vmPath)
	if err != nil {
		log.Error(err, "Failed get VM by path", "path", vmPath)
		return nil, err
	}

	log.V(4).Info("Found VM", "VM", foundVm.Reference())

	return res.NewVMFromObject(foundVm)
}

func (s *Session) lookupVirtualMachineByMoID(ctx context.Context, moId string) (*res.VirtualMachine, error) {
	ref, err := s.Finder.ObjectReference(ctx, types.ManagedObjectReference{Type: "VirtualMachine", Value: moId})
	if err != nil {
		return nil, err
	}

	vm := ref.(*object.VirtualMachine)
	log.V(4).Info("Found VM", "Name", vm.Name(), "Path", vm.InventoryPath, "Moref", vm.Reference())

	return res.NewVMFromObject(vm)
}

func memoryQuantityToMb(q resource.Quantity) int64 {
	return int64(math.Ceil(float64(q.Value()) / float64(1024*1024)))
}

func CpuQuantityToMhz(q resource.Quantity, cpuFreqMhz uint64) int64 {
	return int64(math.Ceil(float64(q.MilliValue()) * float64(cpuFreqMhz) / float64(1000)))
}

func (s *Session) getNicsFromVM(ctx context.Context, vm *v1alpha1.VirtualMachine) ([]vimTypes.BaseVirtualDevice, error) {
	devices := make([]vimTypes.BaseVirtualDevice, 0, len(vm.Spec.NetworkInterfaces))

	// The clients should ensure that existing device keys are not reused as temporary key values for the new device to
	// be added, hence use unique negative integers as temporary keys.
	key := int32(-100)
	for i := range vm.Spec.NetworkInterfaces {
		vif := vm.Spec.NetworkInterfaces[i]
		np, err := NetworkProviderByType(vif.NetworkType, s.k8sClient, s.ncpClient, s.Client.VimClient(), s.Finder, s.cluster)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get network provider")
		}
		dev, err := np.CreateVnic(ctx, vm, &vif)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create vnic '%v'", vif)
		}

		nic := dev.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
		nic.Key = key
		devices = append(devices, dev)

		key--
	}
	return devices, nil
}

// GetNicChangeSpecs returns changes for NIC device that need to be done to get desired VM config
func (s *Session) GetNicChangeSpecs(ctx context.Context, vm *v1alpha1.VirtualMachine, resSrcVm *res.VirtualMachine) ([]vimTypes.BaseVirtualDeviceConfigSpec, error) {
	var deviceSpecs []vimTypes.BaseVirtualDeviceConfigSpec

	// BMV: resSrcVm is never nil
	if resSrcVm != nil {
		netDevices, err := resSrcVm.GetNetworkDevices(ctx)
		if err != nil {
			return nil, err
		}

		// Note: If no network interface is specified in the vm spec we don't remove existing interfaces while cloning.
		// However, if a default network is configured in vmoperator config then we update the backing for the existing
		// network interfaces.
		// BMV: This default network will have to go away for net-op
		if len(vm.Spec.NetworkInterfaces) == 0 {
			if s.network == nil {
				return deviceSpecs, nil
			}

			backingInfo, err := s.network.EthernetCardBackingInfo(ctx)
			if err != nil {
				return nil, errors.Wrapf(err, "unable to create new ethernet card backing info for network %+v", s.network.Reference())
			}

			for _, dev := range netDevices {
				dev.GetVirtualDevice().Backing = backingInfo
				deviceSpecs = append(deviceSpecs, &vimTypes.VirtualDeviceConfigSpec{
					Device:    dev,
					Operation: vimTypes.VirtualDeviceConfigSpecOperationEdit,
				})
			}

			return deviceSpecs, nil
		}

		// Remove all existing NICs
		for _, dev := range netDevices {
			deviceSpecs = append(deviceSpecs, &vimTypes.VirtualDeviceConfigSpec{
				Device:    dev,
				Operation: vimTypes.VirtualDeviceConfigSpecOperationRemove,
			})
		}
	}

	// Add new NICs
	vmNics, err := s.getNicsFromVM(ctx, vm)
	if err != nil {
		return nil, err
	}
	for _, dev := range vmNics {
		deviceSpecs = append(deviceSpecs, &vimTypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
		})
	}

	return deviceSpecs, nil
}

func createDiskLocators(ctx context.Context, resSrcVM *res.VirtualMachine, datastore *vimTypes.ManagedObjectReference, vmProfile []vimTypes.BaseVirtualMachineProfileSpec) (
	[]types.VirtualMachineRelocateSpecDiskLocator, error) {

	disks, err := resSrcVM.GetVirtualDisks(ctx)
	if err != nil {
		return nil, err
	}

	var diskLocators []types.VirtualMachineRelocateSpecDiskLocator
	for _, disk := range disks {
		dl := types.VirtualMachineRelocateSpecDiskLocator{
			DiskId:    disk.GetVirtualDevice().Key,
			Datastore: *datastore,
			Profile:   vmProfile,
			//TODO: Check if policy is encripted and select diskMoveType
			DiskMoveType: string(vimTypes.VirtualMachineRelocateDiskMoveOptionsMoveChildMostDiskBacking),
		}
		diskLocators = append(diskLocators, dl)
	}
	return diskLocators, nil
}

func (s *Session) getCloneSpec(ctx context.Context, name string, resSrcVM *res.VirtualMachine,
	vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) (*vimTypes.VirtualMachineCloneSpec, error) {

	configSpec, err := s.generateConfigSpec(name, &vm.Spec, &vmConfigArgs.VmClass.Spec, vmConfigArgs.VmMetadata, nil, nil)
	if err != nil {
		return nil, err
	}

	memory := false // No full memory clones
	cloneSpec := &vimTypes.VirtualMachineCloneSpec{
		Config: configSpec,
		Memory: &memory,
	}

	nicSpecs, err := s.GetNicChangeSpecs(ctx, vm, resSrcVM)
	if err != nil {
		return nil, err
	}

	for _, changeSpec := range nicSpecs {
		if changeSpec.GetVirtualDeviceConfigSpec().Operation == vimTypes.VirtualDeviceConfigSpecOperationEdit {
			cloneSpec.Location.DeviceChange = append(cloneSpec.Location.DeviceChange, changeSpec)
		} else {
			cloneSpec.Config.DeviceChange = append(cloneSpec.Config.DeviceChange, changeSpec)
		}
	}

	if vmConfigArgs.StorageProfileID != "" {
		cloneSpec.Location.Profile = []vimTypes.BaseVirtualMachineProfileSpec{
			&vimTypes.VirtualMachineDefinedProfileSpec{ProfileId: vmConfigArgs.StorageProfileID},
		}
	} else {
		// BMV: Needed to for placement? Otherwise always overwritten below.
		cloneSpec.Location.Datastore = vimTypes.NewReference(s.datastore.Reference())
	}

	resourcePool, folder, err := s.GetRPAndFolderFromResourcePolicy(ctx, vmConfigArgs.ResourcePolicy)
	if err != nil {
		return nil, err
	}

	// BMV: Why NewReference() b/c we don't elsewhere?
	cloneSpec.Location.Pool = vimTypes.NewReference(resourcePool.Reference())
	cloneSpec.Location.Folder = vimTypes.NewReference(folder.Reference())

	vmRef := &vimTypes.ManagedObjectReference{Type: "VirtualMachine", Value: resSrcVM.ReferenceValue()}
	rSpec, err := computeVMPlacement(ctx, s.cluster, vmRef, cloneSpec, vimTypes.PlacementSpecPlacementTypeClone)
	if err != nil {
		return nil, err
	}

	diskLocators, err := createDiskLocators(ctx, resSrcVM, rSpec.Datastore, cloneSpec.Location.Profile)
	if err != nil {
		return nil, err
	}

	cloneSpec.Location.Host = rSpec.Host
	cloneSpec.Location.Datastore = rSpec.Datastore
	cloneSpec.Location.Disk = diskLocators

	return cloneSpec, nil
}

func (s *Session) cloneVm(ctx context.Context, resSrcVm *res.VirtualMachine, cloneSpec *vimTypes.VirtualMachineCloneSpec) (*res.VirtualMachine, error) {
	log.Info("Cloning VM", "name", cloneSpec.Config.Name, "cloneSpec", *cloneSpec)

	deployment, err := resSrcVm.Clone(ctx, s.folder, cloneSpec)
	if err != nil {
		return nil, err
	}

	ref, err := s.Finder.ObjectReference(ctx, deployment.Reference())
	if err != nil {
		return nil, err
	}

	return res.NewVMFromObject(ref.(*object.VirtualMachine))
}

// policyThickProvision returns true if the storage profile is vSAN and disk provisioning is thick, false otherwise.
// thick provisioning is determined based on its "proportionalCapacity":
// Percentage (0-100) of the logical size of the storage object that will be reserved upon provisioning.
// The UI presents options for "thin" (0%), 25%, 50%, 75% and "thick" (100%)
func policyThickProvision(profile pbmTypes.BasePbmProfile) bool {
	cap, ok := profile.(*pbmTypes.PbmCapabilityProfile)
	if !ok {
		return false
	}

	if cap.ResourceType.ResourceType != string(pbmTypes.PbmProfileResourceTypeEnumSTORAGE) {
		return false
	}

	if cap.ProfileCategory != string(pbmTypes.PbmProfileCategoryEnumREQUIREMENT) {
		return false
	}

	sub, ok := cap.Constraints.(*pbmTypes.PbmCapabilitySubProfileConstraints)
	if !ok {
		return false
	}

	for _, p := range sub.SubProfiles {
		for _, cap := range p.Capability {
			if cap.Id.Namespace != "VSAN" || cap.Id.Id != "proportionalCapacity" {
				continue
			}

			for _, c := range cap.Constraint {
				for _, prop := range c.PropertyInstance {
					if prop.Id != cap.Id.Id {
						continue
					}
					if val, ok := prop.Value.(int32); ok {
						return val == 100 // 100% == thick provisioning
					}
				}
			}
		}
	}

	return false
}

// setStorageProvisioning will change dSpec.StorageProvisioning = thick if the given storage profile is thick.
// Currently this would only happen if the profile is vSAN with proportionalCapacity == 100%.
func (s *Session) setStorageProvisioning(ctx context.Context, dSpec *vcenter.DeploymentSpec, storageProfileID string) error {
	c, err := pbm.NewClient(ctx, s.Client.VimClient())
	if err != nil {
		return err
	}
	profiles, err := c.RetrieveContent(ctx, []pbmTypes.PbmProfileId{{UniqueId: storageProfileID}})
	if err != nil {
		return err
	}

	if len(profiles) != 0 {
		thick := policyThickProvision(profiles[0])
		if thick {
			dSpec.StorageProvisioning = string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeThick)
		}
		log.Info("StorageProfile", "id", storageProfileID, "thick", thick)
	} // else defer error handling to library.Deploy when storage profile can't be found

	return nil
}

func (s *Session) deployOvf(ctx context.Context, itemID string, vmName string, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy,
	storageProfileID string) (*res.VirtualMachine, error) {

	resourcePool, folder, err := s.GetRPAndFolderFromResourcePolicy(ctx, resourcePolicy)
	if err != nil {
		return nil, err
	}

	// BMV: Could set ExtraConfig here but doesn't gain us much.
	dSpec := vcenter.DeploymentSpec{
		Name: vmName,
		// TODO (): Plumb AcceptAllEULA to this Spec
		AcceptAllEULA:       true,
		StorageProvisioning: string(vimTypes.OvfCreateImportSpecParamsDiskProvisioningTypeThin),
	}

	if storageProfileID != "" {
		dSpec.StorageProfileID = storageProfileID
		err = s.setStorageProvisioning(ctx, &dSpec, storageProfileID)
		if err != nil {
			return nil, err
		}
	} else {
		dSpec.DefaultDatastoreID = s.datastore.Reference().Value
	}

	target := vcenter.Target{
		ResourcePoolID: resourcePool.Reference().Value,
		FolderID:       folder.Reference().Value,
	}

	deploy := vcenter.Deploy{
		DeploymentSpec: dSpec,
		Target:         target,
	}

	log.Info("DeployLibraryItem", "context", ctx, "itemID", itemID, "deploy", deploy)

	restClient := s.Client.RestClient()
	deployment, err := vcenter.NewManager(restClient).DeployLibraryItem(ctx, itemID, deploy)
	if err != nil {
		return nil, err
	}

	ref, err := s.Finder.ObjectReference(ctx, deployment.Reference())
	if err != nil {
		return nil, err
	}

	return res.NewVMFromObject(ref.(*object.VirtualMachine))
}

func renderTemplate(name, text string, obj interface{}) string {
	t, err := template.New(name).Parse(text)
	if err != nil {
		return text
	}
	b := strings.Builder{}
	if err := t.Execute(&b, obj); err != nil {
		return text
	}
	return b.String()
}

func ApplyVmSpec(meta map[string]string, vmSpec *v1alpha1.VirtualMachineSpec) map[string]string {
	result := make(map[string]string, len(meta))
	for k, v := range meta {
		result[k] = renderTemplate(k, v, vmSpec)
	}
	return result
}

func MergeExtraConfig(vmSpecMeta, globalMeta map[string]string) map[string]string {
	if len(globalMeta) == 0 {
		return vmSpecMeta
	}

	// global values for extraConfig have been configured, apply them here
	mergedConfig := make(map[string]string)
	for k, v := range globalMeta {
		mergedConfig[k] = v
	}
	// Ensure that VM-specified extraConfig overrides global values
	for k, v := range vmSpecMeta {
		mergedConfig[k] = v
	}

	return mergedConfig
}

func GetExtraConfig(mergedConfig map[string]string) []vimTypes.BaseOptionValue {
	if len(mergedConfig) == 0 {
		return nil
	}
	extraConfigs := make([]vimTypes.BaseOptionValue, 0, len(mergedConfig))
	for k, v := range mergedConfig {
		extraConfigs = append(extraConfigs, &vimTypes.OptionValue{Key: k, Value: v})
	}
	return extraConfigs
}

func GetvAppConfigSpec(ctx context.Context, resVm *res.VirtualMachine, vmConfigArgs *vmprovider.VmConfigArgs) (*vimTypes.VmConfigSpec, error) {
	if vmConfigArgs.VmMetadata == nil || vmConfigArgs.VmMetadata.Transport != v1alpha1.VirtualMachineMetadataOvfEnvTransport {
		return nil, nil
	}
	vAppConfigInfo, err := resVm.GetVAppVmConfigInfo(ctx)
	if err != nil || vAppConfigInfo == nil {
		return nil, err
	}
	return GetMergedvAppConfigSpec(vmConfigArgs.VmMetadata.Data, vAppConfigInfo.Property), nil
}

// Prepare a vApp VmConfigSpec which will set the vmMetadata supplied key/value fields. Only
// fields marked userConfigurable and pre-existing on the VM (ie. originated from the OVF Image)
// will be set, and all others will be ignored.
func GetMergedvAppConfigSpec(inProps map[string]string, vmProps []vimTypes.VAppPropertyInfo) *vimTypes.VmConfigSpec {
	outProps := []vimTypes.VAppPropertySpec{}
	for _, vmProp := range vmProps {
		if vmProp.UserConfigurable == nil || !*vmProp.UserConfigurable {
			continue
		}
		inPropValue, found := inProps[vmProp.Id]
		if !found {
			continue
		}

		vmPropCopy := vmProp
		vmPropCopy.Value = inPropValue
		outProp := vimTypes.VAppPropertySpec{
			ArrayUpdateSpec: vimTypes.ArrayUpdateSpec{
				Operation: vimTypes.ArrayUpdateOperationEdit,
			},
			Info: &vmPropCopy,
		}
		outProps = append(outProps, outProp)
	}

	if len(outProps) == 0 {
		return nil
	}
	return &vimTypes.VmConfigSpec{Property: outProps}
}

// generateConfigSpec generates a configSpec from the VM Spec and the VM Class
func (s *Session) generateConfigSpec(name string, vmSpec *v1alpha1.VirtualMachineSpec, vmClassSpec *v1alpha1.VirtualMachineClassSpec,
	vmMetadata *vmprovider.VmMetadata, deviceSpecs []vimTypes.BaseVirtualDeviceConfigSpec,
	vAppConfigSpec vimTypes.BaseVmConfigSpec) (*vimTypes.VirtualMachineConfigSpec, error) {

	configSpec := &vimTypes.VirtualMachineConfigSpec{
		Name:         name,
		Annotation:   "Virtual Machine managed by the vSphere Virtual Machine service",
		NumCPUs:      int32(vmClassSpec.Hardware.Cpus),
		MemoryMB:     memoryQuantityToMb(vmClassSpec.Hardware.Memory),
		DeviceChange: deviceSpecs,
	}

	if vAppConfigSpec != nil {
		configSpec.VAppConfig = vAppConfigSpec.GetVmConfigSpec()
	}

	// Enable clients to differentiate the managed VMs from the regular VMs.
	configSpec.ManagedBy = &vimTypes.ManagedByInfo{
		ExtensionKey: "com.vmware.vcenter.wcp",
		Type:         "VirtualMachine",
	}

	configSpec.CpuAllocation = &vimTypes.ResourceAllocationInfo{}

	minFreq := s.GetCpuMinMHzInCluster()
	if !vmClassSpec.Policies.Resources.Requests.Cpu.IsZero() {
		rsv := CpuQuantityToMhz(vmClassSpec.Policies.Resources.Requests.Cpu, minFreq)
		configSpec.CpuAllocation.Reservation = &rsv
	}

	if !vmClassSpec.Policies.Resources.Limits.Cpu.IsZero() {
		lim := CpuQuantityToMhz(vmClassSpec.Policies.Resources.Limits.Cpu, minFreq)
		configSpec.CpuAllocation.Limit = &lim
	}

	configSpec.MemoryAllocation = &vimTypes.ResourceAllocationInfo{}

	if !vmClassSpec.Policies.Resources.Requests.Memory.IsZero() {
		rsv := memoryQuantityToMb(vmClassSpec.Policies.Resources.Requests.Memory)
		configSpec.MemoryAllocation.Reservation = &rsv
	}

	if !vmClassSpec.Policies.Resources.Limits.Memory.IsZero() {
		lim := memoryQuantityToMb(vmClassSpec.Policies.Resources.Limits.Memory)
		configSpec.MemoryAllocation.Limit = &lim
	}

	mergedConfig := s.extraConfig
	if vmMetadata != nil && vmMetadata.Data != nil && vmMetadata.Transport == v1alpha1.VirtualMachineMetadataExtraConfigTransport {
		mergedConfig = MergeExtraConfig(vmMetadata.Data, s.extraConfig)
	}
	renderedExtraConfig := ApplyVmSpec(mergedConfig, vmSpec)
	configSpec.ExtraConfig = GetExtraConfig(renderedExtraConfig)

	return configSpec, nil
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

func IsSupportedDeployType(t string) bool {
	switch t {
	case
		library.ItemTypeVMTX, library.ItemTypeOVF:
		return true
	}
	return false
}

// GetCustomizationSpec creates the customization spec for the vm
func (s *Session) GetCustomizationSpec(ctx context.Context, vm *v1alpha1.VirtualMachine, realVM *res.VirtualMachine) (*vimTypes.CustomizationSpec, error) {
	vmName := vm.Name

	customSpec := &vimTypes.CustomizationSpec{
		GlobalIPSettings: vimTypes.CustomizationGlobalIPSettings{},
		// This spec is for Linux guest OS. Need to change if other guest OS needs to be supported.
		Identity: &vimTypes.CustomizationLinuxPrep{
			HostName: &vimTypes.CustomizationFixedName{
				Name: vmName,
			},
			HwClockUTC: vimTypes.NewBool(true),
		},
	}

	nameserverList, err := GetNameserversFromConfigMap(s.k8sClient)
	if err != nil {
		log.Error(err, "Cannot set customized DNS servers", "vmName", vmName)
	} else {
		customSpec.GlobalIPSettings.DnsServerList = nameserverList
	}

	var interfaceCustomizations []vimTypes.CustomizationAdapterMapping
	if len(vm.Spec.NetworkInterfaces) == 0 {
		// In the corresponding code in GetNicChangeSpecs(), none of the existing interfaces were changed,
		// so GetNetworkDevices() will give us all the original interfaces. Assume they should be
		// configured for DHCP since that was the behavior of the prior code. The config is currently
		// really only used in the test environments.
		netDevices, err := realVM.GetNetworkDevices(ctx)
		if err != nil {
			return nil, err
		}

		for _, dev := range netDevices {
			card, ok := dev.(vimTypes.BaseVirtualEthernetCard)
			if !ok {
				continue
			}

			interfaceCustomizations = append(interfaceCustomizations, vimTypes.CustomizationAdapterMapping{
				MacAddress: card.GetVirtualEthernetCard().MacAddress,
				Adapter: vimTypes.CustomizationIPSettings{
					Ip: &vimTypes.CustomizationDhcpIpGenerator{},
				},
			})
		}
	} else {
		// In the corresponding code in GetNicChangeSpecs(), any existing interfaces were removed, and
		// the interfaces in NetworkInterfaces[] were added in order. There is an assumption here that
		// net devices are in the same order, and that they are created in in PCI order for GOSC. That is
		// not really an issue right now because we only ever one network interface. If needed, we can
		// later sort the devices by key like WCP does, but in general NIC reconciliation is difficult
		// with what's currently available. This code is in general pretty brittle.
		for idx := range vm.Spec.NetworkInterfaces {
			nif := vm.Spec.NetworkInterfaces[idx]

			np, err := NetworkProviderByType(nif.NetworkType, s.k8sClient, s.ncpClient, s.Client.VimClient(), s.Finder, s.cluster)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get network provider")
			}

			customization, err := np.GetInterfaceGuestCustomization(vm, &nif)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get guest customization for interface %+v", nif)
			}

			interfaceCustomizations = append(interfaceCustomizations, *customization)
		}
	}

	customSpec.NicSettingMap = interfaceCustomizations

	return customSpec, nil
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
		log.V(4).Info("Successfully set (re)computed CPU min frequency", "prevFreq", prevFreq, "newFreq", minFreq)
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

	obj := cluster.Reference()

	err := cluster.Properties(ctx, obj, nil, &cr)
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

func isNilPtr(i interface{}) bool {
	if i != nil {
		return (reflect.ValueOf(i).Kind() == reflect.Ptr) && (reflect.ValueOf(i).IsNil())
	}
	return true
}

func (s *Session) String() string {
	var sb strings.Builder
	sb.WriteString("{")
	if s.Client != nil {
		sb.WriteString(fmt.Sprintf("client: %+v, ", s.Client))
	}
	if !isNilPtr(s.ncpClient) {
		sb.WriteString(fmt.Sprintf("ncpClient: %+v, ", s.ncpClient))
	}
	if s.contentlib != nil {
		sb.WriteString(fmt.Sprintf("contentlib: %+v, ", *s.contentlib))
	}
	sb.WriteString(fmt.Sprintf("datacenter: %s, ", s.datacenter))
	if s.folder != nil {
		sb.WriteString(fmt.Sprintf("folder: %s, ", s.folder.Reference().Value))
	}
	if s.network != nil {
		sb.WriteString(fmt.Sprintf("network: %s, ", s.network.Reference().Value))
	}
	if s.resourcepool != nil {
		sb.WriteString(fmt.Sprintf("resourcepool: %s, ", s.resourcepool.Reference().Value))
	}
	if s.cluster != nil {
		sb.WriteString(fmt.Sprintf("cluster: %s, ", s.cluster.Reference().Value))
	}
	if s.datastore != nil {
		sb.WriteString(fmt.Sprintf("datastore: %s ", s.datastore.Reference().Value))
	}
	sb.WriteString(fmt.Sprintf("cpuMinMHzInCluster: %v ", s.cpuMinMHzInCluster))
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
	m := cluster.NewManager(restClient)
	modules, err := m.ListModules(ctx)
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
func (s *Session) AttachTagToVm(ctx context.Context, tagName string, tagCatName string, resVm *res.VirtualMachine) error {
	log.Info("Attaching tag", "tag", tagName, "vmName", resVm.Name)

	restClient := s.Client.RestClient()
	manager := tags.NewManager(restClient)
	tag, err := manager.GetTagForCategory(ctx, tagName, tagCatName)
	if err != nil {
		return err
	}

	vmRef := &vimTypes.ManagedObjectReference{Type: "VirtualMachine", Value: resVm.ReferenceValue()}
	return manager.AttachTag(ctx, tag.ID, vmRef)
}

// DetachTagFromVm detaches a tag with a given name from the vm.
func (s *Session) DetachTagFromVm(ctx context.Context, tagName string, tagCatName string, resVm *res.VirtualMachine) error {
	log.Info("Detaching tag", "tag", tagName, "vmName", resVm.Name)

	restClient := s.Client.RestClient()
	manager := tags.NewManager(restClient)
	tag, err := manager.GetTagForCategory(ctx, tagName, tagCatName)
	if err != nil {
		return err
	}

	vmRef := &vimTypes.ManagedObjectReference{Type: "VirtualMachine", Value: resVm.ReferenceValue()}
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

// RenameSessionFolder renames the folder corresponding to this session. Used only by integration tests for now.
func (s *Session) RenameSessionFolder(ctx context.Context, name string) error {
	task, err := s.folder.Rename(ctx, name)
	if err != nil {
		log.Error(err, "Failed to invoke rename for folder", "folderMoID", s.folder.Reference().Value)
		return err
	}

	if taskResult, err := task.WaitForResult(ctx, nil); err != nil {
		msg := ""
		if taskResult != nil && taskResult.Error != nil {
			msg = taskResult.Error.LocalizedMessage
		}
		log.Error(err, "Error in renaming cluster", "clusterMoID", s.folder.Reference().Value, "msg", msg)
		return err
	}

	return nil
}

func (s *Session) updateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) (*res.VirtualMachine, error) {
	resVM, err := s.GetVirtualMachine(ctx, vm)
	if err != nil {
		return nil, err
	}

	isOff, err := resVM.IsVMPoweredOff(ctx)
	if err != nil {
		return nil, err
	}

	// This is just a horrible, temporary hack so that we reconfigure "once" and not disrupt a running VM.
	if isOff {
		// Add device change specs to configSpec
		deviceSpecs, err := s.GetNicChangeSpecs(ctx, vm, resVM)
		if err != nil {
			return nil, err
		}

		vAppConfigSpec, err := GetvAppConfigSpec(ctx, resVM, &vmConfigArgs)
		if err != nil {
			return nil, err
		}

		configSpec, err := s.generateConfigSpec(vm.Name, &vm.Spec, &vmConfigArgs.VmClass.Spec, vmConfigArgs.VmMetadata, deviceSpecs, vAppConfigSpec)
		if err != nil {
			return nil, err
		}

		err = resVM.Reconfigure(ctx, configSpec)
		if err != nil {
			return nil, err
		}

		customizationSpec, err := s.GetCustomizationSpec(ctx, vm, resVM)
		if err != nil {
			return nil, err
		}

		if customizationSpec != nil {
			log.Info("Customizing VM",
				"VirtualMachine", k8sTypes.NamespacedName{Namespace: vm.Namespace, Name: vm.Name},
				"CustomizationSpec", customizationSpec)
			if err := resVM.Customize(ctx, *customizationSpec); err != nil {
				// Ignore customization pending fault as this means we have already tried to customize the VM and it is
				// pending. This can happen if the VM has failed to power-on since the last time we customized the VM. If
				// we don't ignore this error, we will never be able to power-on the VM and the we will always fail here.
				if !IsCustomizationPendingError(err) {
					return nil, err
				}
				log.Info("Ignoring customization error due to pending guest customization", "name", vm.NamespacedName())
			}
		}
	}
	return resVM, nil
}
