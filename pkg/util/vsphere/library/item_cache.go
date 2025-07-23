// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// CachedDisk may refer to a disk directly via its datastore path or by its
// VDiskID if a First Class Disk (FCD).
type CachedDisk struct {
	Path    string
	VDiskID string

	// TODO(akutz) In the future there may be additional information about the
	//             disk, such as its sector format (512 vs 4k), is encrypted,
	//             thin-provisioned, adapter type, etc.
}

// CacheStorageURIsClient implements the client methods used by the
// CacheStorageURIs method.
type CacheStorageURIsClient interface {
	DatastoreFileExists(
		ctx context.Context,
		name string,
		datacenter *object.Datacenter) error

	CopyVirtualDisk(
		ctx context.Context,
		srcName string, srcDatacenter *object.Datacenter,
		dstName string, dstDatacenter *object.Datacenter,
		dstSpec vimtypes.BaseVirtualDiskSpec, force bool) (*object.Task, error)

	CopyDatastoreFile(
		ctx context.Context,
		srcName string, srcDatacenter *object.Datacenter,
		dstName string, dstDatacenter *object.Datacenter,
		force bool) (*object.Task, error)

	MakeDirectory(
		ctx context.Context,
		name string,
		datacenter *object.Datacenter,
		createParentDirectories bool) error

	WaitForTask(ctx context.Context, task *object.Task) error
}

// CacheStorageURIs copies the disk(s) from srcDiskURIs to dstDir and returns
// the path(s) to the copied disk(s).
func CacheStorageURIs(
	ctx context.Context,
	client CacheStorageURIsClient,
	dstDatacenter, srcDatacenter *object.Datacenter,
	dstDir, dstProfileID string,
	dstDiskFormat vimtypes.DatastoreSectorFormat,
	srcDiskURIs ...string) ([]CachedDisk, error) {

	if pkgutil.IsNil(ctx) {
		panic("context is nil")
	}
	if pkgutil.IsNil(client) {
		panic("client is nil")
	}
	if dstDatacenter == nil {
		panic("dstDatacenter is nil")
	}
	if srcDatacenter == nil {
		panic("srcDatacenter is nil")
	}
	if dstDir == "" {
		panic("dstDir is empty")
	}
	if dstProfileID == "" {
		panic("dstProfileID is empty")
	}

	var dstStorageURIs = make([]CachedDisk, len(srcDiskURIs))

	for i := range srcDiskURIs {
		dstFilePath, err := copyFile(
			ctx,
			client,
			dstDir,
			srcDiskURIs[i],
			dstProfileID,
			dstDiskFormat,
			dstDatacenter,
			srcDatacenter)
		if err != nil {
			return nil, err
		}
		dstStorageURIs[i].Path = dstFilePath
	}

	return dstStorageURIs, nil
}

func copyFile(
	ctx context.Context,
	client CacheStorageURIsClient,
	dstDir, srcFilePath, dstProfileID string,
	dstDiskFormat vimtypes.DatastoreSectorFormat,
	dstDatacenter, srcDatacenter *object.Datacenter) (string, error) {

	var (
		srcFileName = path.Base(srcFilePath)
		srcFileExt  = path.Ext(srcFilePath)
		dstFileName = GetCachedFileName(srcFileName) + srcFileExt
		dstFilePath = path.Join(dstDir, dstFileName)
		isDisk      = strings.EqualFold(".vmdk", srcFileExt)
		logger      = pkgutil.FromContextOrDefault(ctx)
	)

	logger = logger.WithValues(
		"srcFilePath", srcFilePath,
		"dstFilePath", dstFilePath,
		"dstDatacenter", dstDatacenter.Reference().Value,
		"srcDatacenter", srcDatacenter.Reference().Value)
	if isDisk {
		logger = logger.WithValues("dstDiskFormat", dstDiskFormat)
	}

	logger.V(4).Info("Checking if file is already cached")

	// Check to see if the disk is already cached.
	fileExistsErr := client.DatastoreFileExists(
		ctx,
		dstFilePath,
		dstDatacenter)
	if fileExistsErr == nil {
		// File exists, return the path to it.
		logger.V(4).Info("File is already cached")
		return dstFilePath, nil
	}
	if !errors.Is(fileExistsErr, os.ErrNotExist) {
		return "", fmt.Errorf(
			"failed to check if file exists: %w", fileExistsErr)
	}

	// Ensure the directory where the file will be cached exists.
	logger.V(4).Info("Making cache directory")
	if err := client.MakeDirectory(
		ctx,
		dstDir,
		dstDatacenter,
		true); err != nil {

		return "", fmt.Errorf("failed to create folder %q: %w", dstDir, err)
	}

	var (
		copyTask *object.Task
		err      error
	)

	if isDisk {
		logger.Info("Caching disk")
		copyTask, err = client.CopyVirtualDisk(
			ctx,
			srcFilePath,
			srcDatacenter,
			dstFilePath,
			dstDatacenter,
			&vimtypes.FileBackedVirtualDiskSpec{
				VirtualDiskSpec: vimtypes.VirtualDiskSpec{
					AdapterType: string(vimtypes.VirtualDiskAdapterTypeLsiLogic),
					DiskType:    string(vimtypes.VirtualDiskTypeThin),
				},
				SectorFormat: string(dstDiskFormat),
				Profile: []vimtypes.BaseVirtualMachineProfileSpec{
					&vimtypes.VirtualMachineDefinedProfileSpec{
						ProfileId: dstProfileID,
					},
				},
			},
			false)
	} else {
		logger.Info("Caching non-disk file")
		copyTask, err = client.CopyDatastoreFile(
			ctx,
			srcFilePath,
			srcDatacenter,
			dstFilePath,
			dstDatacenter,
			false)
	}

	if err != nil {
		return "", fmt.Errorf("failed to call copy %s: %w", srcFileExt, err)
	}
	if err := client.WaitForTask(ctx, copyTask); err != nil {
		return "", fmt.Errorf("failed to wait for copy %s: %w", srcFileExt, err)
	}

	return dstFilePath, nil
}

// GetCacheDirForLibraryItem returns the cache directory for a library item
// beneath a top-level cache directory.
func GetCacheDirForLibraryItem(
	datastoreName, itemName, profileID, contentVersion string) string {

	if datastoreName == "" {
		panic("datastoreName is empty")
	}
	if itemName == "" {
		panic("itemName is empty")
	}
	if profileID == "" {
		panic("profileID is empty")
	}

	profileID = pkgutil.SHA1Sum17(profileID)

	// TODO(akutz) Remove once image reg v2 sets the version info for VM-backed
	//             images.
	// if contentVersion == "" {
	// 	panic("contentVersion is empty")
	// }

	// Encode the contentVersion to ensure it is safe to use as part of the
	// directory name.
	if contentVersion != "" {
		contentVersion = "-" + pkgutil.SHA1Sum17(contentVersion)
	}

	return fmt.Sprintf("[%s] %s-%s%s",
		datastoreName,
		itemName,
		profileID,
		contentVersion)
}

// GetCachedFileName returns the first 17 characters of a SHA-1 sum of
// a file name and extension, ex. my-disk.vmdk or my-firmware.nvram.
func GetCachedFileName(fileName string) string {
	if fileName == "" {
		panic("fileName is empty")
	}
	if ext := path.Ext(fileName); ext != "" {
		fileName, _ = strings.CutSuffix(fileName, ext)
	}
	return pkgutil.SHA1Sum17(fileName)
}
