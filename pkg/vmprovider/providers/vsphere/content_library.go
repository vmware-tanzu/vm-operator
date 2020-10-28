// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25/soap"
)

type ContentLibraryProvider struct {
	session *Session
}

//go:generate mockgen -destination=./mocks/mock_content_library.go -package=mocks github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere ContentDownloadHandler

type ContentDownloadHandler interface {
	GenerateDownloadUriForLibraryItem(ctx context.Context, restClient *rest.Client, item *library.Item) (DownloadUriResponse, error)
}

type ContentDownloadProvider struct {
	ApiWaitTimeSecs int
}

type TimerTaskResponse struct {
	TaskDone bool
	Err      error
}

const (
	libItemName        = "itemName"
	libFileName        = "fileName"
	libItemId          = "itemId"
	libSessionId       = "sessionId"
	libFileDownloadUrl = "fileUrl"
)

func NewContentLibraryProvider(ses *Session) *ContentLibraryProvider {
	contentLibProvider := &ContentLibraryProvider{
		session: ses,
	}
	return contentLibProvider
}

type DownloadUriResponse struct {
	FileUri           string
	DownloadSessionId string
}

func ParseOvf(ovfContent io.Reader) (*ovf.Envelope, error) {
	ovfEnvelope, err := ovf.Unmarshal(ovfContent)
	if err != nil {
		return nil, err
	}

	return ovfEnvelope, nil
}

// RetrieveOvfEnvelopeFromLibraryItem downloads the supported file from content library.
// parses the downloaded ovf and returns the OVF Envelope descriptor for consumption.
func (cs *ContentLibraryProvider) RetrieveOvfEnvelopeFromLibraryItem(ctx context.Context, item *library.Item, clHandler ContentDownloadHandler) (*ovf.Envelope, error) {

	restClient := cs.session.Client.RestClient()
	// download ovf from the library item
	response, err := clHandler.GenerateDownloadUriForLibraryItem(ctx, restClient, item)

	// GenerateDownloadUriForLibraryItem may return error even in case when DownloadSession
	// was created successfully. We need to close DownloadSession in this case.
	defer deleteLibraryItemDownloadSession(restClient, ctx, response.DownloadSessionId)
	if err != nil {
		return nil, err
	}

	if isInvalidResponse(response) {
		return nil, errors.Errorf("error occurred downloading item %v", item.Name)
	}

	// Read the file as string once it is prepared for download
	downloadedFileContent, err := ReadFileFromUrl(ctx, restClient, response.FileUri)
	if err != nil || downloadedFileContent == nil {
		log.Error(err, "error occurred when downloading file from library item", libItemName, item.Name)
		return nil, err
	}

	log.V(4).Info("downloaded library item", libItemName, item.Name)
	defer downloadedFileContent.Close()

	return ParseOvf(downloadedFileContent)
}

// Only used in testing.
func (cs *ContentLibraryProvider) CreateLibrary(ctx context.Context, contentSource string) (string, error) {
	log.Info("Creating Library", "library", contentSource)

	lib := library.Library{
		Name: contentSource,
		Type: "LOCAL",
		Storage: []library.StorageBackings{
			{
				DatastoreID: cs.session.datastore.Reference().Value,
				Type:        "DATASTORE",
			},
		},
	}

	restClient := cs.session.Client.RestClient()
	libID, err := library.NewManager(restClient).CreateLibrary(ctx, lib)
	if err != nil || libID == "" {
		log.Error(err, "failed to create library")
		return "", err
	}

	return libID, nil
}

func (cs *ContentLibraryProvider) CreateLibraryItem(ctx context.Context, libraryItem library.Item, path string) error {
	log.Info("Creating Library Item", "libraryItem", libraryItem, "path", path)

	restClient := cs.session.Client.RestClient()

	libMgr := library.NewManager(restClient)
	itemID, err := libMgr.CreateLibraryItem(ctx, libraryItem)
	if err != nil {
		return err
	}

	itemUpdateSessionId, err := libMgr.CreateLibraryItemUpdateSession(ctx, library.Session{LibraryItemID: itemID})
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

		name := filepath.Base(path)
		size := fi.Size()
		info := library.UpdateFile{
			Name:       name,
			SourceType: "PUSH",
			Size:       size,
		}

		update, err := libMgr.AddLibraryItemFile(ctx, itemUpdateSessionId, info)
		if err != nil {
			return err
		}

		p := soap.DefaultUpload
		p.ContentLength = size

		u, err := url.Parse(update.UploadEndpoint.URI)
		if err != nil {
			return err
		}

		return c.Upload(ctx, f, u, &p)
	}

	if err = uploadFunc(restClient, path); err != nil {
		return err
	}

	return libMgr.CompleteLibraryItemUpdateSession(ctx, itemUpdateSessionId)
}

