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

	"github.com/go-logr/logr"
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
	dstDir string,
	dstDisksFormat vimtypes.DatastoreSectorFormat,
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

	var dstStorageURIs = make([]CachedDisk, len(srcDiskURIs))

	for i := range srcDiskURIs {
		dstFilePath, err := copyFile(
			ctx,
			client,
			dstDir,
			srcDiskURIs[i],
			dstDisksFormat,
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
	dstDir, srcFilePath string,
	dstDiskFormat vimtypes.DatastoreSectorFormat,
	dstDatacenter, srcDatacenter *object.Datacenter) (string, error) {

	var (
		srcFileName = path.Base(srcFilePath)
		srcFileExt  = path.Ext(srcFilePath)
		dstFileName = GetCachedFileNameForVMDK(srcFileName) + srcFileExt
		dstFilePath = path.Join(dstDir, dstFileName)
		isDisk      = strings.EqualFold(".vmdk", srcFileExt)
		logger      = logr.FromContextOrDiscard(ctx)
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

// TopLevelCacheDirName is the name of the top-level cache directory created on
// each datastore.
const TopLevelCacheDirName = ".contentlib-cache"

// GetCacheDirForLibraryItem returns the cache directory for a library item
// beneath a top-level cache directory.
func GetCacheDirForLibraryItem(
	topLevelCacheDir, itemUUID, contentVersion string) string {

	if topLevelCacheDir == "" {
		panic("topLevelCacheDir is empty")
	}
	if itemUUID == "" {
		panic("itemUUID is empty")
	}
	if contentVersion == "" {
		panic("contentVersion is empty")
	}

	// Encode the contentVersion to ensure it is safe to use as part of the
	// directory name.
	contentVersion = pkgutil.SHA1Sum17(contentVersion)

	return path.Join(topLevelCacheDir, itemUUID, contentVersion)
}

// GetCachedFileNameForVMDK returns the first 17 characters of a SHA-1 sum of
// a VMDK file name and extension, ex. my-disk.vmdk.
func GetCachedFileNameForVMDK(vmdkFileName string) string {
	if vmdkFileName == "" {
		panic("vmdkFileName is empty")
	}
	if ext := path.Ext(vmdkFileName); ext != "" {
		vmdkFileName, _ = strings.CutSuffix(vmdkFileName, ext)
	}
	return pkgutil.SHA1Sum17(vmdkFileName)
}
