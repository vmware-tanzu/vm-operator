// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25/soap"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

type Provider interface {
	GetLibraryItems(ctx context.Context, clUUID string) ([]library.Item, error)
	GetLibraryItem(ctx context.Context, libraryUUID, itemName string,
		notFoundReturnErr bool) (*library.Item, error)
	GetLibraryItemID(ctx context.Context, itemUUID string) (*library.Item, error)
	ListLibraryItems(ctx context.Context, libraryUUID string) ([]string, error)
	UpdateLibraryItem(ctx context.Context, itemID, newName string, newDescription *string) error
	RetrieveOvfEnvelopeFromLibraryItem(ctx context.Context, item *library.Item) (*ovf.Envelope, error)
	RetrieveOvfEnvelopeByLibraryItemID(ctx context.Context, itemID string) (*ovf.Envelope, error)

	// TODO: Testing only. Remove these from this file.
	CreateLibraryItem(ctx context.Context, libraryItem library.Item, path string) error
}

type provider struct {
	libMgr        *library.Manager
	retryInterval time.Duration
}

const (
	DefaultContentLibAPIWaitSecs = 5
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

func NewProvider(ctx context.Context, restClient *rest.Client) Provider {
	var waitSeconds int
	if w := pkgcfg.FromContext(ctx).ContentAPIWait; w > 0 {
		waitSeconds = int(w.Seconds())
	} else {
		waitSeconds = DefaultContentLibAPIWaitSecs
	}

	return NewProviderWithWaitSec(restClient, waitSeconds)
}

func NewProviderWithWaitSec(restClient *rest.Client, waitSeconds int) Provider {
	return &provider{
		libMgr:        library.NewManager(restClient),
		retryInterval: time.Duration(waitSeconds) * time.Second,
	}
}

func (cs *provider) ListLibraryItems(ctx context.Context, libraryUUID string) ([]string, error) {
	logger := log.WithValues("libraryUUID", libraryUUID)
	itemList, err := cs.libMgr.ListLibraryItems(ctx, libraryUUID)
	if err != nil {
		if util.IsNotFoundError(err) {
			logger.Error(err, "cannot list items from content library that does not exist")
			return nil, nil
		}
		return nil, err
	}
	return itemList, err
}

func (cs *provider) GetLibraryItems(ctx context.Context, libraryUUID string) ([]library.Item, error) {
	logger := log.WithValues("libraryUUID", libraryUUID)
	itemList, err := cs.libMgr.ListLibraryItems(ctx, libraryUUID)
	if err != nil {
		if util.IsNotFoundError(err) {
			logger.Error(err, "cannot list items from content library that does not exist")
			return nil, nil
		}
		return nil, err
	}

	// best effort to get content library items.
	resErrs := make([]error, 0)
	items := make([]library.Item, 0)
	for _, itemID := range itemList {
		item, err := cs.libMgr.GetLibraryItem(ctx, itemID)
		if err != nil {
			resErrs = append(resErrs, err)
			logger.Error(err, "get library item failed", "itemID", itemID)
			continue
		}
		items = append(items, *item)
	}

	return items, apierrorsutil.NewAggregate(resErrs)
}

func (cs *provider) GetLibraryItem(ctx context.Context, libraryUUID, itemName string,
	notFoundReturnErr bool) (*library.Item, error) {
	itemIDs, err := cs.libMgr.FindLibraryItems(ctx, library.FindItem{LibraryID: libraryUUID, Name: itemName})
	if err != nil {
		return nil, fmt.Errorf("failed to find image: %s: %w", itemName, err)
	}

	if len(itemIDs) == 0 {
		if notFoundReturnErr {
			return nil, fmt.Errorf("no library item named: %s", itemName)
		}
		return nil, nil
	}
	if len(itemIDs) != 1 {
		return nil, fmt.Errorf("multiple library items named: %s", itemName)
	}

	item, err := cs.libMgr.GetLibraryItem(ctx, itemIDs[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get library item: %s: %w", itemName, err)
	}

	return item, nil
}

func (cs *provider) GetLibraryItemID(ctx context.Context, itemUUID string) (*library.Item, error) {
	item, err := cs.libMgr.GetLibraryItem(ctx, itemUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to find image: %s: %w", itemUUID, err)
	}

	return item, nil
}

// RetrieveOvfEnvelopeByLibraryItemID retrieves the OVF Envelope by the given library item ID.
func (cs *provider) RetrieveOvfEnvelopeByLibraryItemID(ctx context.Context, itemID string) (*ovf.Envelope, error) {
	libItem, err := cs.libMgr.GetLibraryItem(ctx, itemID)
	if err != nil {
		return nil, err
	}

	if libItem == nil || libItem.Type != library.ItemTypeOVF {
		log.Error(nil, "empty or non OVF library item type, skipping", "itemID", itemID)
		// No need to return the error here to avoid unnecessary reconciliation.
		return nil, nil
	}

	return cs.RetrieveOvfEnvelopeFromLibraryItem(ctx, libItem)
}

func readerFromURL(ctx context.Context, c *rest.Client, url *url.URL) (io.ReadCloser, error) {
	p := soap.DefaultDownload
	readerStream, _, err := c.Download(ctx, url, &p)
	if err != nil {
		// Log message used by VMC LINT. Refer to before making changes
		log.Error(err, "Error occurred when downloading file", "url", url)
		return nil, err
	}

	return readerStream, nil
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

	downloadedFileContent, err := readerFromURL(ctx, cs.libMgr.Client, fileURL)
	if err != nil {
		logger.Error(err, "error downloading file from library item")
		return nil, err
	}

	logger.V(4).Info("downloaded library item")
	defer func() {
		_ = downloadedFileContent.Close()
	}()

	// OVF file is validated during upload, err here can be internet error.
	envelope, err := ovf.Unmarshal(downloadedFileContent)
	if err != nil {
		logger.Error(err, "error parsing the OVF envelope")
		return nil, err
	}

	return envelope, nil
}

// UpdateLibraryItem updates the content library item's name and description.
func (cs *provider) UpdateLibraryItem(ctx context.Context, itemID, newName string, newDescription *string) error {
	log.Info("Updating Library Item", "itemID", itemID,
		"newName", newName, "newDescription", newDescription)

	item, err := cs.libMgr.GetLibraryItem(ctx, itemID)
	if err != nil {
		log.Error(err, "error getting library item")
		return err
	}

	if newName != "" {
		item.Name = newName
	}
	if newDescription != nil {
		item.Description = newDescription
	}

	return cs.libMgr.UpdateLibraryItem(ctx, item)
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
		f, err := os.Open(filepath.Clean(path))
		if err != nil {
			return err
		}
		defer func() {
			_ = f.Close()
		}()

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

// generateDownloadURLForLibraryItem downloads the file from content library in 3 steps:
// 1. list the available files and downloads only the ovf files based on filename suffix
// 2. prepare the download session and fetch the url to be used for download
// 3. download the file.
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
		return nil, fmt.Errorf("no files with supported deploy type are available for download for %s", item.ID)
	}

	_, err = cs.libMgr.PrepareLibraryItemDownloadSessionFile(ctx, sessionID, fileToDownload)
	if err != nil {
		return nil, err
	}

	logger.V(4).Info("request posted to prepare file", "fileToDownload", fileToDownload)

	// Content library api to prepare a file for download guarantees eventual end state of either
	// ERROR or PREPARED in order to avoid posting too many requests to the api.
	var fileURL string
	err = wait.PollUntilContextCancel(ctx, cs.retryInterval, true, func(_ context.Context) (bool, error) {
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
			return false, fmt.Errorf("error occurred preparing file for download %v", info.ErrorMessage)
		}

		if info.Status != "PREPARED" {
			return false, nil
		}

		if info.DownloadEndpoint == nil {
			return false, fmt.Errorf("prepared file for download does not have endpoint")
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
