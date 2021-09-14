// Copyright (c) 2020-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentsource_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const finalizer = "contentsource.vmoperator.vmware.com"

func intgTests() {
	var (
		ctx *builder.IntegrationTestContext

		cs     vmopv1alpha1.ContentSource
		cl     vmopv1alpha1.ContentLibraryProvider
		csKey  types.NamespacedName
		clKey  types.NamespacedName
		imgKey types.NamespacedName

		img       vmopv1alpha1.VirtualMachineImage
		imageName = "dummy-image"
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		cl = vmopv1alpha1.ContentLibraryProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cl",
			},
		}
		cs = vmopv1alpha1.ContentSource{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cs",
			},
			Spec: vmopv1alpha1.ContentSourceSpec{
				ProviderRef: vmopv1alpha1.ContentProviderReference{
					APIVersion: "vmoperator.vmware.com/v1alpha1",
					Name:       cl.ObjectMeta.Name,
					Kind:       "ContentLibraryProvider",
				},
			},
		}

		img = vmopv1alpha1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: imageName,
			},
			Spec: vmopv1alpha1.VirtualMachineImageSpec{
				ImageID: "dummy-id",
			},
			Status: vmopv1alpha1.VirtualMachineImageStatus{
				ImageName: imageName,
			},
		}

		csKey = types.NamespacedName{Name: cs.ObjectMeta.Name}
		clKey = types.NamespacedName{Name: cl.ObjectMeta.Name}
		imgKey = types.NamespacedName{Name: img.ObjectMeta.Name}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	getContentSource := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1alpha1.ContentSource {
		cs := &vmopv1alpha1.ContentSource{}
		if err := ctx.Client.Get(ctx, objKey, cs); err != nil {
			return nil
		}
		return cs
	}

	waitForContentSourceFinalizer := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) {
		Eventually(func() []string {
			if cs := getContentSource(ctx, objKey); cs != nil {
				return cs.GetFinalizers()
			}
			return nil
		}).Should(ContainElement(finalizer), "waiting for ContentSource finalizer")
	}

	getContentLibraryProvider := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1alpha1.ContentLibraryProvider {
		cl := &vmopv1alpha1.ContentLibraryProvider{}
		if err := ctx.Client.Get(ctx, objKey, cl); err != nil {
			return nil
		}

		return cl
	}

	waitForCLProviderOwnerReference := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName, ownerRef metav1.OwnerReference) {
		Eventually(func() []metav1.OwnerReference {
			if cl := getContentLibraryProvider(ctx, objKey); cl != nil {
				return cl.OwnerReferences
			}
			return []metav1.OwnerReference{}
		}).Should(ContainElement(ownerRef), "waiting for ContentSource OwnerRef on the ContentLibraryProvider resource")
	}

	getVirtualMachineImage := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1alpha1.VirtualMachineImage {
		img := &vmopv1alpha1.VirtualMachineImage{}
		if err := ctx.Client.Get(ctx, objKey, img); err != nil {
			return nil
		}

		return img
	}

	waitForVirtualMachineImage := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName,
		expectedImg vmopv1alpha1.VirtualMachineImage) {
		Eventually(func() bool {
			image := getVirtualMachineImage(ctx, objKey)
			if image == nil {
				return false
			}
			return reflect.DeepEqual(image.OwnerReferences, expectedImg.OwnerReferences) &&
				reflect.DeepEqual(image.Spec, expectedImg.Spec) &&
				reflect.DeepEqual(image.Status, expectedImg.Status)
		}).Should(BeTrue())
	}

	populateExpectedImg := func(image vmopv1alpha1.VirtualMachineImage,
		cl *vmopv1alpha1.ContentLibraryProvider) vmopv1alpha1.VirtualMachineImage {
		expectedImg := image
		expectedImg.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: "vmoperator.vmware.com/v1alpha1",
			Kind:       "ContentLibraryProvider",
			Name:       cl.Name,
			UID:        cl.UID,
		}}
		expectedImg.Spec.ProviderRef = vmopv1alpha1.ContentProviderReference{
			APIVersion: "vmoperator.vmware.com/v1alpha1",
			Kind:       "ContentLibraryProvider",
			Name:       cl.Name,
		}
		return expectedImg
	}

	Context("Reconcile ContentSource", func() {
		When("ContentSource and ContentLibraryProvider exists", func() {
			BeforeEach(func() {
				Expect(ctx.Client.Create(ctx, &cl)).To(Succeed())
				Expect(ctx.Client.Create(ctx, &cs)).To(Succeed())

				intgFakeVMProvider.Lock()
				intgFakeVMProvider.ListVirtualMachineImagesFromContentLibraryFn = func(_ context.Context,
					_ vmopv1alpha1.ContentLibraryProvider, _ map[string]vmopv1alpha1.VirtualMachineImage) (
					[]*vmopv1alpha1.VirtualMachineImage, error) {
					// use DeepCopy to avoid race
					return []*vmopv1alpha1.VirtualMachineImage{img.DeepCopy()}, nil
				}
				intgFakeVMProvider.Unlock()
			})

			AfterEach(func() {
				err := ctx.Client.Delete(ctx, &cl)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())

				err = ctx.Client.Delete(ctx, &cs)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			})

			It("Reconciles after ContentSource creation", func() {
				By("ContentSource should have a finalizer added", func() {
					waitForContentSourceFinalizer(ctx, csKey)
				})

				By("ContentLibraryProvider should have OwnerReference set", func() {
					csObj := getContentSource(ctx, csKey)
					Expect(csObj).ToNot(BeNil())
					isController := true
					ownerRef := metav1.OwnerReference{
						// Not sure why we have to set these manually. csObj.APIVersion is "".
						APIVersion: "vmoperator.vmware.com/v1alpha1",
						Kind:       "ContentSource",
						Name:       csObj.Name,
						UID:        csObj.UID,
						Controller: &isController,
					}
					waitForCLProviderOwnerReference(ctx, clKey, ownerRef)
				})

				By("VirtualMachineImage should be created", func() {
					clObj := getContentLibraryProvider(ctx, clKey)
					Expect(clObj).ToNot(BeNil())
					expectedImg := populateExpectedImg(img, clObj)
					waitForVirtualMachineImage(ctx, imgKey, expectedImg)
				})
			})

			When("Image list in the CL has been updated", func() {
				It("Should add new VirtualMachineImage and remove old VirtualMachineImage", func() {
					clObj := getContentLibraryProvider(ctx, clKey)
					Expect(clObj).ToNot(BeNil())
					expectedImg := populateExpectedImg(img, clObj)
					waitForVirtualMachineImage(ctx, imgKey, expectedImg)

					newImg := vmopv1alpha1.VirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{
							Name: "new-dummy-name",
						},
						Spec: vmopv1alpha1.VirtualMachineImageSpec{
							ImageID: "new-dummy-id",
						},
						Status: vmopv1alpha1.VirtualMachineImageStatus{
							ImageName: "new-dummy-name",
						},
					}
					intgFakeVMProvider.Lock()
					intgFakeVMProvider.ListVirtualMachineImagesFromContentLibraryFn = func(_ context.Context,
						_ vmopv1alpha1.ContentLibraryProvider, _ map[string]vmopv1alpha1.VirtualMachineImage) (
						[]*vmopv1alpha1.VirtualMachineImage, error) {
						return []*vmopv1alpha1.VirtualMachineImage{newImg.DeepCopy()}, nil
					}
					intgFakeVMProvider.Unlock()

					// Trigger ContentSource reconcile
					csObj := getContentSource(ctx, csKey)
					csObj.Annotations = map[string]string{
						"dummy-key": "dummy-value",
					}
					Expect(ctx.Client.Update(ctx, csObj)).To(Succeed())

					By("A new VirtualMachineImage object should be created", func() {
						clObj := getContentLibraryProvider(ctx, clKey)
						Expect(clObj).ToNot(BeNil())
						expectedImg = populateExpectedImg(newImg, clObj)
						waitForVirtualMachineImage(ctx, types.NamespacedName{Name: newImg.ObjectMeta.Name}, expectedImg)
					})

					By("The old VirtualMachineImage should be removed", func() {
						Eventually(func() bool {
							return getVirtualMachineImage(ctx, imgKey) == nil
						})
					})
				})
			})

			When("a new ContentSource with duplicate vm images is created", func() {
				newCL := vmopv1alpha1.ContentLibraryProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dummy-cl-new",
					},
				}
				newCS := vmopv1alpha1.ContentSource{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dummy-cs-new",
					},
					Spec: vmopv1alpha1.ContentSourceSpec{
						ProviderRef: vmopv1alpha1.ContentProviderReference{
							APIVersion: "vmoperator.vmware.com/v1alpha1",
							Name:       newCL.ObjectMeta.Name,
							Kind:       "ContentLibraryProvider",
						},
					},
				}

				newCLKey := types.NamespacedName{Name: newCL.Name}

				BeforeEach(func() {
					// Wait for the first vm image to be created
					clObj := getContentLibraryProvider(ctx, clKey)
					Expect(clObj).ToNot(BeNil())
					expectedImg := populateExpectedImg(img, clObj)
					waitForVirtualMachineImage(ctx, imgKey, expectedImg)

					Expect(ctx.Client.Create(ctx, &newCL)).To(Succeed())
					Expect(ctx.Client.Create(ctx, &newCS)).To(Succeed())
				})

				AfterEach(func() {
					err := ctx.Client.Delete(ctx, &newCL)
					Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())

					err = ctx.Client.Delete(ctx, &newCS)
					Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
				})

				It("should reconcile and generate a new VirtualMachineImage object", func() {
					images := &vmopv1alpha1.VirtualMachineImageList{}
					Eventually(func() int {
						if err := ctx.Client.List(ctx, images); err != nil {
							return 0
						}
						return len(images.Items)
					}).Should(Equal(2))

					newCLObj := getContentLibraryProvider(ctx, newCLKey)
					Expect(newCLObj).ToNot(BeNil())

					var generatedName string
					if images.Items[0].Name == imageName {
						generatedName = images.Items[1].Name
					} else {
						generatedName = images.Items[0].Name
					}

					Expect(generatedName).Should(HavePrefix(imageName + "-"))
					expectedImg := populateExpectedImg(img, newCLObj)
					waitForVirtualMachineImage(ctx, types.NamespacedName{Name: generatedName}, expectedImg)
				})
			})
		})
	})
}
