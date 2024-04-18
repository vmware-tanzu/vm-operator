// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("ErrImageNotFound", func() {
	It("should return true from apierrors.IsNotFound", func() {
		Expect(apierrors.IsNotFound(vmopv1util.ErrImageNotFound{})).To(BeTrue())
	})
})

var _ = Describe("ResolveImageName", func() {

	const (
		actualNamespace = "my-namespace"

		nsImg1ID   = "vmi-1"
		nsImg1Name = "image-a"

		nsImg2ID   = "vmi-2"
		nsImg2Name = "image-b"

		nsImg3ID   = "vmi-3"
		nsImg3Name = "image-b"

		nsImg4ID   = "vmi-4"
		nsImg4Name = "image-c"

		clImg1ID   = "vmi-5"
		clImg1Name = "image-d"

		clImg2ID   = "vmi-6"
		clImg2Name = "image-e"

		clImg3ID   = "vmi-7"
		clImg3Name = "image-e"

		clImg4ID   = "vmi-8"
		clImg4Name = "image-c"
	)

	var (
		name      string
		namespace string
		client    ctrlclient.Client
		err       error
		obj       ctrlclient.Object
	)

	BeforeEach(func() {
		namespace = actualNamespace

		newNsImgFn := func(id, name string) *vmopv1.VirtualMachineImage {
			img := builder.DummyVirtualMachineImageA2(id)
			img.Namespace = actualNamespace
			img.Status.Name = name
			return img
		}

		newClImgFn := func(id, name string) *vmopv1.ClusterVirtualMachineImage {
			img := builder.DummyClusterVirtualMachineImageA2(id)
			img.Status.Name = name
			return img
		}

		// Replace the client with a fake client that has the index of VM images.
		client = fake.NewClientBuilder().WithScheme(builder.NewScheme()).
			WithIndex(
				&vmopv1.VirtualMachineImage{},
				"status.name",
				func(rawObj ctrlclient.Object) []string {
					image := rawObj.(*vmopv1.VirtualMachineImage)
					return []string{image.Status.Name}
				}).
			WithIndex(&vmopv1.ClusterVirtualMachineImage{},
				"status.name",
				func(rawObj ctrlclient.Object) []string {
					image := rawObj.(*vmopv1.ClusterVirtualMachineImage)
					return []string{image.Status.Name}
				}).
			WithObjects(
				newNsImgFn(nsImg1ID, nsImg1Name),
				newNsImgFn(nsImg2ID, nsImg2Name),
				newNsImgFn(nsImg3ID, nsImg3Name),
				newNsImgFn(nsImg4ID, nsImg4Name),
				newClImgFn(clImg1ID, clImg1Name),
				newClImgFn(clImg2ID, clImg2Name),
				newClImgFn(clImg3ID, clImg3Name),
				newClImgFn(clImg4ID, clImg4Name),
			).
			Build()
	})

	JustBeforeEach(func() {
		obj, err = vmopv1util.ResolveImageName(
			context.Background(), client, namespace, name)
	})

	When("name is vmi", func() {
		When("no image exists", func() {
			const missingVmi = "vmi-9999999"
			BeforeEach(func() {
				name = missingVmi
			})
			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
				Expect(err).To(BeAssignableToTypeOf(vmopv1util.ErrImageNotFound{}))
				Expect(err.Error()).To(Equal(fmt.Sprintf("no VM image exists for %q in namespace or cluster scope", missingVmi)))
				Expect(obj).To(BeNil())
			})
		})
		When("img is namespace-scoped", func() {
			BeforeEach(func() {
				name = nsImg1ID
			})
			It("should return image ref", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(obj).To(BeAssignableToTypeOf(&vmopv1.VirtualMachineImage{}))
				img := obj.(*vmopv1.VirtualMachineImage)
				Expect(img.Name).To(Equal(nsImg1ID))
			})
		})
		When("img is cluster-scoped", func() {
			BeforeEach(func() {
				name = clImg1ID
			})
			It("should return image ref", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(obj).To(BeAssignableToTypeOf(&vmopv1.ClusterVirtualMachineImage{}))
				img := obj.(*vmopv1.ClusterVirtualMachineImage)
				Expect(img.Name).To(Equal(clImg1ID))
			})
		})
	})

	When("name is display name", func() {
		BeforeEach(func() {
			name = nsImg1Name
		})
		It("should return image ref", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(obj).To(BeAssignableToTypeOf(&vmopv1.VirtualMachineImage{}))
			img := obj.(*vmopv1.VirtualMachineImage)
			Expect(img.Name).To(Equal(nsImg1ID))
		})
	})
	When("name is empty", func() {
		BeforeEach(func() {
			name = ""
		})
		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("imgName is empty"))
			Expect(obj).To(BeNil())
		})
	})

	When("name matches multiple, namespaced-scoped images", func() {
		BeforeEach(func() {
			name = nsImg2Name
		})
		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(fmt.Sprintf("multiple VM images exist for %q in namespace scope", nsImg2Name)))
			Expect(obj).To(BeNil())
		})
	})

	When("name matches multiple, cluster-scoped images", func() {
		BeforeEach(func() {
			name = clImg2Name
		})
		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(fmt.Sprintf("multiple VM images exist for %q in cluster scope", clImg2Name)))
			Expect(obj).To(BeNil())
		})
	})

	When("name matches both namespace and cluster-scoped images", func() {
		BeforeEach(func() {
			name = clImg4Name
		})
		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(fmt.Sprintf("multiple VM images exist for %q in namespace and cluster scope", clImg4Name)))
			Expect(obj).To(BeNil())
		})
	})

	When("name does not match any namespace or cluster-scoped images", func() {
		const invalidImageID = "invalid"
		BeforeEach(func() {
			name = invalidImageID
		})
		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			Expect(err).To(BeAssignableToTypeOf(vmopv1util.ErrImageNotFound{}))
			Expect(err.Error()).To(Equal(fmt.Sprintf("no VM image exists for %q in namespace or cluster scope", invalidImageID)))
			Expect(obj).To(BeNil())
		})
	})

	When("name matches a single namespace-scoped image", func() {
		BeforeEach(func() {
			name = nsImg1Name
		})
		It("should return image ref", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(obj).To(BeAssignableToTypeOf(&vmopv1.VirtualMachineImage{}))
			img := obj.(*vmopv1.VirtualMachineImage)
			Expect(img.Name).To(Equal(nsImg1ID))
		})
	})

	When("name matches a single cluster-scoped image", func() {
		BeforeEach(func() {
			name = clImg1Name
		})
		It("should return image ref", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(obj).To(BeAssignableToTypeOf(&vmopv1.ClusterVirtualMachineImage{}))
			img := obj.(*vmopv1.ClusterVirtualMachineImage)
			Expect(img.Name).To(Equal(clImg1ID))
		})
	})
})
