// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package library_test

import (
	"context"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	clsutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/library"
)

type nilContextKey uint8

var nilContext = context.WithValue(context.Background(), nilContextKey(0), "nil")

type fakeCacheStorageURIsClient struct {
	queryErr    error
	queryResult string
	queryCalls  int32

	copyErr    error
	copyResult *object.Task
	copyCalls  int32

	makeErr   error
	makeCalls int32

	waitErr   error
	waitCalls int32
}

func (m *fakeCacheStorageURIsClient) QueryVirtualDiskUuid( //nolint:revive,stylecheck
	ctx context.Context,
	name string,
	datacenter *object.Datacenter) (string, error) {

	_ = atomic.AddInt32(&m.queryCalls, 1)
	return m.queryResult, m.queryErr
}

func (m *fakeCacheStorageURIsClient) CopyVirtualDisk(
	ctx context.Context,
	srcName string, srcDatacenter *object.Datacenter,
	dstName string, dstDatacenter *object.Datacenter,
	dstSpec vimtypes.BaseVirtualDiskSpec, force bool) (*object.Task, error) {

	_ = atomic.AddInt32(&m.copyCalls, 1)
	return m.copyResult, m.copyErr
}

func (m *fakeCacheStorageURIsClient) MakeDirectory(
	ctx context.Context,
	name string,
	datacenter *object.Datacenter,
	createParentDirectories bool) error {

	_ = atomic.AddInt32(&m.makeCalls, 1)
	return m.makeErr
}

func (m *fakeCacheStorageURIsClient) WaitForTask(
	ctx context.Context, task *object.Task) error {

	_ = atomic.AddInt32(&m.waitCalls, 1)
	return m.waitErr
}

