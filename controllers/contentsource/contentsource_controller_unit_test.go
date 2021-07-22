// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentsource_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/contentsource"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking VirtualMachineImage CRUD unit tests", unitTestsCRUDImage)
	Describe("Invoking ReconcileProviderRef unit tests", reconcileProviderRef)
	Describe("Invoking SortedContentSource unit tests", testSortedContentSources)
	Describe("Invoking GetContentLibraryNameFromOwnerRefs unit tests", unitTestGetContentLibraryNameFromOwnerRefs)
}

func reconcileProviderRef() {
	var (
		ctx            *builder.UnitTestContextForController
		reconciler     *contentsource.ContentSourceReconciler
		fakeVmProvider *providerfake.FakeVmProvider
		initObjects    []client.Object

		cs v1alpha1.ContentSource
		cl v1alpha1.ContentLibraryProvider
	)

	BeforeEach(func() {
		cl = v1alpha1.ContentLibraryProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cl",
			},
			Spec: v1alpha1.ContentLibraryProviderSpec{
				UUID: "dummy-cl-uuid",
			},
		}

		cs = v1alpha1.ContentSource{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cs",
			},
			Spec: v1alpha1.ContentSourceSpec{
				ProviderRef: v1alpha1.ContentProviderReference{
					Name: cl.Name,
					Kind: "ContentLibraryProvider",
				},
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = contentsource.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VmProvider,
		)
		fakeVmProvider = ctx.VmProvider.(*providerfake.FakeVmProvider)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		reconciler = nil
		fakeVmProvider.Reset()
		fakeVmProvider = nil
	})

	Context("ReconcileProviderRef", func() {
		Context("with a ContentLibraryProvider pointing to a vSphere content library", func() {
			BeforeEach(func() {
				initObjects = []client.Object{&cs, &cl}
			})

			It("updates the ContentLibraryProvider to add the OwnerRef", func() {
				err := reconciler.ReconcileProviderRef(ctx, &cs)
				Expect(err).NotTo(HaveOccurred())

				clAfterReconcile := &v1alpha1.ContentLibraryProvider{}
				clKey := client.ObjectKey{Name: cl.ObjectMeta.Name}
				err = ctx.Client.Get(ctx, clKey, clAfterReconcile)
				Expect(err).NotTo(HaveOccurred())
				Expect(clAfterReconcile.OwnerReferences[0].Name).To(Equal(cs.Name))
			})
		})
	})
}

func unitTestGetContentLibraryNameFromOwnerRefs() {
	Context("with list of OwnerRefs", func() {
		expectedCLName := "cl-name"
		ownerRefs := []metav1.OwnerReference{
			{
				Kind: "ContentLibraryProvider",
				Name: expectedCLName,
			},
			{
				Kind: "dummy-kind",
				Name: "dummy-name-2",
			},
		}

		It("returns the name of ContentLibraryProvider", func() {
			clName := contentsource.GetContentLibraryNameFromOwnerRefs(ownerRefs)
			Expect(clName).To(Equal(expectedCLName))
		})
	})
}

