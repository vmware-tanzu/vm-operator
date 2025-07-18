// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func fastDeploy(
	vmCtx pkgctx.VirtualMachineContext,
	vimClient *vim25.Client,
	createArgs *CreateArgs) (vmRef *vimtypes.ManagedObjectReference, retErr error) {

	logger := vmCtx.Logger.WithName("fastDeploy")

	if len(createArgs.Datastores) == 0 {
		return nil, errors.New("no compatible datastores")
	}

	vmDir := path.Dir(createArgs.ConfigSpec.Files.VmPathName)
	logger.Info("Got vm dir", "vmDir", vmDir)

	srcFilePaths := createArgs.FilePaths
	logger.Info("Got source file paths", "srcFilePaths", srcFilePaths)

	srcDiskPaths := createArgs.DiskPaths
	logger.Info("Got source disk paths", "srcDiskPaths", srcDiskPaths)

	dstDiskFormat := pkgutil.GetPreferredDiskFormat(
		createArgs.Datastores[0].DiskFormats...)
	logger.Info("Got destination disk format", "dstDiskFormat", dstDiskFormat)

	dstDiskPaths := make([]string, len(srcDiskPaths))
	for i := 0; i < len(dstDiskPaths); i++ {
		dstDiskPaths[i] = fmt.Sprintf("%s/disk-%d.vmdk", vmDir, i)
	}
	logger.Info("Got destination disk paths", "dstDiskPaths", dstDiskPaths)

	dstFilePaths := make([]string, len(srcFilePaths))
	for i := 0; i < len(dstFilePaths); i++ {
		// Rename file using vm name + same extension
		ext := path.Ext(srcFilePaths[i])
		name := createArgs.ConfigSpec.Name + ext

		dstFilePaths[i] = path.Join(vmDir, name)
	}
	logger.Info("Got destination file paths", "dstFilePaths", dstFilePaths)

	// Collect the disks and remove the storage profile from them.
	var (
		disks     []*vimtypes.VirtualDisk
		diskSpecs []*vimtypes.VirtualDeviceConfigSpec
	)
	for i := range createArgs.ConfigSpec.DeviceChange {
		dc := createArgs.ConfigSpec.DeviceChange[i].GetVirtualDeviceConfigSpec()
		if d, ok := dc.Device.(*vimtypes.VirtualDisk); ok {
			d.VirtualDiskFormat = string(dstDiskFormat)
			disks = append(disks, d)
			diskSpecs = append(diskSpecs, dc)
		}
	}

	if a, b := len(srcDiskPaths), len(disks); a != b {
		logger.Info("Got disks", "disks", disks)
		return nil, fmt.Errorf(
			"invalid disk count: len(srcDiskPaths)=%d, len(disks)=%d", a, b)
	}

	// Update the disks with their expected file names.
	for i := range disks {
		d := disks[i]
		if bfb, ok := d.Backing.(vimtypes.BaseVirtualDeviceFileBackingInfo); ok {
			fb := bfb.GetVirtualDeviceFileBackingInfo()
			fb.Datastore = &createArgs.Datastores[0].MoRef
			fb.FileName = dstDiskPaths[i]
		}
	}
	logger.Info("Got disks", "disks", disks)

	datacenter := object.NewDatacenter(
		vimClient,
		vimtypes.ManagedObjectReference{
			Type:  string(vimtypes.ManagedObjectTypesDatacenter),
			Value: createArgs.DatacenterMoID,
		})
	logger.Info("Got datacenter", "datacenter", datacenter.Reference())

	// Create the directory where the VM will be created.
	fm := object.NewFileManager(vimClient)
	if err := fm.MakeDirectory(vmCtx, vmDir, datacenter, true); err != nil {
		return nil, fmt.Errorf("failed to create vm dir %q: %w", vmDir, err)
	}

	// Copy any non-disk files to the target directory.
	for i := range dstFilePaths {
		task, err := fm.CopyDatastoreFile(
			vmCtx,
			srcFilePaths[i],
			datacenter,
			dstFilePaths[i],
			datacenter, false)
		if err != nil {
			return nil, err
		}
		if err = task.Wait(vmCtx); err != nil {
			return nil, err
		}
	}

	// Update the config spec to know about a potential NVRAM file.
	for i := range createArgs.ConfigSpec.ExtraConfig {
		ov := createArgs.ConfigSpec.ExtraConfig[i].GetOptionValue()

		if ov != nil && ov.Key == "nvram" {

			if v, ok := ov.Value.(string); ok {

				oldNvramPath := v

				for j := range dstFilePaths {
					if strings.EqualFold(".nvram", path.Ext(dstFilePaths[j])) {
						newNvramPath := path.Base(dstFilePaths[j])
						logger.Info("Updated NVRAM in ExtraConfig",
							"oldValue", oldNvramPath,
							"newValue", newNvramPath)
						createArgs.ConfigSpec.ExtraConfig[i] = &vimtypes.OptionValue{
							Key:   ov.Key,
							Value: newNvramPath,
						}
					}
				}
			}
		}
	}

	// If any error occurs after this point, the newly created VM directory and
	// its contents need to be cleaned up.
	defer func() {
		if retErr == nil {
			// Do not delete the VM directory if this function was successful.
			return
		}

		// Use a new context to ensure cleanup happens even if the context
		// is cancelled.
		ctx := context.Background()

		// Delete the VM directory and its contents.
		t, err := fm.DeleteDatastoreFile(ctx, vmDir, datacenter)
		if err != nil {
			err = fmt.Errorf(
				"failed to call delete api for vm dir %q: %w", vmDir, err)
			if retErr == nil {
				retErr = err
			} else {
				retErr = fmt.Errorf("%w,%w", err, retErr)
			}
			return
		}

		// Wait for the delete call to return.
		if err := t.Wait(ctx); err != nil {
			if !fault.Is(err, &vimtypes.FileNotFound{}) {
				err = fmt.Errorf("failed to delete vm dir %q: %w", vmDir, err)
				if retErr == nil {
					retErr = err
				} else {
					retErr = fmt.Errorf("%w,%w", err, retErr)
				}
			}
		}
	}()

	folder := object.NewFolder(vimClient, vimtypes.ManagedObjectReference{
		Type:  "Folder",
		Value: createArgs.FolderMoID,
	})
	logger.Info("Got folder", "folder", folder.Reference())

	pool := object.NewResourcePool(vimClient, vimtypes.ManagedObjectReference{
		Type:  "ResourcePool",
		Value: createArgs.ResourcePoolMoID,
	})
	logger.Info("Got pool", "pool", pool.Reference())

	var host *object.HostSystem
	if createArgs.HostMoID != "" {
		host = object.NewHostSystem(vimClient, vimtypes.ManagedObjectReference{
			Type:  "HostSystem",
			Value: createArgs.HostMoID,
		})
		logger.Info("Got host", "host", host.Reference())
	}

	// Determine the type of fast deploy operation.
	var fastDeployMode string

	if createArgs.IsEncryptedStorageProfile {
		fastDeployMode = pkgconst.FastDeployModeDirect
	} else {
		fastDeployMode = vmCtx.VM.Annotations[pkgconst.FastDeployAnnotationKey]
		if fastDeployMode == "" {
			fastDeployMode = pkgcfg.FromContext(vmCtx).FastDeployMode
		}
	}
	logger.Info(
		"Deploying OVF Library Item with Fast Deploy",
		"mode", fastDeployMode)

	if strings.EqualFold(fastDeployMode, pkgconst.FastDeployModeLinked) {
		return fastDeployLinked(
			vmCtx,
			folder,
			pool,
			host,
			createArgs.ConfigSpec,
			createArgs.Datastores[0].MoRef,
			disks,
			diskSpecs,
			srcDiskPaths)
	}

	return fastDeployDirect(
		vmCtx,
		datacenter,
		folder,
		pool,
		host,
		createArgs.ConfigSpec,
		diskSpecs,
		dstDiskFormat,
		dstDiskPaths,
		srcDiskPaths)
}