var _ = Describe("CacheStorageURIs", func() {

	var (
		ctx    context.Context
		client *fakeCacheStorageURIsClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = &fakeCacheStorageURIsClient{}

		Expect(ctx).ToNot(BeNil())
		Expect(client).ToNot(BeNil())
	})

	var _ = DescribeTable("it should panic",
		func(
			ctx context.Context,
			client clsutil.CacheStorageURIsClient,
			dstDatacenter, srcDatacenter *object.Datacenter,
			dstDir string,
			expPanic string) {

			if ctx == nilContext {
				ctx = nil
			}

			f := func() {
				_, _ = clsutil.CacheStorageURIs(
					ctx,
					client,
					dstDatacenter,
					srcDatacenter,
					dstDir)
			}

			Expect(f).To(PanicWith(expPanic))
		},

		Entry(
			"nil ctx",
			nilContext,
			&fakeCacheStorageURIsClient{},
			&object.Datacenter{},
			&object.Datacenter{},
			"[my-datastore] .contentlib-cache/123/v1",
			"context is nil",
		),
		Entry(
			"nil client",
			context.Background(),
			nil,
			&object.Datacenter{},
			&object.Datacenter{},
			"[my-datastore] .contentlib-cache/123/v1",
			"client is nil",
		),
		Entry(
			"nil dstDatacenter",
			context.Background(),
			&fakeCacheStorageURIsClient{},
			nil,
			&object.Datacenter{},
			"[my-datastore] .contentlib-cache/123/v1",
			"dstDatacenter is nil",
		),
		Entry(
			"nil srcDatacenter",
			context.Background(),
			&fakeCacheStorageURIsClient{},
			&object.Datacenter{},
			nil,
			"[my-datastore] .contentlib-cache/123/v1",
			"srcDatacenter is nil",
		),
		Entry(
			"empty dstDir",
			context.Background(),
			&fakeCacheStorageURIsClient{},
			&object.Datacenter{},
			&object.Datacenter{},
			"",
			"dstDir is empty",
		),
	)

	When("it should not panic", func() {

		const (
			srcDatastoreName                = "my-datastore-2"
			srcContentLibID                 = "4c502fa7-7ac6-45b9-bd31-15918a193026"
			srcContentLibItemID             = "d9c5e5fa-5f41-4c80-bd11-a70716f860ac"
			srcContentLibItemContentVersion = "v1"
			srcContentLibItemPath           = "[" + srcDatastoreName + "] contentlib/" +
				srcContentLibID + "/" + srcContentLibItemID

			dstDatastoreName = "my-datastore-1"
			dstDir           = "[" + dstDatastoreName + "] .contentlib-cache/" +
				srcContentLibItemID + "/" + srcContentLibItemContentVersion
		)

		var (
			ctx           context.Context
			client        *fakeCacheStorageURIsClient
			dstDatacenter *object.Datacenter
			srcDatacenter *object.Datacenter
			srcDiskURIs   []string

			err error
			out []string
		)

		BeforeEach(func() {
			ctx = context.Background()
			client = &fakeCacheStorageURIsClient{}
			dstDatacenter = object.NewDatacenter(
				nil, vimtypes.ManagedObjectReference{
					Type:  "Datacenter",
					Value: "datacenter-1",
				})
			srcDatacenter = dstDatacenter
			srcDiskURIs = []string{
				srcContentLibItemPath + "/photon5-disk1.vmdk",
				srcContentLibItemPath + "/photon5-disk2.vmdk",
			}
		})

		JustBeforeEach(func() {
			out, err = clsutil.CacheStorageURIs(
				ctx,
				client,
				dstDatacenter,
				srcDatacenter,
				dstDir,
				srcDiskURIs...)
		})

		When("the disks are already cached", func() {
			It("should return the paths to the cached disks", func() {
				Expect(client.queryCalls).To(Equal(int32(2)))
				Expect(client.makeCalls).To(BeZero())
				Expect(client.copyCalls).To(BeZero())
				Expect(client.waitCalls).To(BeZero())
				Expect(err).ToNot(HaveOccurred())
				Expect(out).To(Equal([]string{
					dstDir + "/" + "e66e8b0765f8ff917.vmdk",
					dstDir + "/" + "b020a5eae7f68a91d.vmdk",
				}))
			})
		})

		When("the disks are not already cached", func() {

			When("querying the virtual disk fails with a RuntimeFault", func() {

				BeforeEach(func() {
					client.queryErr = soap.WrapVimFault(&vimtypes.RuntimeFault{})
				})

				It("should return the error", func() {
					Expect(client.queryCalls).To(Equal(int32(1)))
					Expect(client.makeCalls).To(BeZero())
					Expect(client.copyCalls).To(BeZero())
					Expect(client.waitCalls).To(BeZero())
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(soap.WrapVimFault(&vimtypes.RuntimeFault{})))
				})

			})

			When("querying the virtual disk fails with FileNotFound", func() {

				BeforeEach(func() {
					client.queryErr = soap.WrapVimFault(&vimtypes.FileNotFound{})
				})

				When("creating the path where the disk is cached fails", func() {
					BeforeEach(func() {
						client.makeErr = soap.WrapVimFault(&vimtypes.RuntimeFault{})
					})
					It("should return the error", func() {
						Expect(client.queryCalls).To(Equal(int32(1)))
						Expect(client.makeCalls).To(Equal(int32(1)))
						Expect(client.copyCalls).To(BeZero())
						Expect(client.waitCalls).To(BeZero())
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(soap.WrapVimFault(&vimtypes.RuntimeFault{})))
					})
				})

				When("creating the path where the disk is cached succeeds", func() {

					When("calling the copy disk api fails", func() {
						BeforeEach(func() {
							client.copyErr = soap.WrapVimFault(&vimtypes.RuntimeFault{})
						})
						It("should return the error", func() {
							Expect(client.queryCalls).To(Equal(int32(1)))
							Expect(client.makeCalls).To(Equal(int32(1)))
							Expect(client.copyCalls).To(Equal(int32(1)))
							Expect(client.waitCalls).To(BeZero())
							Expect(err).To(HaveOccurred())
							Expect(err).To(MatchError(soap.WrapVimFault(&vimtypes.RuntimeFault{})))
						})
					})

					When("calling the copy disk api succeeds", func() {

						When("copying the disk fails", func() {
							BeforeEach(func() {
								client.waitErr = soap.WrapVimFault(&vimtypes.RuntimeFault{})
							})
							It("should return the error", func() {
								Expect(client.queryCalls).To(Equal(int32(1)))
								Expect(client.makeCalls).To(Equal(int32(1)))
								Expect(client.copyCalls).To(Equal(int32(1)))
								Expect(client.waitCalls).To(Equal(int32(1)))
								Expect(err).To(HaveOccurred())
								Expect(err).To(MatchError(soap.WrapVimFault(&vimtypes.RuntimeFault{})))
							})
						})

						When("copying the disk succeeds", func() {
							It("should return the paths to the cached disks", func() {
								Expect(client.queryCalls).To(Equal(int32(2)))
								Expect(client.makeCalls).To(Equal(int32(2)))
								Expect(client.copyCalls).To(Equal(int32(2)))
								Expect(client.waitCalls).To(Equal(int32(2)))
								Expect(err).ToNot(HaveOccurred())
								Expect(out).To(Equal([]string{
									dstDir + "/" + "e66e8b0765f8ff917.vmdk",
									dstDir + "/" + "b020a5eae7f68a91d.vmdk",
								}))
							})
						})
					})
				})
			})

		})
	})
})

