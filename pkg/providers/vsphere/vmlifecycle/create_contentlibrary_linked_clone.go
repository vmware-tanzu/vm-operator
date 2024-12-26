// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	clsutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/library"
)

func linkedCloneOVF(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	restClient *rest.Client,
	datacenter *object.Datacenter,
	item *library.Item,
	createArgs *CreateArgs) (*vimtypes.ManagedObjectReference, error) {

	logger := vmCtx.Logger.WithName("linkedCloneOVF")

	if len(createArgs.Datastores) == 0 {
		return nil, errors.New("no compatible datastores")
	}

	// Get the information required to do the linked clone.
	imgInfo, err := getImageLinkedCloneInfo(
		vmCtx,
		k8sClient,
		restClient,
		item)
	if err != nil {
		return nil, err
	}

	topLevelCacheDir, err := clsutil.GetTopLevelCacheDir(
		vmCtx,
		object.NewDatastoreNamespaceManager(vimClient),
		datacenter,
		object.NewDatastore(vimClient, createArgs.Datastores[0].MoRef),
		createArgs.Datastores[0].Name,
		createArgs.Datastores[0].URL,
		createArgs.Datastores[0].TopLevelDirectoryCreateSupported)
	if err != nil {
		return nil, fmt.Errorf("failed to create top-level cache dir: %w", err)
	}
	logger.Info("Got top-level cache dir", "topLevelCacheDir", topLevelCacheDir)

	dstDir := clsutil.GetCacheDirForLibraryItem(
		topLevelCacheDir,
		imgInfo.ItemID,
		imgInfo.ItemContentVersion)
	logger.Info("Got item cache dir", "dstDir", dstDir)

	dstURIs, err := clsutil.CacheStorageURIs(
		vmCtx,
		newCacheStorageURIsClient(vimClient),
		datacenter,
		datacenter,
		dstDir,
		imgInfo.DiskURIs...)
	if err != nil {
		return nil, fmt.Errorf("failed to cache library item disks: %w", err)
	}
	logger.Info("Got parent disks", "dstURIs", dstURIs)

	vmDir := path.Dir(createArgs.ConfigSpec.Files.VmPathName)
	logger.Info("Got vm dir", "vmDir", vmDir)

	// Update the ConfigSpec with the disk chains.
	var disks []*vimtypes.VirtualDisk
	for i := range createArgs.ConfigSpec.DeviceChange {
		dc := createArgs.ConfigSpec.DeviceChange[i].GetVirtualDeviceConfigSpec()
		if d, ok := dc.Device.(*vimtypes.VirtualDisk); ok {
			disks = append(disks, d)

			// The profile is no longer needed since we have placement.
			dc.Profile = nil
		}
	}
	logger.Info("Got disks", "disks", disks)

	if a, b := len(dstURIs), len(disks); a != b {
		return nil, fmt.Errorf(
			"invalid disk count: len(uris)=%d, len(disks)=%d", a, b)
	}

	for i := range disks {
		d := disks[i]
		if bfb, ok := d.Backing.(vimtypes.BaseVirtualDeviceFileBackingInfo); ok {
			fb := bfb.GetVirtualDeviceFileBackingInfo()
			fb.Datastore = &createArgs.Datastores[0].MoRef
			fb.FileName = fmt.Sprintf("%s/%s-%d.vmdk", vmDir, vmCtx.VM.Name, i)
		}
		if fb, ok := d.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo); ok {
			fb.Parent = &vimtypes.VirtualDiskFlatVer2BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					Datastore: &createArgs.Datastores[0].MoRef,
					FileName:  dstURIs[i],
				},
				DiskMode:        string(vimtypes.VirtualDiskModePersistent),
				ThinProvisioned: ptr.To(true),
			}
		}
	}

	// The profile is no longer needed since we have placement.
	createArgs.ConfigSpec.VmProfile = nil

	vmCtx.Logger.Info(
		"Deploying OVF Library Item as linked clone",
		"itemID", item.ID,
		"itemName", item.Name,
		"configSpec", createArgs.ConfigSpec)

	folder := object.NewFolder(
		vimClient,
		vimtypes.ManagedObjectReference{
			Type:  "Folder",
			Value: createArgs.FolderMoID,
		})
	pool := object.NewResourcePool(
		vimClient,
		vimtypes.ManagedObjectReference{
			Type:  "ResourcePool",
			Value: createArgs.ResourcePoolMoID,
		})

	createTask, err := folder.CreateVM(
		vmCtx,
		createArgs.ConfigSpec,
		pool,
		nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call create task: %w", err)
	}

	createTaskInfo, err := createTask.WaitForResult(vmCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for create task: %w", err)
	}

	vmRef, ok := createTaskInfo.Result.(vimtypes.ManagedObjectReference)
	if !ok {
		return nil, fmt.Errorf(
			"failed to assert create task result is ref: %[1]T %+[1]v",
			createTaskInfo.Result)
	}

	return &vmRef, nil
}

func getImageLinkedCloneInfo(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	restClient *rest.Client,
	item *library.Item) (vmopv1util.ImageDiskInfo, error) {

	imgInfo, err := vmopv1util.GetImageDiskInfo(
		vmCtx,
		k8sClient,
		*vmCtx.VM.Spec.Image,
		vmCtx.VM.Namespace)

	if err == nil {
		return imgInfo, nil
	}

	if !errors.Is(err, vmopv1util.ErrImageNotSynced) {
		return vmopv1util.ImageDiskInfo{},
			fmt.Errorf("failed to get image info before syncing: %w", err)
	}

	if err := clsutil.SyncLibraryItem(
		vmCtx,
		library.NewManager(restClient),
		item.ID); err != nil {

		return vmopv1util.ImageDiskInfo{}, fmt.Errorf("failed to sync: %w", err)
	}

	if imgInfo, err = vmopv1util.GetImageDiskInfo(
		vmCtx,
		k8sClient,
		*vmCtx.VM.Spec.Image,
		vmCtx.VM.Namespace); err != nil {

		return vmopv1util.ImageDiskInfo{},
			fmt.Errorf("failed to get image info after syncing: %w", err)
	}

	return imgInfo, nil
}

func newCacheStorageURIsClient(c *vim25.Client) clsutil.CacheStorageURIsClient {
	return &cacheStorageURIsClient{
		FileManager:        object.NewFileManager(c),
		VirtualDiskManager: object.NewVirtualDiskManager(c),
	}
}

type cacheStorageURIsClient struct {
	*object.FileManager
	*object.VirtualDiskManager
}

func (c *cacheStorageURIsClient) WaitForTask(
	ctx context.Context, task *object.Task) error {

	return task.Wait(ctx)
}