func fastDeployLinked(
	ctx context.Context,
	folder *object.Folder,
	pool *object.ResourcePool,
	host *object.HostSystem,
	configSpec vimtypes.VirtualMachineConfigSpec,
	datastoreRef vimtypes.ManagedObjectReference,
	disks []*vimtypes.VirtualDisk,
	diskSpecs []*vimtypes.VirtualDeviceConfigSpec,
	srcDiskPaths []string) (*vimtypes.ManagedObjectReference, error) {

	logger := pkgutil.FromContextOrDefault(ctx).WithName("fastDeployLinked")

	// Linked clones do not fully support encryption, so remove the possible
	// crypto information from the VM's disks.
	for i := range diskSpecs {
		ds := diskSpecs[i]
		if ds.Backing != nil {
			ds.Backing.Crypto = nil
		}
	}

	for i := range disks {
		fileBackingInfo := vimtypes.VirtualDeviceFileBackingInfo{
			Datastore: &datastoreRef,
			FileName:  srcDiskPaths[i],
		}
		switch tBack := disks[i].Backing.(type) {
		case *vimtypes.VirtualDiskFlatVer2BackingInfo:
			// Point the disk to its parent.
			tBack.Parent = &vimtypes.VirtualDiskFlatVer2BackingInfo{
				VirtualDeviceFileBackingInfo: fileBackingInfo,
				DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
				ThinProvisioned:              ptr.To(true),
			}
		case *vimtypes.VirtualDiskSeSparseBackingInfo:
			// Point the disk to its parent.
			tBack.Parent = &vimtypes.VirtualDiskSeSparseBackingInfo{
				VirtualDeviceFileBackingInfo: fileBackingInfo,
				DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
			}
		case *vimtypes.VirtualDiskSparseVer2BackingInfo:
			// Point the disk to its parent.
			tBack.Parent = &vimtypes.VirtualDiskSparseVer2BackingInfo{
				VirtualDeviceFileBackingInfo: fileBackingInfo,
				DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
			}
		}
	}

	return fastDeployCreateVM(ctx, logger, folder, pool, host, configSpec)
}

