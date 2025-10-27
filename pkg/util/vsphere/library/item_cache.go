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

	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	pkgnil "github.com/vmware-tanzu/vm-operator/pkg/util/nil"
)

// SourceFile refers to a file that is to be copied, including information about
// what the destination file.
type SourceFile struct {
	Path    string
	VDiskID string

	DstDir        string
	DstProfileID  string
	DstDiskFormat vimtypes.DatastoreSectorFormat

	// TODO(akutz) In the future there may be additional information about the
	//             disk, such as its sector format (512 vs 4k), is encrypted,
	//             thin-provisioned, adapter type, etc.
}

// CachedFile refers to file that has been cached.
type CachedFile struct {
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

// CacheStorageURIs copies the file(s) from srcFiles to dstDir and returns the
// the information about the copied file(s).
func CacheStorageURIs(
	ctx context.Context,
	client CacheStorageURIsClient,
	dstDatacenter, srcDatacenter *object.Datacenter,
	srcFiles ...SourceFile) ([]CachedFile, error) {

	if pkgnil.IsNil(ctx) {
		panic("context is nil")
	}
	if pkgnil.IsNil(client) {
		panic("client is nil")
	}
	if dstDatacenter == nil {
		panic("dstDatacenter is nil")
	}
	if srcDatacenter == nil {
		panic("srcDatacenter is nil")
	}

	cachedFiles := make([]CachedFile, len(srcFiles))

	for i := range srcFiles {
		dstFilePath, err := copyFile(
			ctx,
			client,
			srcFiles[i],
			dstDatacenter,
			srcDatacenter)
		if err != nil {
			return nil, err
		}
		cachedFiles[i].Path = dstFilePath
	}

	return cachedFiles, nil
}

func copyFile(
	ctx context.Context,
	client CacheStorageURIsClient,
	srcFile SourceFile,
	dstDatacenter, srcDatacenter *object.Datacenter) (string, error) {

	var (
		srcFileName = path.Base(srcFile.Path)
		srcFileExt  = path.Ext(srcFile.Path)
		dstFileName = GetCachedFileName(srcFileName) + srcFileExt
		dstFilePath = path.Join(srcFile.DstDir, dstFileName)
		isDisk      = strings.EqualFold(".vmdk", srcFileExt)
		logger      = pkglog.FromContextOrDefault(ctx)
	)

	logger = logger.WithValues(
		"srcFilePath", srcFile.Path,
		"dstFilePath", dstFilePath,
		"dstDatacenter", dstDatacenter.Reference().Value,
		"srcDatacenter", srcDatacenter.Reference().Value)
	if isDisk {
		logger = logger.WithValues("dstDiskFormat", srcFile.DstDiskFormat)
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
		srcFile.DstDir,
		dstDatacenter,
		true); err != nil {

		return "", fmt.Errorf("failed to create folder %q: %w",
			srcFile.DstDir, err)
	}

	var (
		copyTask *object.Task
		err      error
	)

	if isDisk {
		logger.Info("Caching disk")
		copyTask, err = client.CopyVirtualDisk(
			ctx,
			srcFile.Path,
			srcDatacenter,
			dstFilePath,
			dstDatacenter,
			&vimtypes.FileBackedVirtualDiskSpec{
				VirtualDiskSpec: vimtypes.VirtualDiskSpec{
					AdapterType: string(vimtypes.VirtualDiskAdapterTypeLsiLogic),
					DiskType:    string(vimtypes.VirtualDiskTypeThin),
				},
				SectorFormat: string(srcFile.DstDiskFormat),
				Profile: []vimtypes.BaseVirtualMachineProfileSpec{
					&vimtypes.VirtualMachineDefinedProfileSpec{
						ProfileId: srcFile.DstProfileID,
					},
				},
			},
			false)
	} else {
		logger.Info("Caching non-disk file")
		copyTask, err = client.CopyDatastoreFile(
			ctx,
			srcFile.Path,
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

// GetCacheDirectory returns the cache directory for a library item
// based on its name, version, and intended storage profile.
func GetCacheDirectory(
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
