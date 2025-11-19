// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package datastore_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/find"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/datastore"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const localDS0 = "LocalDS_0"

var _ = Describe("GetDatastoreURLFromDatastorePath", func() {
	var (
		ctx    context.Context
		finder *find.Finder
	)

	BeforeEach(func() {
		vcsimCtx := builder.NewTestContextForVCSim(
			ctxop.WithContext(
				pkgcfg.NewContextWithDefaultConfig()),
			builder.VCSimTestConfig{})
		ctx = vcsimCtx
		ctx = vmconfig.WithContext(ctx)

		finder = find.NewFinder(vcsimCtx.VCClient.Client)
	})

	Context("GetDatastoreURLForStorageURI", func() {
		It("should panic when context is nil", func() {
			datastoreName := localDS0

			Expect(func() {
				ctx = nil
				_, _ = datastore.GetDatastoreURLFromDatastorePath(ctx, finder, datastoreName)
			}).To(PanicWith("ctx is nil"))
		})

		It("should panic when finder is nil", func() {
			datastoreName := localDS0

			Expect(func() {
				_, _ = datastore.GetDatastoreURLFromDatastorePath(ctx, nil, datastoreName)
			}).To(PanicWith("finder is nil"))
		})

		It("should return datastore URL for valid storage URI", func() {
			datastorePath := "[" + localDS0 + "] vm-name/vm-name.vmx"

			url, err := datastore.GetDatastoreURLFromDatastorePath(
				ctx, finder, datastorePath)

			Expect(err).ToNot(HaveOccurred())
			Expect(url).ToNot(BeEmpty())
			Expect(url).To(ContainSubstring(localDS0))
		})

		It("should return datastore URL for storage URI with just datastore name", func() {
			datastorePath := "[" + localDS0 + "]"

			url, err := datastore.GetDatastoreURLFromDatastorePath(
				ctx, finder, datastorePath)

			Expect(err).ToNot(HaveOccurred())
			Expect(url).ToNot(BeEmpty())
			Expect(url).To(ContainSubstring(localDS0))
		})

		It("should return datastore URL for storage URI with complex path", func() {
			datastorePath := "[" + localDS0 + "] complex/path/to/vm/vm-name.vmx"

			url, err := datastore.GetDatastoreURLFromDatastorePath(
				ctx, finder, datastorePath)

			Expect(err).ToNot(HaveOccurred())
			Expect(url).ToNot(BeEmpty())
			Expect(url).To(ContainSubstring(localDS0))
		})

		It("should return error for empty storage URI", func() {
			datastorePath := ""

			url, err := datastore.GetDatastoreURLFromDatastorePath(
				ctx, finder, datastorePath)

			Expect(err).To(MatchError(`failed to parse datastore path ""`))
			Expect(url).To(BeEmpty())
		})

		It("should return error for invalid storage URI format", func() {
			datastorePath := "invalid-uri-format"

			url, err := datastore.GetDatastoreURLFromDatastorePath(
				ctx, finder, datastorePath)

			Expect(err).To(MatchError(`failed to parse datastore path "invalid-uri-format"`))
			Expect(url).To(BeEmpty())
		})

		It("should return error for storage URI with empty datastore name", func() {
			datastorePath := "[] vm-name/vm-name.vmx"

			url, err := datastore.GetDatastoreURLFromDatastorePath(
				ctx, finder, datastorePath)

			Expect(err).To(MatchError(`failed to get datastore for "": datastore '' not found`))
			Expect(url).To(BeEmpty())
		})

		It("should return error for non-existent datastore", func() {
			datastorePath := "[NonExistentDS] vm-name/vm-name.vmx"

			url, err := datastore.GetDatastoreURLFromDatastorePath(
				ctx, finder, datastorePath)

			Expect(err).To(MatchError(`failed to get datastore for "NonExistentDS": datastore 'NonExistentDS' not found`))
			Expect(url).To(BeEmpty())
		})
	})

})
