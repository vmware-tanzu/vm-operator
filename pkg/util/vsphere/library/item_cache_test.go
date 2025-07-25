// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package library_test

import (
	"context"
	"os"
	"sync/atomic"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	clsutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/library"
)

type nilContextKey uint8

var nilContext = context.WithValue(context.Background(), nilContextKey(0), "nil")

type fakeCacheStorageURIsClient struct {
	queryErr   error
	queryCalls int32

	copyErr    error
	copyResult *object.Task
	copyCalls  int32

	makeErr   error
	makeCalls int32

	waitErr   error
	waitCalls int32
}

func (m *fakeCacheStorageURIsClient) DatastoreFileExists(
	ctx context.Context,
	name string,
	datacenter *object.Datacenter) error {

	_ = atomic.AddInt32(&m.queryCalls, 1)
	return m.queryErr
}

func (m *fakeCacheStorageURIsClient) CopyVirtualDisk(
	ctx context.Context,
	srcName string, srcDatacenter *object.Datacenter,
	dstName string, dstDatacenter *object.Datacenter,
	dstSpec vimtypes.BaseVirtualDiskSpec, force bool) (*object.Task, error) {

	_ = atomic.AddInt32(&m.copyCalls, 1)
	return m.copyResult, m.copyErr
}

func (m *fakeCacheStorageURIsClient) CopyDatastoreFile(
	ctx context.Context,
	srcName string, srcDatacenter *object.Datacenter,
	dstName string, dstDatacenter *object.Datacenter,
	force bool) (*object.Task, error) {

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
		ctx = logr.NewContext(context.Background(), klog.Background())
		client = &fakeCacheStorageURIsClient{}

		Expect(ctx).ToNot(BeNil())
		Expect(client).ToNot(BeNil())
	})

	var _ = DescribeTable("it should panic",
		func(
			ctx context.Context,
			client clsutil.CacheStorageURIsClient,
			dstDatacenter, srcDatacenter *object.Datacenter,
			expPanic string) {

			if ctx == nilContext {
				ctx = nil
			}

			f := func() {
				_, _ = clsutil.CacheStorageURIs(
					ctx,
					client,
					dstDatacenter,
					srcDatacenter)
			}

			Expect(f).To(PanicWith(expPanic))
		},

		Entry(
			"nil ctx",
			nilContext,
			&fakeCacheStorageURIsClient{},
			&object.Datacenter{},
			&object.Datacenter{},
			"context is nil",
		),
		Entry(
			"nil client",
			context.Background(),
			nil,
			&object.Datacenter{},
			&object.Datacenter{},
			"client is nil",
		),
		Entry(
			"nil dstDatacenter",
			context.Background(),
			&fakeCacheStorageURIsClient{},
			nil,
			&object.Datacenter{},
			"dstDatacenter is nil",
		),
		Entry(
			"nil srcDatacenter",
			context.Background(),
			&fakeCacheStorageURIsClient{},
			&object.Datacenter{},
			nil,
			"srcDatacenter is nil",
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
			dstProfileID = "123456"
		)

		var (
			dstDatacenter *object.Datacenter
			srcDatacenter *object.Datacenter
			srcDisks      []clsutil.SourceFile

			err error
			out []clsutil.CachedFile
		)

		BeforeEach(func() {
			dstDatacenter = object.NewDatacenter(
				nil, vimtypes.ManagedObjectReference{
					Type:  "Datacenter",
					Value: "datacenter-1",
				})
			srcDatacenter = dstDatacenter
			srcDisks = []clsutil.SourceFile{
				{
					Path:          srcContentLibItemPath + "/photon5-disk1.vmdk",
					DstDir:        dstDir,
					DstProfileID:  dstProfileID,
					DstDiskFormat: vimtypes.DatastoreSectorFormatNative_512,
				},
				{
					Path:          srcContentLibItemPath + "/photon5-disk2.vmdk",
					DstDir:        dstDir,
					DstProfileID:  dstProfileID,
					DstDiskFormat: vimtypes.DatastoreSectorFormatNative_512,
				},
			}
		})

		JustBeforeEach(func() {
			out, err = clsutil.CacheStorageURIs(
				ctx,
				client,
				dstDatacenter,
				srcDatacenter,
				srcDisks...)
		})

		When("the disks are already cached", func() {
			It("should return the paths to the cached disks", func() {
				Expect(client.queryCalls).To(Equal(int32(2)))
				Expect(client.makeCalls).To(BeZero())
				Expect(client.copyCalls).To(BeZero())
				Expect(client.waitCalls).To(BeZero())
				Expect(err).ToNot(HaveOccurred())
				Expect(out).To(Equal([]clsutil.CachedFile{
					{
						Path: dstDir + "/" + "e66e8b0765f8ff917.vmdk",
					},
					{
						Path: dstDir + "/" + "b020a5eae7f68a91d.vmdk",
					}}))
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
					client.queryErr = os.ErrNotExist
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
								Expect(out).To(Equal([]clsutil.CachedFile{
									{
										Path: dstDir + "/" + "e66e8b0765f8ff917.vmdk",
									},
									{
										Path: dstDir + "/" + "b020a5eae7f68a91d.vmdk",
									}}))
							})
						})
					})
				})
			})

		})
	})
})

var _ = DescribeTable("GetCacheDirectory",
	func(datastoreName, itemName, profileID, contentVersion, expOut, expPanic string) {
		var out string
		f := func() {
			out = clsutil.GetCacheDirectory(
				datastoreName,
				itemName,
				profileID,
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
		"empty datastoreName should panic",
		"", "b", "c", "d",
		"",
		"datastoreName is empty",
	),
	Entry(
		"empty itemName should panic",
		"a", "", "c", "d",
		"",
		"itemName is empty",
	),
	Entry(
		"empty profileID should panic",
		"a", "b", "", "d",
		"",
		"profileID is empty",
	),
	Entry(
		"empty contentVersion should panic",
		"a", "b", "c", "",
		"[a] b-84a516841ba77a5b4",
		"",
	),
	Entry(
		"expected path",
		"a", "b", "c", "d",
		"[a] b-84a516841ba77a5b4-3c363836cf4e16666",
		"",
	),
)

var _ = DescribeTable("GetCachedFileName",
	func(fileName, expOut, expPanic string) {
		var out string
		f := func() {
			out = clsutil.GetCachedFileName(fileName)
		}
		if expPanic != "" {
			Expect(f).To(PanicWith(expPanic))
		} else {
			Expect(f).ToNot(Panic())
			Expect(out).To(Equal(expOut))
		}
	},
	Entry(
		"empty fileName should panic",
		"",
		"",
		"fileName is empty",
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
