// Copyright (c) 2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vapi/library"
	vapirest "github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
)

// VerifyContentLibraryItemDiskHasNullProvider downloads the VMDK descriptor
// from the named content library item and asserts it contains "VMWARE-NULL" as
// the key provider ID. vSphere writes this pseudo-provider when an encrypted VM
// is published to a content library — the real key is stripped on publish.
func VerifyContentLibraryItemDiskHasNullProvider(
	ctx context.Context,
	vimClient *vim25.Client,
	libraryID, itemName string,
) {
	restClient := vapirest.NewClient(vimClient)
	Expect(restClient.Login(ctx, url.UserPassword(testbed.AdminUsername, testbed.AdminPassword))).
		To(Succeed(), "failed to login to REST API for CL item inspection")
	defer func() { _ = restClient.Logout(ctx) }()

	mgr := library.NewManager(restClient)

	ids, err := mgr.FindLibraryItems(ctx, library.FindItem{
		LibraryID: libraryID,
		Name:      itemName,
	})
	Expect(err).NotTo(HaveOccurred(), "failed to find CL item %q in library %q", itemName, libraryID)
	Expect(ids).To(HaveLen(1), "expected exactly one item named %q in library %q", itemName, libraryID)

	sessionID, err := mgr.CreateLibraryItemDownloadSession(ctx, library.Session{
		LibraryItemID: ids[0],
	})
	Expect(err).NotTo(HaveOccurred(), "failed to create download session for CL item %q", itemName)
	defer func() { _ = mgr.DeleteLibraryItemDownloadSession(ctx, sessionID) }()

	files, err := mgr.ListLibraryItemDownloadSessionFile(ctx, sessionID)
	Expect(err).NotTo(HaveOccurred(), "failed to list files in download session for CL item %q", itemName)

	// The VMDK descriptor is the small text file (.vmdk without -flat in the name).
	var descriptorName string
	for _, f := range files {
		if strings.HasSuffix(f.Name, ".vmdk") && !strings.Contains(f.Name, "-flat") {
			descriptorName = f.Name
			break
		}
	}
	Expect(descriptorName).NotTo(BeEmpty(), "no VMDK descriptor found in CL item %q (files: %v)", itemName, files)

	_, err = mgr.PrepareLibraryItemDownloadSessionFile(ctx, sessionID, descriptorName)
	Expect(err).NotTo(HaveOccurred(), "failed to prepare download of %q", descriptorName)

	var downloadURI string
	Eventually(func(g Gomega) {
		f, err := mgr.GetLibraryItemDownloadSessionFile(ctx, sessionID, descriptorName)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(f.Status).To(Equal("PREPARED"),
			"waiting for %q to be PREPARED, got %q", descriptorName, f.Status)
		g.Expect(f.DownloadEndpoint).NotTo(BeNil())
		downloadURI = f.DownloadEndpoint.URI
	}, 30*time.Second, 2*time.Second).Should(Succeed(),
		"timed out waiting for CL item file %q to be PREPARED for download", descriptorName)

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
	resp, err := httpClient.Get(downloadURI)
	Expect(err).NotTo(HaveOccurred(), "failed to download VMDK descriptor from %q", downloadURI)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred(), "failed to read VMDK descriptor body")

	Expect(string(body)).To(ContainSubstring("VMWARE-NULL"),
		"expected VMDK descriptor of CL item %q to contain VMWARE-NULL key provider ID "+
			"(vSphere strips the real key when publishing an encrypted VM to a content library)",
		itemName)
}
