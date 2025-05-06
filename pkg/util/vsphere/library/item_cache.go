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
		dstFilePath, err := copyDisk(
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

func copyDisk(
	ctx context.Context,
	client CacheStorageURIsClient,
	dstDir, srcFilePath string,
	dstDisksFormat vimtypes.DatastoreSectorFormat,
	dstDatacenter, srcDatacenter *object.Datacenter) (string, error) {

	var (
		srcFileName = path.Base(srcFilePath)
		srcFileExt  = path.Ext(srcFilePath)
		dstFileName = GetCachedFileNameForVMDK(srcFileName) + srcFileExt
		dstFilePath = path.Join(dstDir, dstFileName)
	)

	// Check to see if the disk is already cached.
	queryDiskErr := client.DatastoreFileExists(
		ctx,
		dstFilePath,
		dstDatacenter)
	if queryDiskErr == nil {
		// Disk exists, return the path to it.
		return dstFilePath, nil
	}
	if !errors.Is(queryDiskErr, os.ErrNotExist) {
		return "", fmt.Errorf("failed to query disk: %w", queryDiskErr)
	}

	// Ensure the directory where the disks will be cached exists.
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

	if srcFileExt == ".vmdk" {
		// The base disk does not exist, create it.
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
				SectorFormat: string(dstDisksFormat),
			},
			false)
	} else {
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
