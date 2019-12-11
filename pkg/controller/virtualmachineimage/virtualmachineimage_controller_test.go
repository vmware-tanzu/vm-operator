// +build !integration

//go:generate mockgen -destination=../../../mocks/mock_virtual_machine_provider_interface.go -package=mocks github.com/vmware-tanzu/vm-operator/pkg/vmprovider VirtualMachineProviderInterface
//go:generate mockgen -destination=../../../mocks/mock_client.go -package=mocks sigs.k8s.io/controller-runtime/pkg/client Client

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

//nolint:golint,dupl // The dupl linter is too aggressive in labeling this code as duplicate.
package virtualmachineimage

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/mocks"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
)

var _ = Describe("VirtualMachineImageDiscoverer", func() {
	var (
		mockCtrl        *gomock.Controller
		mockClient      *mocks.MockClient
		mockVmProvider  *mocks.MockVirtualMachineProviderInterface
		imageDiscoverer *VirtualMachineImageDiscoverer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = mocks.NewMockClient(mockCtrl)
		mockVmProvider = mocks.NewMockVirtualMachineProviderInterface(mockCtrl)
		options := VirtualMachineImageDiscovererOptions{
			initialDiscoveryFrequency:    1 * time.Second,
			continuousDiscoveryFrequency: 2 * time.Second,
		}
		imageDiscoverer = newVirtualMachineImageDiscoverer(mockClient, mockVmProvider, options)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("createImages", func() {
		Context("when images is empty", func() {
			var images []v1alpha1.VirtualMachineImage
			ctx := context.Background()

			BeforeEach(func() {
				clientCreateImageNotCalled(mockClient)
			})

			It("return success", func() {
				err := imageDiscoverer.createImages(ctx, images)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when images is non-empty", func() {
			var images []v1alpha1.VirtualMachineImage
			ctx := context.Background()

			BeforeEach(func() {
				image := v1alpha1.VirtualMachineImage{}
				images = append(images, image)
				clientCreateImageSucceeds(mockClient, ctx, &image)
			})

			It("return success", func() {
				err := imageDiscoverer.createImages(ctx, images)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when client create fails", func() {
			var images []v1alpha1.VirtualMachineImage
			ctx := context.Background()

			BeforeEach(func() {
				image := v1alpha1.VirtualMachineImage{}
				images = append(images, image)
				clientCreateImageFails(mockClient, ctx, &image)
			})

			It("returns an error", func() {
				err := imageDiscoverer.createImages(ctx, images)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("deleteImages", func() {
		Context("when images is empty", func() {
			var images []v1alpha1.VirtualMachineImage
			ctx := context.Background()

			BeforeEach(func() {
				clientDeleteImageNotCalled(mockClient)
			})

			It("return success", func() {
				err := imageDiscoverer.deleteImages(ctx, images)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when images is non-empty", func() {
			var images []v1alpha1.VirtualMachineImage
			ctx := context.Background()

			BeforeEach(func() {
				image := v1alpha1.VirtualMachineImage{}
				images = append(images, image)
				clientDeleteImageSucceeds(mockClient, ctx, &image)
			})

			It("return success", func() {
				err := imageDiscoverer.deleteImages(ctx, images)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when client delete fails", func() {
			var images []v1alpha1.VirtualMachineImage
			ctx := context.Background()

			BeforeEach(func() {
				image := v1alpha1.VirtualMachineImage{}
				images = append(images, image)
				clientDeleteImageFails(mockClient, ctx, &image)
			})

			It("returns an error", func() {
				err := imageDiscoverer.deleteImages(ctx, images)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("diffImages", func() {
		Context("when left and right is empty", func() {
			var left []v1alpha1.VirtualMachineImage
			var right []v1alpha1.VirtualMachineImage

			It("return empty sets", func() {
				added, removed, updated := imageDiscoverer.diffImages(left, right)
				Expect(added).To(BeEmpty())
				Expect(removed).To(BeEmpty())
				Expect(updated).To(BeEmpty())
			})
		})

		Context("when left is empty and right is non-empty", func() {
			var left []v1alpha1.VirtualMachineImage
			var right []v1alpha1.VirtualMachineImage
			var image v1alpha1.VirtualMachineImage

			BeforeEach(func() {
				image = v1alpha1.VirtualMachineImage{}
				right = append(right, image)
			})

			It("return a non-empty added set", func() {
				added, removed, updated := imageDiscoverer.diffImages(left, right)
				Expect(added).ToNot(BeEmpty())
				Expect(added).To(HaveLen(1))
				Expect(removed).To(BeEmpty())
				Expect(updated).To(BeEmpty())
			})
		})

		Context("when left is non-empty and right is empty", func() {
			var left []v1alpha1.VirtualMachineImage
			var right []v1alpha1.VirtualMachineImage
			var image v1alpha1.VirtualMachineImage

			BeforeEach(func() {
				image = v1alpha1.VirtualMachineImage{}
				left = append(left, image)
			})

			It("return a non-empty removed set", func() {
				added, removed, updated := imageDiscoverer.diffImages(left, right)
				Expect(added).To(BeEmpty())
				Expect(removed).ToNot(BeEmpty())
				Expect(removed).To(HaveLen(1))
				Expect(updated).To(BeEmpty())
			})
		})

		Context("when left and right are non-empty and the same", func() {
			var left []v1alpha1.VirtualMachineImage
			var right []v1alpha1.VirtualMachineImage
			var image v1alpha1.VirtualMachineImage

			BeforeEach(func() {
				image = v1alpha1.VirtualMachineImage{}
				left = append(left, image)
				right = append(right, image)
			})

			It("return a non-empty updated set", func() {
				added, removed, updated := imageDiscoverer.diffImages(left, right)
				Expect(added).To(BeEmpty())
				Expect(removed).To(BeEmpty())
				Expect(updated).ToNot(BeEmpty())
				Expect(updated).To(HaveLen(1))
			})
		})

		Context("when left and right are non-empty and unique", func() {
			var left []v1alpha1.VirtualMachineImage
			var right []v1alpha1.VirtualMachineImage
			var imageLeft v1alpha1.VirtualMachineImage
			var imageRight v1alpha1.VirtualMachineImage

			BeforeEach(func() {
				imageLeft = v1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "left",
					},
				}
				imageRight = v1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "right",
					},
				}
				left = append(left, imageLeft)
				right = append(right, imageRight)
			})

			It("return a non-empty added and removed set", func() {
				added, removed, updated := imageDiscoverer.diffImages(left, right)
				Expect(added).ToNot(BeEmpty())
				Expect(added).To(HaveLen(1))
				Expect(removed).ToNot(BeEmpty())
				Expect(removed).To(HaveLen(1))
				Expect(updated).To(BeEmpty())
			})
		})

		Context("when left and right are non-empty and have a non-complete intersection", func() {
			var left []v1alpha1.VirtualMachineImage
			var right []v1alpha1.VirtualMachineImage
			var imageLeft v1alpha1.VirtualMachineImage
			var imageRight v1alpha1.VirtualMachineImage

			BeforeEach(func() {
				imageLeft = v1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "left",
					},
				}
				imageRight = v1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "right",
					},
				}
				left = append(left, imageLeft)
				right = append(right, imageLeft)
				right = append(right, imageRight)
			})

			It("return a non-empty added set with a single entry", func() {
				added, removed, updated := imageDiscoverer.diffImages(left, right)
				Expect(added).ToNot(BeEmpty())
				Expect(added).To(HaveLen(1))
				Expect(added).To(ContainElement(imageRight))
				Expect(removed).To(BeEmpty())
				Expect(updated).ToNot(BeEmpty())
				Expect(updated).To(HaveLen(1))
			})
		})
	})

	Describe("differenceImages", func() {

		Context("when image list fails", func() {
			ctx := context.Background()
			var imageList v1alpha1.VirtualMachineImageList

			BeforeEach(func() {
				clientListImageFails(mockClient, ctx, &imageList)
			})

			It("returns an error", func() {
				err, _, _ := imageDiscoverer.differenceImages(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to list images from control plane"))
			})
		})

		Context("when vmprovider image list fails", func() {
			ctx := context.Background()
			var imageList v1alpha1.VirtualMachineImageList

			BeforeEach(func() {
				clientListImageSucceeds(mockClient, ctx, &imageList)
				vmproviderListImageFails(mockVmProvider, ctx, "")
			})

			It("returns an error", func() {
				err, _, _ := imageDiscoverer.differenceImages(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to list images from vmprovider"))
			})
		})

		Context("when image list and vmprovider list succeed", func() {
			ctx := context.Background()
			images := []*v1alpha1.VirtualMachineImage{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "image",
				},
			}}
			var imageList v1alpha1.VirtualMachineImageList

			BeforeEach(func() {
				clientListImageSucceeds(mockClient, ctx, &imageList)
				vmproviderListImageSucceeds(mockVmProvider, ctx, "", images)
			})

			It("returns success", func() {
				err, _, _ := imageDiscoverer.differenceImages(ctx)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Describe("Start", func() {
		Context("when sent a stop change event", func() {
			ctx := context.Background()
			image := &v1alpha1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "image",
				},
			}

			images := []*v1alpha1.VirtualMachineImage{image}

			var imageList v1alpha1.VirtualMachineImageList

			BeforeEach(func() {
				clientListImageSucceeds(mockClient, ctx, &imageList)
				vmproviderListImageSucceeds(mockVmProvider, ctx, "", images)
				clientCreateImageSucceeds(mockClient, ctx, image)
			})

			It("should stop the ticker", func(done Done) {
				stopChan := make(chan struct{})
				doneChan := make(chan struct{})
				imageDiscoverer.Start(stopChan, doneChan)
				close(stopChan)

				Eventually(doneChan).Should(BeClosed())
				close(done)
			}, 5) // 5 second timeout due to some delay
		})
	})
})

func clientCreateImageNotCalled(m *mocks.MockClient) *gomock.Call {
	return m.EXPECT().Create(gomock.Nil(), gomock.Nil()).MinTimes(0).MaxTimes(0)
}

func clientCreateImageSucceeds(m *mocks.MockClient, ctx context.Context, image *v1alpha1.VirtualMachineImage) *gomock.Call {
	return m.EXPECT().Create(gomock.Eq(ctx), gomock.Eq(image)).MinTimes(1).MaxTimes(1).Return(nil)
}

func clientCreateImageFails(m *mocks.MockClient, ctx context.Context, image *v1alpha1.VirtualMachineImage) *gomock.Call {
	return m.EXPECT().Create(gomock.Eq(ctx), gomock.Eq(image)).MinTimes(1).MaxTimes(1).Return(fmt.Errorf("failed to create image"))
}

func clientDeleteImageNotCalled(m *mocks.MockClient) *gomock.Call {
	return m.EXPECT().Delete(gomock.Nil(), gomock.Nil()).MinTimes(0).MaxTimes(0)
}

func clientDeleteImageSucceeds(m *mocks.MockClient, ctx context.Context, image *v1alpha1.VirtualMachineImage) *gomock.Call {
	return m.EXPECT().Delete(gomock.Eq(ctx), gomock.Eq(image)).MinTimes(1).MaxTimes(1).Return(nil)
}

func clientDeleteImageFails(m *mocks.MockClient, ctx context.Context, image *v1alpha1.VirtualMachineImage) *gomock.Call {
	return m.EXPECT().Delete(gomock.Eq(ctx), gomock.Eq(image)).MinTimes(1).MaxTimes(1).Return(fmt.Errorf("failed to delete image"))
}

func clientListImageSucceeds(m *mocks.MockClient, ctx context.Context, imageList *v1alpha1.VirtualMachineImageList) *gomock.Call {
	return m.EXPECT().List(gomock.Eq(ctx), &client.ListOptions{}, gomock.Eq(imageList)).MinTimes(1).MaxTimes(1).Return(nil)
}

func clientListImageFails(m *mocks.MockClient, ctx context.Context, imageList *v1alpha1.VirtualMachineImageList) *gomock.Call {
	return m.EXPECT().List(gomock.Eq(ctx), &client.ListOptions{}, gomock.Eq(imageList)).MinTimes(1).MaxTimes(1).Return(fmt.Errorf("failed to list images"))
}

func vmproviderListImageSucceeds(m *mocks.MockVirtualMachineProviderInterface, ctx context.Context, ns string, images []*v1alpha1.VirtualMachineImage) *gomock.Call {
	return m.EXPECT().ListVirtualMachineImages(gomock.Eq(ctx), gomock.Eq(ns)).MinTimes(1).MaxTimes(1).Return(images, nil)
}

func vmproviderListImageFails(m *mocks.MockVirtualMachineProviderInterface, ctx context.Context, ns string) *gomock.Call {
	return m.EXPECT().ListVirtualMachineImages(gomock.Eq(ctx), gomock.Eq(ns)).MinTimes(1).MaxTimes(1).Return(nil, fmt.Errorf("failed to list images from vmprovider"))
}
