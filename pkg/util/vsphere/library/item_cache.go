// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package library

import (
	"context"
	"crypto/sha1" //nolint:gosec // used for creating directory name
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// CacheStorageURIsClient implements the client methods used by the
// CacheStorageURIs method.
type CacheStorageURIsClient interface {
	QueryVirtualDiskUuid(
		ctx context.Context,
		name string,
		datacenter *object.Datacenter) (string, error)

	CopyVirtualDisk(
		ctx context.Context,
		srcName string, srcDatacenter *object.Datacenter,
		dstName string, dstDatacenter *object.Datacenter,
		dstSpec vimtypes.BaseVirtualDiskSpec, force bool) (*object.Task, error)

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
	srcDiskURIs ...string) ([]string, error) {

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

	var dstStorageURIs = make([]string, len(srcDiskURIs))

	for i := range srcDiskURIs {
		dstFilePath, err := copyDisk(
			ctx,
			client,
			dstDir,
			srcDiskURIs[i],
			dstDatacenter,
			srcDatacenter)
		if err != nil {
			return nil, err
		}
		dstStorageURIs[i] = dstFilePath
	}

	return dstStorageURIs, nil
}

func copyDisk(
	ctx context.Context,
	client CacheStorageURIsClient,
	dstDir, srcFilePath string,
	dstDatacenter, srcDatacenter *object.Datacenter) (string, error) {

	var (
		srcFileName = path.Base(srcFilePath)
		dstFileName = GetCachedFileNameForVMDK(srcFileName) + ".vmdk"
		dstFilePath = path.Join(dstDir, dstFileName)
	)

	// Check to see if the disk is already cached.
	_, queryDiskErr := client.QueryVirtualDiskUuid(
		ctx,
		dstFilePath,
		dstDatacenter)
	if queryDiskErr == nil {
		// Disk exists, return the path to it.
		return dstFilePath, nil
	}
	if !fault.Is(queryDiskErr, &vimtypes.FileNotFound{}) {
		return "", fmt.Errorf("failed to query disk uuid: %w", queryDiskErr)
	}

	// Create the VM folder.
	if err := client.MakeDirectory(
		ctx,
		dstDir,
		dstDatacenter,
		true); err != nil {

		return "", fmt.Errorf("failed to create folder %q: %w", dstDir, err)
	}

	// The base disk does not exist, create it.
	copyDiskTask, err := client.CopyVirtualDisk(
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
		},
		false)
	if err != nil {
		return "", fmt.Errorf("failed to call copy disk: %w", err)
	}
	if err := client.WaitForTask(ctx, copyDiskTask); err != nil {
		return "", fmt.Errorf("failed to wait for copy disk: %w", err)
	}

	return dstFilePath, nil
}

const topLevelCacheDirName = ".contentlib-cache"

// GetTopLevelCacheDirClient implements the client methods used by the
// GetTopLevelCacheDir method.
type GetTopLevelCacheDirClient interface {
	CreateDirectory(
		ctx context.Context,
		datastore *object.Datastore,
		displayName, policy string) (string, error)

	ConvertNamespacePathToUuidPath(
		ctx context.Context,
		datacenter *object.Datacenter,
		datastoreURL string) (string, error)
}

// GetTopLevelCacheDir returns the top-level cache directory at the root of the
// datastore.
// If the datastore uses vSAN, this function also ensures the top-level
// directory exists.
func GetTopLevelCacheDir(
	ctx context.Context,
	client GetTopLevelCacheDirClient,
	dstDatacenter *object.Datacenter,
	dstDatastore *object.Datastore,
	dstDatastoreName, dstDatastoreURL string,
	topLevelDirectoryCreateSupported bool) (string, error) {

	if pkgutil.IsNil(ctx) {
		panic("context is nil")
	}
	if pkgutil.IsNil(client) {
		panic("client is nil")
	}
	if dstDatacenter == nil {
		panic("dstDatacenter is nil")
	}
	if dstDatastore == nil {
		panic("dstDatastore is nil")
	}
	if dstDatastoreName == "" {
		panic("dstDatastoreName is empty")
	}
	if dstDatastoreURL == "" {
		panic("dstDatastoreURL is empty")
	}

	logger := logr.FromContextOrDiscard(ctx).WithName("GetTopLevelCacheDir")

	logger.V(4).Info(
		"Args",
		"dstDatastoreName", dstDatastoreName,
		"dstDatastoreURL", dstDatastoreURL,
		"topLevelDirectoryCreateSupported", topLevelDirectoryCreateSupported)

	if topLevelDirectoryCreateSupported {
		return fmt.Sprintf(
			"[%s] %s", dstDatastoreName, topLevelCacheDirName), nil
	}

	// TODO(akutz) Figure out a way to test if the directory already exists
	//             instead of trying to just create it again and using the
	//             FileAlreadyExists error as signal.

	dstDatastorePath, _ := strings.CutPrefix(dstDatastoreURL, "ds://")
	topLevelCacheDirPath := path.Join(dstDatastorePath, topLevelCacheDirName)

	logger.V(4).Info(
		"CreateDirectory",
		"dstDatastorePath", dstDatastorePath,
		"topLevelCacheDirPath", topLevelCacheDirPath)

	uuidTopLevelCacheDirPath, err := client.CreateDirectory(
		ctx,
		dstDatastore,
		topLevelCacheDirPath,
		"")
	if err != nil {
		if !fault.Is(err, &vimtypes.FileAlreadyExists{}) {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}

		logger.V(4).Info(
			"ConvertNamespacePathToUuidPath",
			"dstDatacenter", dstDatacenter.Reference().Value,
			"topLevelCacheDirPath", topLevelCacheDirPath)

		uuidTopLevelCacheDirPath, err = client.ConvertNamespacePathToUuidPath(
			ctx,
			dstDatacenter,
			topLevelCacheDirPath)
		if err != nil {
			return "", fmt.Errorf(
				"failed to convert namespace path=%q: %w",
				topLevelCacheDirPath, err)
		}
	}

	logger.V(4).Info(
		"Got absolute top level cache dir path",
		"uuidTopLevelCacheDirPath", uuidTopLevelCacheDirPath)

	topLevelCacheDirName := path.Base(uuidTopLevelCacheDirPath)

	return fmt.Sprintf("[%s] %s", dstDatastoreName, topLevelCacheDirName), nil
}

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
	h := sha1.New() //nolint:gosec // used for creating directory name
	_, _ = io.WriteString(h, vmdkFileName)
	return fmt.Sprintf("%x", h.Sum(nil))[0:17]
}
