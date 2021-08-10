// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25/soap"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
)

type Provider interface {
	GetLibraryItems(ctx context.Context, clUUID string) ([]library.Item, error)
	GetLibraryItem(ctx context.Context, clUUID, itemName string) (*library.Item, error)
	RetrieveOvfEnvelopeFromLibraryItem(ctx context.Context, item *library.Item) (*ovf.Envelope, error)

	// TODO: Testing only. Remove these from this file.
	CreateLibrary(ctx context.Context, contentSource, datastoreID string) (string, error)
	CreateLibraryItem(ctx context.Context, libraryItem library.Item, path string) error
	DeleteLibraryItem(ctx context.Context, libraryItem *library.Item) error

	VirtualMachineImageResourcesForLibrary(
		ctx context.Context,
		clUUID string,
		currentCLImages map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error)
}

type provider struct {
	libMgr        *library.Manager
	retryInterval time.Duration
}

const (
	// BMV: Investigate if setting this to 1 actually reduces the integration test time.
	EnvContentLibApiWaitSecs     = "CONTENT_API_WAIT_SECS"
	DefaultContentLibApiWaitSecs = 5
)

func IsSupportedDeployType(t string) bool {
	switch t {
	case library.ItemTypeVMTX, library.ItemTypeOVF:
		// Keep in sync with what cloneVMFromContentLibrary() handles.
		return true
	default:
		return false
	}
}

func NewProvider(restClient *rest.Client) Provider {
	waitSeconds, err := strconv.Atoi(os.Getenv(EnvContentLibApiWaitSecs))
	if err != nil || waitSeconds < 1 {
		waitSeconds = DefaultContentLibApiWaitSecs
	}

	return NewProviderWithWaitSec(restClient, waitSeconds)
}

func NewProviderWithWaitSec(restClient *rest.Client, waitSeconds int) Provider {
	return &provider{
		libMgr:        library.NewManager(restClient),
		retryInterval: time.Duration(waitSeconds) * time.Second,
	}
}

func (cs *provider) GetLibraryItems(ctx context.Context, libraryUUID string) ([]library.Item, error) {
	items, err := cs.libMgr.GetLibraryItems(ctx, libraryUUID)
	if err != nil {
		if lib.IsNotFoundError(err) {
			log.Error(err, "cannot list items from content library that does not exist", "libraryUUID", libraryUUID)
			return nil, nil
		}
		return nil, err
	}

	return items, nil
}

func (cs *provider) GetLibraryItem(ctx context.Context, libraryUUID, itemName string) (*library.Item, error) {
	itemIDs, err := cs.libMgr.FindLibraryItems(ctx, library.FindItem{LibraryID: libraryUUID, Name: itemName})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find image: %s", itemName)
	}

	if len(itemIDs) == 0 {
		return nil, errors.Errorf("no library item named: %s", itemName)
	}
	if len(itemIDs) != 1 {
		return nil, errors.Errorf("multiple library items named: %s", itemName)
	}

	item, err := cs.libMgr.GetLibraryItem(ctx, itemIDs[0])
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get library item: %s", itemName)
	}

	return item, nil
}

// RetrieveOvfEnvelopeFromLibraryItem downloads the supported file from content library.
// parses the downloaded ovf and returns the OVF Envelope descriptor for consumption.
func (cs *provider) RetrieveOvfEnvelopeFromLibraryItem(ctx context.Context, item *library.Item) (*ovf.Envelope, error) {
	// Create a download session for the file referred to by item id.
	sessionID, err := cs.libMgr.CreateLibraryItemDownloadSession(ctx, library.Session{LibraryItemID: item.ID})
	if err != nil {
		return nil, err
	}

	logger := log.WithValues("sessionID", sessionID, "itemID", item.ID, "itemName", item.Name)
	logger.V(4).Info("download session for item created")

	defer func() {
		if err := cs.libMgr.DeleteLibraryItemDownloadSession(ctx, sessionID); err != nil {
			logger.Error(err, "Error deleting download session")
		}
	}()

	// Download ovf from the library item.
	fileURL, err := cs.generateDownloadURLForLibraryItem(ctx, logger, sessionID, item)
	if err != nil {
		return nil, err
	}

	downloadedFileContent, err := readerFromUrl(ctx, cs.libMgr.Client, fileURL)
	if err != nil {
		logger.Error(err, "error downloading file from library item")
		return nil, err
	}

	logger.V(4).Info("downloaded library item")
	defer downloadedFileContent.Close()

	envelope, err := ovf.Unmarshal(downloadedFileContent)
	if err != nil {
		logger.Error(err, "error parsing the OVF envelope")
		return nil, nil
	}

	return envelope, nil
}

// Only used in testing.
func (cs *provider) CreateLibrary(ctx context.Context, name, datastoreID string) (string, error) {
	log.Info("Creating Library", "libraryName", name)

	lib := library.Library{
		Name: name,
		Type: "LOCAL",
		Storage: []library.StorageBackings{
			{
				DatastoreID: datastoreID,
				Type:        "DATASTORE",
			},
		},
	}

	libID, err := cs.libMgr.CreateLibrary(ctx, lib)
	if err != nil || libID == "" {
		log.Error(err, "failed to create library")
		return "", err
	}

	return libID, nil
}

// Only used in testing.
func (cs *provider) DeleteLibraryItem(ctx context.Context, libraryItem *library.Item) error {
	return cs.libMgr.DeleteLibraryItem(ctx, libraryItem)
}