func unitTestsCRUDImage() {
	var (
		ctx            *builder.UnitTestContextForController
		reconciler     *contentsource.ContentSourceReconciler
		fakeVmProvider *providerfake.FakeVmProvider
		initObjects    []client.Object

		cs v1alpha1.ContentSource
		cl v1alpha1.ContentLibraryProvider
	)

	BeforeEach(func() {
		cl = v1alpha1.ContentLibraryProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cl",
			},
		}

		cs = v1alpha1.ContentSource{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cs",
			},
			Spec: v1alpha1.ContentSourceSpec{
				ProviderRef: v1alpha1.ContentProviderReference{
					Name:      cl.Name,
					Namespace: cl.Namespace,
				},
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = contentsource.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VmProvider,
		)
		fakeVmProvider = ctx.VmProvider.(*providerfake.FakeVmProvider)
	})

	JustAfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		reconciler = nil
		fakeVmProvider.Reset()
		fakeVmProvider = nil
	})

	Context("SyncImages", func() {
		Context("VirtualMachineImage already exists", func() {
			var existingImg, providerImg *v1alpha1.VirtualMachineImage

			BeforeEach(func() {
				existingImg = &v1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dummy-image",
						Annotations: map[string]string{
							"dummy-key": "dummy-value",
						},
					},
					Spec: v1alpha1.VirtualMachineImageSpec{
						Type: "dummy-type-1",
					},
				}

				providerImg = &v1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: existingImg.Name,
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: "vmoperator.vmware.com/v1alpha1",
							Kind:       "ContentLibraryProvider",
							Name:       "dummy-cl",
						}},
					},
					Spec: v1alpha1.VirtualMachineImageSpec{
						Type: "dummy-type-2",
					},
				}
			})

			providerListImageFromCLFunc := func(ctx context.Context, cl v1alpha1.ContentLibraryProvider, currentCLImages map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error) {
				return []*v1alpha1.VirtualMachineImage{providerImg}, nil
			}

			Context("another library with a duplicate image name is added", func() {

				When("the existing image does not have any ContentLibraryProvider annotations", func() {
					BeforeEach(func() {
						initObjects = append(initObjects, existingImg, &cl, &cs)
					})

					It("the existing VirtualMachineImage is overwritten", func() {
						fakeVmProvider.ListVirtualMachineImagesFromContentLibraryFn = providerListImageFromCLFunc

						err := reconciler.SyncImages(ctx.Context)
						Expect(err).NotTo(HaveOccurred())

						img := &v1alpha1.VirtualMachineImage{}
						Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: existingImg.Name}, img)).To(Succeed())
						Expect(img.OwnerReferences).To(Equal(providerImg.OwnerReferences))
						Expect(img.Spec).To(Equal(providerImg.Spec))
						Expect(img.Annotations).To(Equal(providerImg.Annotations))
					})
				})

				When("the existing image has an ContentLibraryProvider OwnerRef", func() {

					BeforeEach(func() {
						existingImg.OwnerReferences = []metav1.OwnerReference{{
							APIVersion: "vmoperator.vmware.com/v1alpha1",
							Kind:       "ContentLibraryProvider",
							Name:       "dummy-cl-2",
						}}
						initObjects = append(initObjects, existingImg, &cl, &cs)
					})

					It("the existing VirtualMachineImage is not overwritten", func() {
						fakeVmProvider.ListVirtualMachineImagesFromContentLibraryFn = providerListImageFromCLFunc

						err := reconciler.SyncImages(ctx.Context)
						Expect(err).NotTo(HaveOccurred())

						img := &v1alpha1.VirtualMachineImage{}
						Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: existingImg.Name}, img)).To(Succeed())
						Expect(img.OwnerReferences).To(Equal(existingImg.OwnerReferences))
						Expect(img.Spec).To(Equal(existingImg.Spec))
						Expect(img.Annotations).To(Equal(existingImg.Annotations))
					})
				})
			})

			When("the existing image has a valid ContentLibraryProvider OwnerRef", func() {

				BeforeEach(func() {
					initObjects = append(initObjects, providerImg, &cl, &cs)
				})

				It("calls provider with the current image in map", func() {
					var called bool
					fakeVmProvider.ListVirtualMachineImagesFromContentLibraryFn = func(_ context.Context, _ v1alpha1.ContentLibraryProvider,
						currentCLImages map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error) {

						called = true
						Expect(currentCLImages).To(HaveKey(providerImg.Name))
						return []*v1alpha1.VirtualMachineImage{providerImg}, nil
					}

					err := reconciler.SyncImages(ctx.Context)
					Expect(err).NotTo(HaveOccurred())
					Expect(called).To(BeTrue())
				})
			})
		})
	})

	Context("DeleteImages", func() {
		var (
			images []v1alpha1.VirtualMachineImage
			image  v1alpha1.VirtualMachineImage
		)

		BeforeEach(func() {
			image = v1alpha1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-vm-image",
				},
			}
		})

		When("no images are specified", func() {
			It("does not throw an error", func() {
				err := reconciler.DeleteImages(ctx, images)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("non-empty list of images is specified", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, &image)
				images = append(images, image)
			})

			It("successfully deletes the images", func() {
				err := reconciler.DeleteImages(ctx, images)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("when client delete fails because the image doesnt exist", func() {
			BeforeEach(func() {
				images = append(images, image)
			})

			It("returns an error", func() {
				err := reconciler.DeleteImages(ctx, images)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("UpdateImages", func() {
		var (
			images []v1alpha1.VirtualMachineImage
			image  v1alpha1.VirtualMachineImage
		)

		BeforeEach(func() {
			image = v1alpha1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-vm-image",
				},
			}
		})

		When("no images are specified", func() {
			It("does not throw an error", func() {
				err := reconciler.UpdateImages(ctx, images)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("non-empty list of images is specified", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, &image)
				images = append(images, image)
			})

			It("successfully updates the images", func() {
				// Modify the VirtualMachineImage spec
				images[0].Spec.Type = "updated-dummy-type"
				imgBeforeUpdate := images[0]

				err := reconciler.UpdateImages(ctx, images)
				Expect(err).NotTo(HaveOccurred())

				imgAfterUpdate := &v1alpha1.VirtualMachineImage{}
				objKey := client.ObjectKey{Name: images[0].Name, Namespace: images[0].Namespace}
				Expect(ctx.Client.Get(ctx, objKey, imgAfterUpdate)).To(Succeed())

				Expect(imgBeforeUpdate.Spec.Type).To(Equal(imgAfterUpdate.Spec.Type))
			})
		})

		When("when client update fails", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, &image)
				images = append(images, image)
			})

			It("fails to update the images", func() {
				images[0].Name = "invalid_name" // invalid name, to fail the Update op.

				err := reconciler.UpdateImages(ctx, images)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("CreateImages", func() {
		var (
			images []v1alpha1.VirtualMachineImage
			image  v1alpha1.VirtualMachineImage
		)

		BeforeEach(func() {
			image = v1alpha1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-vm-image",
				},
			}
		})

		When("non-empty list of images is specified", func() {
			BeforeEach(func() {
				images = append(images, image)
			})

			It("successfully creates the images", func() {
				err := reconciler.CreateImages(ctx, images)
				Expect(err).NotTo(HaveOccurred())

				img := images[0]
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: img.Name}, &img)).To(Succeed())
			})
		})
	})

	Context("DiffImages: Difference VirtualMachineImage resources", func() {
		Context("when left and right is empty", func() {
			var left []v1alpha1.VirtualMachineImage
			var right []v1alpha1.VirtualMachineImage

			It("return empty sets", func() {
				added, removed, updated := reconciler.DiffImages(left, right)
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
				added, removed, updated := reconciler.DiffImages(left, right)
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
				added, removed, updated := reconciler.DiffImages(left, right)
				Expect(added).To(BeEmpty())
				Expect(removed).ToNot(BeEmpty())
				Expect(removed).To(HaveLen(1))
				Expect(updated).To(BeEmpty())
			})
		})

		Context("when left and right are not empty", func() {
			var left []v1alpha1.VirtualMachineImage
			var right []v1alpha1.VirtualMachineImage
			var imageL v1alpha1.VirtualMachineImage
			var imageR v1alpha1.VirtualMachineImage

			BeforeEach(func() {
				imageL = v1alpha1.VirtualMachineImage{}
				imageR = v1alpha1.VirtualMachineImage{}
				left = []v1alpha1.VirtualMachineImage{}
				right = []v1alpha1.VirtualMachineImage{}
			})

			JustBeforeEach(func() {
				left = append(left, imageL)
				right = append(right, imageR)
			})

			Context("when left and right have a different Spec", func() {
				BeforeEach(func() {
					imageL = v1alpha1.VirtualMachineImage{
						Spec: v1alpha1.VirtualMachineImageSpec{
							Type: "left-type",
						},
					}

					imageR = v1alpha1.VirtualMachineImage{
						Spec: v1alpha1.VirtualMachineImageSpec{
							Type: "right-type",
						},
					}
				})

				It("should return a non-empty updated spec", func() {
					added, removed, updated := reconciler.DiffImages(left, right)
					Expect(added).To(BeEmpty())
					Expect(removed).To(BeEmpty())
					Expect(updated).ToNot(BeEmpty())
					Expect(updated).To(HaveLen(1))
				})
			})

			Context("when left and right have same Spec", func() {
				It("should return an empty updated spec", func() {
					added, removed, updated := reconciler.DiffImages(left, right)
					Expect(added).To(BeEmpty())
					Expect(removed).To(BeEmpty())
					Expect(updated).To(BeEmpty())
				})
			})

			When("left and right have different Annotations", func() {
				var annotations = map[string]string{
					"key": "value",
				}

				BeforeEach(func() {
					imageL = v1alpha1.VirtualMachineImage{}

					imageR = v1alpha1.VirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: annotations,
						},
					}
				})

				It("should return left with annotation set", func() {
					added, removed, updated := reconciler.DiffImages(left, right)
					Expect(added).To(BeEmpty())
					Expect(removed).To(BeEmpty())
					Expect(updated).ToNot(BeEmpty())
					Expect(updated).To(HaveLen(1))
					Expect(updated[0].Annotations).To(Equal(annotations))
				})
			})

			When("left and right have different OwnerReference", func() {
				var ownerRef = []metav1.OwnerReference{{
					Name: "dummy-name",
				}}

				BeforeEach(func() {
					imageL = v1alpha1.VirtualMachineImage{}

					imageR = v1alpha1.VirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{
							OwnerReferences: ownerRef,
						},
					}
				})

				It("should return left with annotation set", func() {
					added, removed, updated := reconciler.DiffImages(left, right)
					Expect(added).To(BeEmpty())
					Expect(removed).To(BeEmpty())
					Expect(updated).ToNot(BeEmpty())
					Expect(updated).To(HaveLen(1))
					Expect(updated[0].OwnerReferences).To(Equal(ownerRef))
				})
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
				added, removed, updated := reconciler.DiffImages(left, right)
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
				added, removed, updated := reconciler.DiffImages(left, right)
				Expect(added).ToNot(BeEmpty())
				Expect(added).To(HaveLen(1))
				Expect(added).To(ContainElement(imageRight))
				Expect(removed).To(BeEmpty())
				Expect(updated).To(BeEmpty())
			})
		})
	})

	Context("GetImagesFromContentProvider", func() {

		Context("when the ContentLibraryProvider resource doesnt exist", func() {
			It("returns error", func() {
				images, err := reconciler.GetImagesFromContentProvider(ctx.Context, cs, nil)
				Expect(err).To(HaveOccurred())
				Expect(apiErrors.IsNotFound(err)).To(BeTrue())
				Expect(images).To(BeNil())
			})
		})

		When("provider returns error in listing images from CL", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, &cs, &cl)
			})

			It("provider returns error when listing images", func() {
				fakeVmProvider.ListVirtualMachineImagesFromContentLibraryFn = func(ctx context.Context, _ v1alpha1.ContentLibraryProvider, _ map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error) {
					return nil, fmt.Errorf("error listing images from provider")
				}

				images, err := reconciler.GetImagesFromContentProvider(ctx.Context, cs, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error listing images from provider"))
				Expect(images).To(BeNil())
			})
		})

		Context("when ContentSource resource passes to a valid vSphere CL", func() {
			var images []*v1alpha1.VirtualMachineImage

			BeforeEach(func() {
				initObjects = append(initObjects, &cs, &cl)

				images = []*v1alpha1.VirtualMachineImage{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dummy-image-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dummy-image-2",
						},
					},
				}
			})

			It("provider successfully lists images", func() {
				fakeVmProvider.ListVirtualMachineImagesFromContentLibraryFn = func(ctx context.Context, _ v1alpha1.ContentLibraryProvider, _ map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error) {
					return images, nil
				}

				clImages, err := reconciler.GetImagesFromContentProvider(ctx.Context, cs, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(clImages).Should(HaveLen(2))
				Expect(clImages).Should(Equal(images))
			})
		})

		When("the current image has ContentLibraryProvider ownerRef", func() {
			var existingImg v1alpha1.VirtualMachineImage

			BeforeEach(func() {
				existingImg = v1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dummy-image",
						Annotations: map[string]string{
							"dummy-key": "dummy-value",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "vmoperator.vmware.com/v1alpha1",
								Kind:       "ContentLibraryProvider",
								Name:       cl.Name,
							},
						},
					},
					Spec: v1alpha1.VirtualMachineImageSpec{
						Type: "dummy-type-1",
					},
				}

				initObjects = append(initObjects, &cl, &cs)
			})

			It("calls list with the current image in map", func() {
				var called bool
				fakeVmProvider.ListVirtualMachineImagesFromContentLibraryFn = func(_ context.Context, _ v1alpha1.ContentLibraryProvider,
					currentCLImages map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error) {

					called = true
					Expect(currentCLImages).To(HaveKey(existingImg.Name))
					return []*v1alpha1.VirtualMachineImage{&existingImg}, nil
				}

				clImages, err := reconciler.GetImagesFromContentProvider(ctx.Context, cs, []v1alpha1.VirtualMachineImage{existingImg})
				Expect(err).NotTo(HaveOccurred())
				Expect(clImages).Should(HaveLen(1))
				Expect(called).To(BeTrue())
			})
		})
	})

	Context("DifferenceImages", func() {

		var (
			img1 *v1alpha1.VirtualMachineImage
			img2 *v1alpha1.VirtualMachineImage
		)

		BeforeEach(func() {
			img1 = &v1alpha1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-image-1",
				},
			}
			img2 = &v1alpha1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-image-2",
				},
			}
		})

		imageExists := func(imageName string, images []v1alpha1.VirtualMachineImage) bool {
			for _, img := range images {
				if imageName == img.Name {
					return true
				}
			}

			return false
		}

		When("no ContentSources exist", func() {
			It("returns no error and no added/removed/updated images", func() {
				err, added, removed, updated := reconciler.DifferenceImages(ctx.Context)
				Expect(err).NotTo(HaveOccurred())

				Expect(added).To(BeEmpty())
				Expect(removed).To(BeEmpty())
				Expect(updated).To(BeEmpty())
			})
		})

		When("Images exist on the API server and provider", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, img1, &cl, &cs)
			})

			It("Should remove the image from APIServer and add image from provider", func() {
				fakeVmProvider.ListVirtualMachineImagesFromContentLibraryFn = func(ctx context.Context, cl v1alpha1.ContentLibraryProvider, currentCLImages map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error) {
					return []*v1alpha1.VirtualMachineImage{img2}, nil
				}

				err, added, removed, updated := reconciler.DifferenceImages(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(added).NotTo(BeEmpty())
				Expect(added).To(HaveLen(1))
				Expect(imageExists(img2.Name, added)).To(BeTrue())

				Expect(removed).NotTo(BeEmpty())
				Expect(removed).To(HaveLen(1))
				Expect(imageExists(img1.Name, removed)).To(BeTrue())

				Expect(updated).To(BeEmpty())
			})
		})

		Context("with a ContentSource pointing to a non-existent content library", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, img1, &cl, &cs)
			})

			It("returns the list of VirtualMachineImages from the valid CL", func() {
				fakeVmProvider.ListVirtualMachineImagesFromContentLibraryFn = func(ctx context.Context, cl v1alpha1.ContentLibraryProvider, _ map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error) {
					return nil, nil
				}
				err, added, removed, updated := reconciler.DifferenceImages(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(added).To(BeNil())

				Expect(removed).NotTo(BeEmpty())
				Expect(removed).To(HaveLen(1))
				Expect(imageExists(img1.Name, removed)).To(BeTrue())

				Expect(updated).To(BeNil())
			})
		})
	})
}