// DownloadUriResponse considered to be not valid if either DownloadSessionId or
// FileUri are not set
func isInvalidResponse(response DownloadUriResponse) bool {
	if response.DownloadSessionId == "" || response.FileUri == "" {
		return true
	}

	return false
}

// GenerateDownloadUriForLibraryItem downloads the file from content library in 4 stages
// 1. create a download session
// 2. list the available files and downloads only the ovf files based on filename suffix
// 3. prepare the download session and fetch the url to be used for download
// 4. download the file
func (contentSession ContentDownloadProvider) GenerateDownloadUriForLibraryItem(ctx context.Context, c *rest.Client, item *library.Item) (DownloadUriResponse, error) {
	response := DownloadUriResponse{}

	libMgr := library.NewManager(c)

	// create a download session for the file referred to by item id.
	session, err := libMgr.CreateLibraryItemDownloadSession(ctx, library.Session{LibraryItemID: item.ID})
	if err != nil {
		return response, err
	}

	log.V(4).Info("download session created", libItemId, item.ID, libSessionId, session)
	response.DownloadSessionId = session

	// list the files available for download in the library item
	files, err := libMgr.ListLibraryItemDownloadSessionFile(ctx, session)
	if err != nil {
		return response, err
	}

	var fileDownloadUri string
	var fileToDownload string

	for _, file := range files {
		log.V(4).Info("Library Item file ", libFileName, file.Name)
		fileNameParts := strings.Split(file.Name, ".")
		if IsSupportedDeployType(fileNameParts[len(fileNameParts)-1]) {
			fileToDownload = file.Name
			break
		}
	}

	if fileToDownload == "" {
		return response, errors.Errorf("No files with supported deploy type are available for download for %s", item.ID)
	}

	_, err = libMgr.PrepareLibraryItemDownloadSessionFile(ctx, session, fileToDownload)
	if err != nil {
		return response, err
	}

	log.V(4).Info("request posted to prepare file", libFileName, fileToDownload, libSessionId, session)

	// content library api to prepare a file for download guarantees eventual end state of either ERROR or PREPARED
	// in order to avoid posting too many requests to the api we are setting a sleep of 'n' seconds between each retry
	work := func(ctx context.Context) (TimerTaskResponse, error) {
		downloadSessResp, err := libMgr.GetLibraryItemDownloadSession(ctx, session)
		if err != nil {
			return TimerTaskResponse{}, err
		}

		if downloadSessResp.ErrorMessage != nil {
			return TimerTaskResponse{}, downloadSessResp.ErrorMessage
		}

		info, err := libMgr.GetLibraryItemDownloadSessionFile(ctx, session, fileToDownload)
		if err != nil {
			return TimerTaskResponse{}, err
		}

		if info.Status == "ERROR" {
			return TimerTaskResponse{}, errors.Errorf("Error occurred preparing file for download %v",
				info.ErrorMessage)
		}

		if info.Status == "PREPARED" {
			fileDownloadUri = info.DownloadEndpoint.URI
			log.V(4).Info("Download file", libFileDownloadUrl, fileDownloadUri)
			return TimerTaskResponse{
				TaskDone: true,
			}, nil
		}

		return TimerTaskResponse{}, nil
	}

	delay := time.Duration(contentSession.ApiWaitTimeSecs) * time.Second

	err = RunTaskAtInterval(ctx, delay, work)
	if err != nil {
		return response, err
	}

	response.FileUri = fileDownloadUri

	return response, nil
}

// RunTaskAtInterval calls the work function at an period interval until either theTimerTaskResponse returned with
// the value of TaskDone true or an error returned
func RunTaskAtInterval(ctx context.Context, checkDelay time.Duration, work func(ctx context.Context) (TimerTaskResponse, error)) error {
	for {
		routineResponse, err := work(ctx)
		if err != nil {
			return err
		} else if routineResponse.TaskDone {
			return nil
		} else {
			time.Sleep(checkDelay)
		}
	}
}

func ReadFileFromUrl(ctx context.Context, c *rest.Client, fileUri string) (io.ReadCloser, error) {
	src, err := url.Parse(fileUri)
	if err != nil {
		return nil, err
	}

	p := soap.DefaultDownload
	readerStream, _, err := c.Download(ctx, src, &p)
	if err != nil {
		log.Error(err, "Error occurred when downloading file", "source", src, "fileURI", fileUri)
		return nil, err
	}

	return readerStream, nil
}

func deleteLibraryItemDownloadSession(c *rest.Client, ctx context.Context, sessionId string) {
	if sessionId != "" {
		if err := library.NewManager(c).DeleteLibraryItemDownloadSession(ctx, sessionId); err != nil {
			log.Error(err, "Error occurred when deleting download session", libSessionId, sessionId)
		}
	}
}
