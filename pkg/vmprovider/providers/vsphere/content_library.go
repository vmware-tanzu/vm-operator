// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"io"
	"net/url"
	"strings"

	"github.com/vmware/govmomi/ovf"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25/soap"
)

type ContentLibraryProvider struct {
	session *Session
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

// ParseAndRetrievePropsFromLibraryItem downloads the supported file from content library.
// parses the downloaded ovf and retrieves the properties defined under
// VirtualSystem.Product.Property and return them as a map.
func (cs *ContentLibraryProvider) ParseAndRetrievePropsFromLibraryItem(ctx context.Context, item library.Item) (map[string]string, error) {
	var downloadedFileContent io.ReadCloser
	s := cs.session

	// download ovf from the library item
	err := s.WithRestClient(ctx, func(c *rest.Client) error {
		fileUri, downloadSessionId, err := cs.GenerateDownloadUriForLibraryItem(ctx, item, c)
		if err != nil {
			return err
		}

		if downloadSessionId != "" {
			defer deleteLibraryItemDownloadSession(c, ctx, downloadSessionId)
		}

		if fileUri != "" {
			//read the file as string once it is prepared for download
			downloadedFileContent, err = ReadFileFromUrl(ctx, c, fileUri)
			if err != nil {
				return err
			}
		}

		log.Info("downloaded library item", libItemName, item.Name)

		return nil

	})

	if downloadedFileContent != nil {
		defer downloadedFileContent.Close()
	}

	if err != nil {
		log.Error(err, "error occurred when downloading file from library item", libItemName, item.Name)
		return nil, err
	}

	ovfProperties, err := ParseOvfAndFetchProperties(downloadedFileContent)
	if err != nil {
		return nil, err
	}

	return ovfProperties, nil

}

func ParseOvfAndFetchProperties(fileContent io.ReadCloser) (map[string]string, error) {
	var env *ovf.Envelope
	properties := make(map[string]string)
	defer fileContent.Close()

	env, err := ovf.Unmarshal(fileContent)
	if err != nil {
		return nil, err
	}

	for _, product := range env.VirtualSystem.Product {
		for _, prop := range product.Property {
			if strings.HasPrefix(prop.Key, "vmware-system") {
				properties[prop.Key] = *prop.Default
			}
		}
	}
	return properties, nil
}

// GenerateDownloadUriForLibraryItem downloads the file from content library in 4 stages
// 1. create a download session
// 2. list the available files and downloads only the ovf files based on filename suffix
// 3. prepare the download session and fetch the url to be used for download
// 4. download the file
func (cs *ContentLibraryProvider) GenerateDownloadUriForLibraryItem(ctx context.Context, item library.Item, client *rest.Client) (string, string, error) {

	var fileDownloadUri string
	var fileToDownload string
	var downloadSessionId string

	s := cs.session

	err := s.WithRestClient(ctx, func(c *rest.Client) error {

		libManager := library.NewManager(c)
		var info *library.DownloadFile

		// create a download session for the file referred to by item id.
		session, err := libManager.CreateLibraryItemDownloadSession(ctx, library.Session{
			LibraryItemID: item.ID,
		})
		if err != nil {
			return err
		}

		downloadSessionId = session

		//list the files available for download in the library item
		files, err := libManager.ListLibraryItemDownloadSessionFile(ctx, session)
		if err != nil {
			return err
		}

		for _, file := range files {
			log.Info("Library Item file ", libFileName, file.Name)
			fileNameParts := strings.Split(file.Name, ".")
			if IsSupportedDeployType(fileNameParts[len(fileNameParts)-1]) {
				fileToDownload = file.Name
				break
			}
		}
		log.Info("download session created", libFileName, fileToDownload, libItemId, item.ID, libSessionId, session)

		_, err = libManager.PrepareLibraryItemDownloadSessionFile(ctx, session, fileToDownload)
		if err != nil {
			return err
		}

		log.Info("request posted to prepare file", libFileName, fileToDownload, libSessionId, session)

		info, err = libManager.GetLibraryItemDownloadSessionFile(ctx, session, fileToDownload)
		if err != nil {
			return err
		}

		if info.Status == "ERROR" {
			return errors.Errorf("Error occurred preparing file for download %v",
				info.ErrorMessage)
		}

		if info.Status == "PREPARED" {
			log.Info("Download file", libFileDownloadUrl, info.DownloadEndpoint.URI)
			fileDownloadUri = info.DownloadEndpoint.URI
		}

		return nil

	})

	return fileDownloadUri, downloadSessionId, err

}

func ReadFileFromUrl(ctx context.Context, client *rest.Client, fileUri string) (io.ReadCloser, error) {

	p := soap.DefaultDownload

	src, err := url.Parse(fileUri)
	if err != nil {
		return nil, err
	}

	readerStream, _, err := client.Download(ctx, src, &p)
	if err != nil {
		log.Error(err, "Error occurred when downloading file", libFileDownloadUrl, src, "fileuri", fileUri)
		return nil, err
	}

	return readerStream, nil
}

func deleteLibraryItemDownloadSession(c *rest.Client, ctx context.Context, session string) {
	sessionError := library.NewManager(c).DeleteLibraryItemDownloadSession(ctx, session)
	if sessionError != nil {
		log.Error(sessionError, "Error occurred when deleting download session", libSessionId, session)
	}
}