func fastDeployDirect(
	ctx context.Context,
	datacenter *object.Datacenter,
	folder *object.Folder,
	pool *object.ResourcePool,
	host *object.HostSystem,
	configSpec vimtypes.VirtualMachineConfigSpec,
	diskSpecs []*vimtypes.VirtualDeviceConfigSpec,
	diskFormat vimtypes.DatastoreSectorFormat,
	dstDiskPaths,
	srcDiskPaths []string) (*vimtypes.ManagedObjectReference, error) {

	logger := pkgutil.FromContextOrDefault(ctx).WithName("fastDeployDirect")

	// Copy each disk into the VM directory.
	if err := fastDeployDirectCopyDisks(
		ctx,
		logger,
		datacenter,
		configSpec,
		srcDiskPaths,
		dstDiskPaths,
		diskFormat); err != nil {

		return nil, err
	}

	_, isVMEncrypted := configSpec.Crypto.(*vimtypes.CryptoSpecEncrypt)

	for i := range diskSpecs {
		ds := diskSpecs[i]

		// Set the file operation to an empty string since the disk already
		// exists.
		ds.FileOperation = ""

		if isVMEncrypted {
			// If the VM is to be encrypted, then the disks need to be updated
			// so they are not marked as encrypted upon VM creation. This is
			// because it is not possible to change the encryption state of VM
			// disks when they are being attached. Instead the disks must be
			// encrypted after they are attached to the VM.
			ds.Profile = nil
			if ds.Backing != nil {
				ds.Backing.Crypto = nil
			}
		}
	}

	return fastDeployCreateVM(ctx, logger, folder, pool, host, configSpec)
}

