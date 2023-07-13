// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentsource_test

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/contentsource"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking VirtualMachineImage CRUD unit tests", unitTestsCRUDImage)
	Describe("Invoking ReconcileProviderRef unit tests", reconcileProviderRef)
	Describe("Invoking IsImageOwnedByContentLibrary unit tests", unitTestIsImageOwnedByContentLibrary)
}

func reconcileProviderRef() {
	var (
		ctx            *builder.UnitTestContextForController
		reconciler     *contentsource.Reconciler
		fakeVMProvider *providerfake.VMProvider
		initObjects    []client.Object

		cs vmopv1.ContentSource
		cl vmopv1.ContentLibraryProvider
	)

	BeforeEach(func() {
		cl = vmopv1.ContentLibraryProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cl",
			},
			Spec: vmopv1.ContentLibraryProviderSpec{
				UUID: "dummy-cl-uuid",
			},
		}

		cs = vmopv1.ContentSource{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cs",
			},
			Spec: vmopv1.ContentSourceSpec{
				ProviderRef: vmopv1.ContentProviderReference{
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
			ctx.VMProvider,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		reconciler = nil
		fakeVMProvider.Reset()
		fakeVMProvider = nil
	})

	Context("ReconcileProviderRef", func() {
		Context("with a ContentLibraryProvider pointing to a vSphere content library", func() {
			BeforeEach(func() {
				initObjects = []client.Object{&cs, &cl}
			})

			It("updates the ContentLibraryProvider to add the OwnerRef", func() {
				clProvider, err := reconciler.ReconcileProviderRef(ctx, &cs)
				Expect(err).NotTo(HaveOccurred())
				Expect(clProvider).NotTo(BeNil())

				clAfterReconcile := &vmopv1.ContentLibraryProvider{}
				clKey := client.ObjectKey{Name: cl.ObjectMeta.Name}
				err = ctx.Client.Get(ctx, clKey, clAfterReconcile)
				Expect(err).NotTo(HaveOccurred())
				Expect(clAfterReconcile.OwnerReferences[0].Name).To(Equal(cs.Name))
			})
		})
	})
}

func unitTestIsImageOwnedByContentLibrary() {
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
	img := vmopv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "dummy-image",
			OwnerReferences: ownerRefs,
		},
	}

	Context("with list of OwnerRefs", func() {
		BeforeEach(func() {
			img.OwnerReferences = ownerRefs
		})

		AfterEach(func() {
			img.OwnerReferences = []metav1.OwnerReference{}
		})

		It("Image is owned by the content library", func() {
			Expect(contentsource.IsImageOwnedByContentLibrary(img, expectedCLName)).To(BeTrue())
		})

		It("Image is not owned by the content library", func() {
			img.OwnerReferences[0].Name = "dummy-cl"
			Expect(contentsource.IsImageOwnedByContentLibrary(img, expectedCLName)).To(BeFalse())
		})
	})

	Context("With empty ContentLibraryProvider OwnerRefs", func() {
		It("Image is owned by the content library", func() {
			img.OwnerReferences = []metav1.OwnerReference{
				{
					Kind: "dummy-kind",
					Name: "dummy-name-2",
				},
			}
			Expect(contentsource.IsImageOwnedByContentLibrary(img, "dummy-name")).To(BeTrue())
		})
	})
}