type fakeGetTopLevelCacheDirClient struct {
	createErr    error
	createResult string
	createCalls  int32

	convertErr    error
	convertResult string
	convertCalls  int32
}

func (m *fakeGetTopLevelCacheDirClient) CreateDirectory(
	ctx context.Context,
	datastore *object.Datastore,
	displayName, policy string) (string, error) {

	_ = atomic.AddInt32(&m.createCalls, 1)
	return m.createResult, m.createErr
}

func (m *fakeGetTopLevelCacheDirClient) ConvertNamespacePathToUuidPath( //nolint:revive,stylecheck
	ctx context.Context,
	datacenter *object.Datacenter,
	datastoreURL string) (string, error) {

	_ = atomic.AddInt32(&m.convertCalls, 1)
	return m.convertResult, m.convertErr
}

var _ = Describe("GetTopLevelCacheDir", func() {

	var _ = DescribeTable("it should panic",
		func(
			ctx context.Context,
			client clsutil.GetTopLevelCacheDirClient,
			dstDatacenter *object.Datacenter,
			dstDatastore *object.Datastore,
			dstDatastoreName,
			dstDatastoreURL string,
			topLevelDirectoryCreateSupported bool,
			expPanic string) {

			if ctx == nilContext {
				ctx = nil
			}

			f := func() {
				_, _ = clsutil.GetTopLevelCacheDir(
					ctx,
					client,
					dstDatacenter,
					dstDatastore,
					dstDatastoreName,
					dstDatastoreURL,
					topLevelDirectoryCreateSupported)
			}

			Expect(f).To(PanicWith(expPanic))
		},

		Entry(
			"nil ctx",
			nilContext,
			&fakeGetTopLevelCacheDirClient{},
			&object.Datacenter{},
			&object.Datastore{},
			"my-datastore",
			"ds://my-datastore",
			false,
			"context is nil",
		),
		Entry(
			"nil client",
			context.Background(),
			nil,
			&object.Datacenter{},
			&object.Datastore{},
			"my-datastore",
			"ds://my-datastore",
			false,
			"client is nil",
		),
		Entry(
			"nil dstDatacenter",
			context.Background(),
			&fakeGetTopLevelCacheDirClient{},
			nil,
			&object.Datastore{},
			"my-datastore",
			"ds://my-datastore",
			false,
			"dstDatacenter is nil",
		),
		Entry(
			"nil dstDatastore",
			context.Background(),
			&fakeGetTopLevelCacheDirClient{},
			&object.Datacenter{},
			nil,
			"my-datastore",
			"ds://my-datastore",
			false,
			"dstDatastore is nil",
		),
		Entry(
			"empty dstDatastoreName",
			context.Background(),
			&fakeGetTopLevelCacheDirClient{},
			&object.Datacenter{},
			&object.Datastore{},
			"",
			"ds://my-datastore",
			false,
			"dstDatastoreName is empty",
		),
		Entry(
			"empty dstDatastoreURL",
			context.Background(),
			&fakeGetTopLevelCacheDirClient{},
			&object.Datacenter{},
			&object.Datastore{},
			"my-datastore",
			"",
			false,
			"dstDatastoreURL is empty",
		),
	)

	var _ = When("it should not panic", func() {

		var (
			ctx                              context.Context
			client                           *fakeGetTopLevelCacheDirClient
			dstDatacenter                    *object.Datacenter
			dstDatastore                     *object.Datastore
			dstDatastoreName                 string
			dstDatastoreURL                  string
			topLevelDirectoryCreateSupported bool

			err error
			out string
		)

		BeforeEach(func() {
			ctx = context.Background()
			client = &fakeGetTopLevelCacheDirClient{}
			dstDatacenter = object.NewDatacenter(
				nil, vimtypes.ManagedObjectReference{
					Type:  "Datacenter",
					Value: "datacenter-1",
				})
			dstDatastore = object.NewDatastore(
				nil, vimtypes.ManagedObjectReference{
					Type:  "Datastore",
					Value: "datastore-1",
				})
			dstDatastoreName = "my-datastore"
			dstDatastoreURL = "ds://my-datastore"
			topLevelDirectoryCreateSupported = true
		})

		JustBeforeEach(func() {
			out, err = clsutil.GetTopLevelCacheDir(
				ctx,
				client,
				dstDatacenter,
				dstDatastore,
				dstDatastoreName,
				dstDatastoreURL,
				topLevelDirectoryCreateSupported)
		})

		When("datastore supports top-level directories", func() {
			It("should return the expected path", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(out).To(Equal("[my-datastore] .contentlib-cache"))
			})
		})

		When("datastore does not support top-level directories", func() {
			BeforeEach(func() {
				topLevelDirectoryCreateSupported = false
				client.createResult = "ds://vmfs/volumes/123/abc"
			})
			It("should return the expected path", func() {
				Expect(client.createCalls).To(Equal(int32(1)))
				Expect(client.convertCalls).To(BeZero())

				Expect(err).ToNot(HaveOccurred())
				Expect(out).To(Equal("[my-datastore] abc"))
			})

			When("create directory returns an error", func() {
				When("error is not FileAlreadyExists", func() {
					BeforeEach(func() {
						client.createErr = fmt.Errorf(
							"nested %w",
							soap.WrapVimFault(&vimtypes.RuntimeFault{}))
					})

					It("should return the error", func() {
						Expect(client.createCalls).To(Equal(int32(1)))
						Expect(client.convertCalls).To(BeZero())

						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(soap.WrapVimFault(
							&vimtypes.RuntimeFault{})))
					})
				})
				When("error is FileAlreadyExists", func() {
					BeforeEach(func() {
						client.createErr = fmt.Errorf(
							"nested %w",
							soap.WrapVimFault(&vimtypes.FileAlreadyExists{}))

						client.convertResult = "abc"
					})

					It("should return the expected path", func() {
						Expect(client.createCalls).To(Equal(int32(1)))
						Expect(client.convertCalls).To(Equal(int32(1)))

						Expect(err).ToNot(HaveOccurred())
						Expect(out).To(Equal("[my-datastore] abc"))
					})

					When("convert path returns an error", func() {
						BeforeEach(func() {
							client.convertErr = fmt.Errorf(
								"nested %w",
								soap.WrapVimFault(&vimtypes.RuntimeFault{}))
						})

						It("should return the error", func() {
							Expect(client.createCalls).To(Equal(int32(1)))
							Expect(client.convertCalls).To(Equal(int32(1)))

							Expect(err).To(HaveOccurred())
							Expect(err).To(MatchError(soap.WrapVimFault(
								&vimtypes.RuntimeFault{})))
						})
					})
				})
			})

		})
	})
})