func fastDeployCreateVM(
	ctx context.Context,
	logger logr.Logger,
	folder *object.Folder,
	pool *object.ResourcePool,
	host *object.HostSystem,
	configSpec vimtypes.VirtualMachineConfigSpec) (*vimtypes.ManagedObjectReference, error) {

	logger.Info("Creating VM", "configSpec", vimtypes.ToString(configSpec))

	createTask, err := folder.CreateVM(
		ctx,
		configSpec,
		pool,
		host)
	if err != nil {
		return nil, fmt.Errorf("failed to call create task: %w", err)
	}

	createTaskInfo, err := createTask.WaitForResult(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for create task: %w", err)
	}

	vmRefVal, ok := createTaskInfo.Result.(vimtypes.ManagedObjectReference)
	if !ok {
		return nil, fmt.Errorf(
			"failed to assert create task result is ref: %[1]T %+[1]v",
			createTaskInfo.Result)
	}

	return &vmRefVal, nil
}

func fastDeployDirectCopyDisks(
	ctx context.Context,
	logger logr.Logger,
	datacenter *object.Datacenter,
	configSpec vimtypes.VirtualMachineConfigSpec,
	srcDiskPaths,
	dstDiskPaths []string,
	diskFormat vimtypes.DatastoreSectorFormat) error {

	var (
		wg            sync.WaitGroup
		copyDiskTasks = make([]*object.Task, len(srcDiskPaths))
		copyDiskErrs  = make(chan error, len(srcDiskPaths))
		copyDiskSpec  = vimtypes.FileBackedVirtualDiskSpec{
			VirtualDiskSpec: vimtypes.VirtualDiskSpec{
				AdapterType: string(vimtypes.VirtualDiskAdapterTypeLsiLogic),
				DiskType:    string(vimtypes.VirtualDiskTypeThin),
			},
			SectorFormat: string(diskFormat),
			Profile:      configSpec.VmProfile,
		}
		diskManager = object.NewVirtualDiskManager(datacenter.Client())
	)

	for i := range srcDiskPaths {
		s := srcDiskPaths[i]
		d := dstDiskPaths[i]

		logger.Info(
			"Copying disk",
			"dstDiskPath", d,
			"srcDiskPath", s,
			"copyDiskSpec", copyDiskSpec)

		t, err := diskManager.CopyVirtualDisk(
			ctx,
			s,
			datacenter,
			d,
			datacenter,
			&copyDiskSpec,
			false)
		if err != nil {
			logger.Error(err, "failed to copy disk, cancelling other tasks")

			// Cancel any other outstanding disk copies.
			for _, t := range copyDiskTasks {
				if t != nil {
					// Cancel the task using a background context to ensure it
					// goes through.
					_ = t.Cancel(context.Background())
				}
			}

			logger.Info("waiting on other copy tasks to complete")
			// Wait on any other outstanding disk copies to complete before
			// returning to ensure the parent folder can be cleaned up.
			wg.Wait()
			logger.Info("waited on other copy tasks to complete")

			return fmt.Errorf("failed to call copy disk %q to %q: %w", s, d, err)
		}

		wg.Add(1)
		copyDiskTasks[i] = t

		go func() {
			defer wg.Done()
			if err := t.Wait(context.Background()); err != nil {
				copyDiskErrs <- fmt.Errorf(
					"failed to copy disk %q to %q: %w", s, d, err)
			}
		}()
	}

	// Wait on all the disk copies to complete before proceeding.
	go func() {
		wg.Wait()
		close(copyDiskErrs)
	}()
	var copyDiskErr error
	for err := range copyDiskErrs {
		if err != nil {
			if copyDiskErr == nil {
				copyDiskErr = err
			} else {
				copyDiskErr = fmt.Errorf("%w,%w", copyDiskErr, err)
			}
		}
	}

	return copyDiskErr
}