func unitTestsCRUDImage() {
	var (
		ctx            *builder.UnitTestContextForController
		reconciler     *contentsource.Reconciler
		fakeVMProvider *providerfake.VMProvider
		initObjects    []client.Object

		cs vmopv1.ContentSource
		cl vmopv1.ContentLibraryProvider
	)

	BeforeEach(func() {
		cl = vmopv1.ContentLibraryProvider{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "vmoperator.vmware.com/vmopv1",
				Kind:       "ContentLibraryProvider",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cl",
			},
		}

		cs = vmopv1.ContentSource{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "vmoperator.vmware.com/vmopv1",
				Kind:       "ContentSource",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cs",
			},
			Spec: vmopv1.ContentSourceSpec{
				ProviderRef: vmopv1.ContentProviderReference{
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
			ctx.VMProvider,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
	})

	JustAfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		reconciler = nil
		fakeVMProvider.Reset()
		fakeVMProvider = nil
	})

	Context("SyncImagesFromContentProvider", func() {
		Context("VirtualMachineImage already exists", func() {
			var existingImg, providerImg, providerConvertedImg *vmopv1.VirtualMachineImage

			BeforeEach(func() {
				existingImg = &vmopv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dummy-image",
						Annotations: map[string]string{
							"dummy-key": "dummy-value",
						},
					},
					Spec: vmopv1.VirtualMachineImageSpec{
						Type: "dummy-type-1",
					},
				}

				providerImg = &vmopv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dummy-image",
					},
					Spec: vmopv1.VirtualMachineImageSpec{
						Type:    "dummy-type-2",
						ImageID: "dummy-id-2",
					},
					Status: vmopv1.VirtualMachineImageStatus{
						ImageName: "dummy-image",
					},
				}

				providerConvertedImg = &vmopv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dummy-image",
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: "vmoperator.vmware.com/vmopv1",
							Kind:       "ContentLibraryProvider",
							Name:       "dummy-cl",
						}},
					},
					Spec: vmopv1.VirtualMachineImageSpec{
						Type:    "dummy-type-2",
						ImageID: "dummy-id-2",
						ProviderRef: vmopv1.ContentProviderReference{
							APIVersion: "vmoperator.vmware.com/vmopv1",
							Kind:       "ContentLibraryProvider",
							Name:       "dummy-cl",
						},
					},
					Status: vmopv1.VirtualMachineImageStatus{
						ImageName: "dummy-image",
					},
				}

				initObjects = append(initObjects, &cl, &cs, existingImg)
			})

			Context("When upgrading from not supporting duplicate vm image names", func() {
				BeforeEach(func() {
					existingImg.OwnerReferences = []metav1.OwnerReference{{
						APIVersion: "vmoperator.vmware.com/vmopv1",
						Kind:       "ContentLibraryProvider",
						Name:       "dummy-cl",
					}}
					existingImg.Spec.ProviderRef = vmopv1.ContentProviderReference{
						Name: "dummy-cl",
					}
					existingImg.Status.ImageName = "dummy-image"
				})

				It("Should delete all VirtualMachineImage resources with old versions", func() {
					fakeVMProvider.ListItemsFromContentLibraryFn = func(_ context.Context, contentLibrary *vmopv1.ContentLibraryProvider) ([]string, error) {
						return []string{}, nil
					}

					err := reconciler.SyncImagesFromContentProvider(ctx.Context, &cl)
					Expect(err).NotTo(HaveOccurred())
					imgs := &vmopv1.VirtualMachineImageList{}
					Expect(ctx.Client.List(ctx, imgs)).To(Succeed())
					Expect(imgs.Items).To(BeEmpty())
				})
			})

			Context("content library contains a valid VM image", func() {
				JustBeforeEach(func() {
					fakeVMProvider.Lock()
					fakeVMProvider.ListItemsFromContentLibraryFn = func(_ context.Context, contentLibrary *vmopv1.ContentLibraryProvider) ([]string, error) {
						return []string{providerImg.Spec.ImageID}, nil
					}
					fakeVMProvider.GetVirtualMachineImageFromContentLibraryFn = func(_ context.Context, _ *vmopv1.ContentLibraryProvider, _ string,
						_ map[string]vmopv1.VirtualMachineImage) (*vmopv1.VirtualMachineImage, error) {
						return providerImg.DeepCopy(), nil
					}
					fakeVMProvider.Unlock()
				})

				JustAfterEach(func() {
					fakeVMProvider.Lock()
					fakeVMProvider.ListItemsFromContentLibraryFn = nil
					fakeVMProvider.GetVirtualMachineImageFromContentLibraryFn = nil
					fakeVMProvider.Unlock()
				})

				Context("another library with a duplicate image name is added", func() {
					BeforeEach(func() {
						existingImg.OwnerReferences = []metav1.OwnerReference{{
							APIVersion: "vmoperator.vmware.com/vmopv1",
							Kind:       "ContentLibraryProvider",
							Name:       "dummy-cl-2",
						}}
						existingImg.Spec.ImageID = "dummy-id-1"
						existingImg.Spec.ProviderRef = vmopv1.ContentProviderReference{
							Name: "dummy-cl-2",
						}
						existingImg.Status.ImageName = "dummy-image"
					})

					It("a new VirtualMachineImage should be created", func() {
						err := reconciler.SyncImagesFromContentProvider(ctx.Context, &cl)
						Expect(err).NotTo(HaveOccurred())

						imgList := &vmopv1.VirtualMachineImageList{}
						Expect(ctx.Client.List(ctx, imgList)).To(Succeed())
						Expect(len(imgList.Items)).To(Equal(2))
						if reflect.DeepEqual(imgList.Items[0].OwnerReferences, existingImg.OwnerReferences) {
							// Existing image should still have the expected non-generated name.
							Expect(imgList.Items[0]).To(Equal(*existingImg))
							Expect(imgList.Items[1].Name).To(ContainSubstring(providerConvertedImg.Name + "-"))
							Expect(imgList.Items[1].OwnerReferences).To(Equal(providerConvertedImg.OwnerReferences))
							Expect(imgList.Items[1].Spec).To(Equal(providerConvertedImg.Spec))
							Expect(imgList.Items[1].Status).To(Equal(providerConvertedImg.Status))
							Expect(imgList.Items[1].Annotations).To(Equal(providerConvertedImg.Annotations))
						} else {
							Expect(imgList.Items[1]).To(Equal(*existingImg))
							Expect(imgList.Items[0].Name).To(ContainSubstring(providerConvertedImg.Name + "-"))
							Expect(imgList.Items[0].OwnerReferences).To(Equal(providerConvertedImg.OwnerReferences))
							Expect(imgList.Items[0].Spec).To(Equal(providerConvertedImg.Spec))
							Expect(imgList.Items[0].Status).To(Equal(providerConvertedImg.Status))
							Expect(imgList.Items[0].Annotations).To(Equal(providerConvertedImg.Annotations))
						}
					})
				})

				When("the existing image does not have any ContentLibraryProvider OwnerRef", func() {
					It("the existing VirtualMachineImage is overwritten", func() {
						err := reconciler.SyncImagesFromContentProvider(ctx.Context, &cl)
						Expect(err).NotTo(HaveOccurred())

						imgList := &vmopv1.VirtualMachineImageList{}
						Expect(ctx.Client.List(ctx, imgList)).To(Succeed())
						Expect(len(imgList.Items)).To(Equal(1))

						vmImage := imgList.Items[0]
						// Existing image should still have the expected non-generated name.
						Expect(vmImage.Name).To(Equal(providerConvertedImg.Name))
						Expect(vmImage.Spec).To(Equal(providerConvertedImg.Spec))
						Expect(vmImage.Annotations).To(Equal(providerConvertedImg.Annotations))
						Expect(vmImage.OwnerReferences).To(Equal(providerConvertedImg.OwnerReferences))
					})
				})

				When("the existing image doesn't have Image ID and ProviderRef in Spec", func() {
					BeforeEach(func() {
						existingImg.OwnerReferences = []metav1.OwnerReference{{
							APIVersion: "vmoperator.vmware.com/vmopv1",
							Kind:       "ContentLibraryProvider",
							Name:       "dummy-cl",
						}}
					})

					It("the existing VirtualMachineImage is overwritten", func() {
						err := reconciler.SyncImagesFromContentProvider(ctx.Context, &cl)
						Expect(err).NotTo(HaveOccurred())

						imgList := &vmopv1.VirtualMachineImageList{}
						Expect(ctx.Client.List(ctx, imgList)).To(Succeed())
						Expect(len(imgList.Items)).To(Equal(1))

						vmImage := imgList.Items[0]
						// Existing image should still have the expected non-generated name.
						Expect(vmImage.Name).To(Equal(providerConvertedImg.Name))
						Expect(vmImage.Spec).To(Equal(providerConvertedImg.Spec))
						Expect(vmImage.Annotations).To(Equal(providerConvertedImg.Annotations))
						Expect(vmImage.OwnerReferences).To(Equal(providerConvertedImg.OwnerReferences))
					})
				})

				When("the existing image has a valid ContentLibraryProvider OwnerRef, providerRef and ImageID", func() {
					It("calls provider with the current image in map", func() {
						var called bool
						fakeVMProvider.Lock()
						fakeVMProvider.GetVirtualMachineImageFromContentLibraryFn = func(_ context.Context, _ *vmopv1.ContentLibraryProvider, _ string,
							_ map[string]vmopv1.VirtualMachineImage) (*vmopv1.VirtualMachineImage, error) {
							called = true
							return providerImg.DeepCopy(), nil
						}
						fakeVMProvider.Unlock()

						err := reconciler.SyncImagesFromContentProvider(ctx.Context, &cl)
						Expect(err).NotTo(HaveOccurred())
						Expect(called).To(BeTrue())
					})
				})

				When("Images have been deleted from the CL", func() {
					BeforeEach(func() {
						existingImg.Name = "dummy-image-existing"
						existingImg.OwnerReferences = []metav1.OwnerReference{{
							APIVersion: "vmoperator.vmware.com/vmopv1",
							Kind:       "ContentLibraryProvider",
							Name:       "dummy-cl",
						}}
						existingImg.Spec.ImageID = "dummy-id-1"
						existingImg.Spec.ProviderRef = vmopv1.ContentProviderReference{
							Name: "dummy-cl",
						}
						existingImg.Status.ImageName = "dummy-image-existing"
					})

					It("Should delete related VirtualMachineImages", func() {
						err := reconciler.SyncImagesFromContentProvider(ctx.Context, &cl)
						Expect(err).NotTo(HaveOccurred())

						imgList := &vmopv1.VirtualMachineImageList{}
						Expect(ctx.Client.List(ctx, imgList)).To(Succeed())
						Expect(len(imgList.Items)).To(Equal(1))
						Expect(imgList.Items[0].Name).To(Equal(providerConvertedImg.Name))
						Expect(imgList.Items[0].OwnerReferences).To(Equal(providerConvertedImg.OwnerReferences))
						Expect(imgList.Items[0].Spec).To(Equal(providerConvertedImg.Spec))
						Expect(imgList.Items[0].Status).To(Equal(providerConvertedImg.Status))
						Expect(imgList.Items[0].Annotations).To(Equal(providerConvertedImg.Annotations))
					})
				})
			})
		})
	})

	Context("DeleteImage", func() {
		var (
			images []vmopv1.VirtualMachineImage
			image  vmopv1.VirtualMachineImage
		)

		BeforeEach(func() {
			image = vmopv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-vm-image",
				},
			}
		})

		When(" the image doesnt exist", func() {
			BeforeEach(func() {
				images = append(images, image)
			})

			It("doesn't return an error", func() {
				err := reconciler.DeleteImage(ctx, image)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("ProcessItemFromContentLibrary", func() {
		var (
			itemID          = "dummy-id"
			currentCLImages = map[string]vmopv1.VirtualMachineImage{}
			vmImage         *vmopv1.VirtualMachineImage
		)

		Context("Failed to get VirtualMachineImage from content library", func() {
			It("Returns error", func() {
				fakeVMProvider.GetVirtualMachineImageFromContentLibraryFn = func(_ context.Context,
					_ *vmopv1.ContentLibraryProvider, _ string,
					_ map[string]vmopv1.VirtualMachineImage) (*vmopv1.VirtualMachineImage, error) {
					return nil, fmt.Errorf("failed to get virtual machine image")
				}
				err := reconciler.ProcessItemFromContentLibrary(ctx, ctx.Logger, &cl, itemID, currentCLImages)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).Should(ContainSubstring("failed to get virtual machine image"))
			})
		})

		Context("Get VirtualMachineImage from content library successfully", func() {
			JustBeforeEach(func() {
				vmImage = &vmopv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dummy-image",
					},
					Spec: vmopv1.VirtualMachineImageSpec{
						Type: "dummy-type-1",
					},
					Status: vmopv1.VirtualMachineImageStatus{
						ImageName: "dummy-image",
					},
				}

				fakeVMProvider.GetVirtualMachineImageFromContentLibraryFn = func(_ context.Context,
					_ *vmopv1.ContentLibraryProvider, _ string,
					_ map[string]vmopv1.VirtualMachineImage) (*vmopv1.VirtualMachineImage, error) {
					vmImage.Annotations = map[string]string{
						"dummy-key": "dummy-value",
					}
					vmImage.Status.ImageName = vmImage.Name
					return vmImage.DeepCopy(), nil
				}
			})

			It("Create a new VirtualMachineImage if VirtualMachineImage doesn't exist", func() {
				err := reconciler.ProcessItemFromContentLibrary(ctx, ctx.Logger, &cl, itemID, currentCLImages)
				Expect(err).ShouldNot(HaveOccurred())
			})

			When("Another image with the same name exists", func() {
				var existingImg *vmopv1.VirtualMachineImage
				JustBeforeEach(func() {
					existingImg = vmImage.DeepCopy()
					Expect(ctx.Client.Create(ctx, existingImg)).To(Succeed())
				})

				JustAfterEach(func() {
					Expect(ctx.Client.Delete(ctx, existingImg)).To(Succeed())
				})

				When("Existing image is from another CL", func() {
					It("Successfully creates the image with a different generated name if the existing", func() {
						err := reconciler.ProcessItemFromContentLibrary(ctx, ctx.Logger, &cl, itemID, currentCLImages)
						Expect(err).ShouldNot(HaveOccurred())

						imgs := vmopv1.VirtualMachineImageList{}
						Expect(ctx.Client.List(ctx, &imgs)).To(Succeed())
						Expect(imgs.Items).Should(HaveLen(2))
						if imgs.Items[0].Name == vmImage.Name {
							Expect(imgs.Items[1].Name).Should(ContainSubstring(vmImage.Name + "-"))
							Expect(imgs.Items[1].Status.ImageName).Should(Equal(vmImage.Name))
						} else {
							Expect(imgs.Items[0].Name).Should(ContainSubstring(vmImage.Name + "-"))
							Expect(imgs.Items[0].Status.ImageName).Should(Equal(vmImage.Name))
						}
					})
				})

				When("Existing image is from the same CL", func() {
					It("Update a new VirtualMachineImage if VirtualMachineImage exists", func() {
						img := &vmopv1.VirtualMachineImage{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vmImage), img)).To(Succeed())
						currentCLImages[vmImage.Status.ImageName] = *img

						err := reconciler.ProcessItemFromContentLibrary(ctx, ctx.Logger, &cl, itemID, currentCLImages)
						Expect(err).ShouldNot(HaveOccurred())

						img = &vmopv1.VirtualMachineImage{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vmImage), img)).To(Succeed())
						Expect(img.Annotations).NotTo(BeEmpty())
						Expect(img.Status.ImageName).To(Equal(vmImage.Name))
					})
				})
			})
		})
	})

	Context("Get VM image name", func() {
		var (
			img vmopv1.VirtualMachineImage
		)

		BeforeEach(func() {
			img = vmopv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-image",
				},
				Status: vmopv1.VirtualMachineImageStatus{
					ImageName: "dummy-image-1",
				},
			}
		})

		It("Should return Status.ImageName if it is not empty", func() {
			name := contentsource.GetVMImageName(img)
			Expect(name).Should(Equal(img.Status.ImageName))
		})

		It("Should return ObjectMeta.Name if Status.ImageName is empty", func() {
			img.Status.ImageName = ""
			name := contentsource.GetVMImageName(img)
			Expect(name).Should(Equal(img.Name))
		})
	})
}