// Only used in testing.
func (cs *provider) CreateLibraryItem(ctx context.Context, libraryItem library.Item, path string) error {
	log.Info("Creating Library Item", "item", libraryItem, "path", path)

	itemID, err := cs.libMgr.CreateLibraryItem(ctx, libraryItem)
	if err != nil {
		return err
	}

	sessionID, err := cs.libMgr.CreateLibraryItemUpdateSession(ctx, library.Session{LibraryItemID: itemID})
	if err != nil {
		return err
	}

	// Update Library item with library file "ovf"
	uploadFunc := func(c *rest.Client, path string) error {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			return err
		}

		info := library.UpdateFile{
			Name:       filepath.Base(path),
			SourceType: "PUSH",
			Size:       fi.Size(),
		}

		update, err := cs.libMgr.AddLibraryItemFile(ctx, sessionID, info)
		if err != nil {
			return err
		}

		u, err := url.Parse(update.UploadEndpoint.URI)
		if err != nil {
			return err
		}

		p := soap.DefaultUpload
		p.ContentLength = info.Size

		return c.Upload(ctx, f, u, &p)
	}

	if err = uploadFunc(cs.libMgr.Client, path); err != nil {
		return err
	}

	return cs.libMgr.CompleteLibraryItemUpdateSession(ctx, sessionID)
}

// Lists all the VirtualMachineImages from a CL by a given UUID.
func (cs *provider) VirtualMachineImageResourcesForLibrary(
	ctx context.Context,
	clUUID string,
	currentCLImages map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error) {

	log.V(4).Info("Listing VirtualMachineImages from ContentLibrary", "contentLibraryUUID", clUUID)

	items, err := cs.GetLibraryItems(ctx, clUUID)
	if err != nil {
		return nil, err
	}

	var images []*v1alpha1.VirtualMachineImage
	for i := range items {
		var ovfEnvelope *ovf.Envelope
		item := items[i]

		if curImage, ok := currentCLImages[item.ID]; ok {
			// If there is already an VMImage for this item, and it is the same - as determined by _just_ the
			// annotation - reuse the existing VMImage. This is to avoid repeated CL fetch tasks that would
			// otherwise be created, spamming the UI. It would be nice if CL provided an external API that
			// allowed us to silently fetch the OVF.
			annotations := curImage.GetAnnotations()
			if ver := annotations[constants.VMImageCLVersionAnnotation]; ver == libItemVersionAnnotation(&item) {
				images = append(images, &curImage)
				continue
			}
		}

		switch item.Type {
		case library.ItemTypeOVF:
			if ovfEnvelope, err = cs.RetrieveOvfEnvelopeFromLibraryItem(ctx, &item); err != nil {
				log.Error(err, "error extracting the OVF envelope from the library item", "itemName", item.Name)
				return nil, err
			}
			if ovfEnvelope == nil {
				log.Error(err, "no valid OVF envelope found, skipping library item", "itemName", item.Name)
				continue
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

// generateDownloadURLForLibraryItem downloads the file from content library in 3 steps:
// 1. list the available files and downloads only the ovf files based on filename suffix
// 2. prepare the download session and fetch the url to be used for download
// 3. download the file
func (cs *provider) generateDownloadURLForLibraryItem(
	ctx context.Context,
	logger logr.Logger,
	sessionID string,
	item *library.Item) (*url.URL, error) {

	// List the files available for download in the library item.
	files, err := cs.libMgr.ListLibraryItemDownloadSessionFile(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	var fileToDownload string
	for _, file := range files {
		logger.V(4).Info("Library Item file", "fileName", file.Name)
		if ext := filepath.Ext(file.Name); ext != "" && IsSupportedDeployType(ext[1:]) {
			fileToDownload = file.Name
			break
		}
	}
	if fileToDownload == "" {
		return nil, errors.Errorf("No files with supported deploy type are available for download for %s", item.ID)
	}

	_, err = cs.libMgr.PrepareLibraryItemDownloadSessionFile(ctx, sessionID, fileToDownload)
	if err != nil {
		return nil, err
	}

	logger.V(4).Info("request posted to prepare file", "fileToDownload", fileToDownload)

	// Content library api to prepare a file for download guarantees eventual end state of either
	// ERROR or PREPARED in order to avoid posting too many requests to the api.
	var fileURL string
	err = wait.PollImmediateInfinite(cs.retryInterval, func() (bool, error) {
		downloadSessResp, err := cs.libMgr.GetLibraryItemDownloadSession(ctx, sessionID)
		if err != nil {
			return false, err
		}

		if downloadSessResp.ErrorMessage != nil {
			return false, downloadSessResp.ErrorMessage
		}

		info, err := cs.libMgr.GetLibraryItemDownloadSessionFile(ctx, sessionID, fileToDownload)
		if err != nil {
			return false, err
		}

		if info.Status == "ERROR" {
			// Log message used by VMC LINT. Refer to before making changes
			return false, errors.Errorf("Error occurred preparing file for download %v", info.ErrorMessage)
		}

		if info.Status != "PREPARED" {
			return false, nil
		}

		if info.DownloadEndpoint == nil {
			return false, errors.Errorf("Prepared file for download does not have endpoint")
		}

		fileURL = info.DownloadEndpoint.URI
		log.V(4).Info("Downloaded file", "fileURL", fileURL)
		return true, nil
	})

	if err != nil {
		return nil, err
	}

	return url.Parse(fileURL)
}