var _ = DescribeTable("GetCacheDirForLibraryItem",
	func(topLevelCacheDir, itemUUID, contentVersion, expOut, expPanic string) {
		var out string
		f := func() {
			out = clsutil.GetCacheDirForLibraryItem(
				topLevelCacheDir,
				itemUUID,
				contentVersion)
		}
		if expPanic != "" {
			Expect(f).To(PanicWith(expPanic))
		} else {
			Expect(f).ToNot(Panic())
			Expect(out).To(Equal(expOut))
		}
	},
	Entry(
		"empty topLevelCacheDir should panic",
		"", "b", "c",
		"",
		"topLevelCacheDir is empty",
	),
	Entry(
		"empty itemUUID should panic",
		"a", "", "c",
		"",
		"itemUUID is empty",
	),
	Entry(
		"empty contentVersion should panic",
		"a", "b", "",
		"",
		"contentVersion is empty",
	),
	Entry(
		"absolute topLevelCacheDir",
		"/a", "b", "c",
		"/a/b/c",
		"",
	),
	Entry(
		"relative topLevelCacheDir",
		"a", "b", "c",
		"a/b/c",
		"",
	),
)

var _ = DescribeTable("GetCachedFileNameForVMDK",
	func(vmdkFileName, expOut, expPanic string) {
		var out string
		f := func() {
			out = clsutil.GetCachedFileNameForVMDK(vmdkFileName)
		}
		if expPanic != "" {
			Expect(f).To(PanicWith(expPanic))
		} else {
			Expect(f).ToNot(Panic())
			Expect(out).To(Equal(expOut))
		}
	},
	Entry(
		"empty vmdkFileName should panic",
		"",
		"",
		"vmdkFileName is empty",
	),
	Entry(
		"file name sans extension",
		"disk",
		"a07bdcbcbb025d146",
		"",
	),
	Entry(
		"file name with extension",
		"disk.vmdk",
		"a07bdcbcbb025d146",
		"",
	),
)
