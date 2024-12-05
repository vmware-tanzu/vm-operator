// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
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
			img := builder.DummyVirtualMachineImage(id)
			img.Namespace = actualNamespace
			img.Status.Name = name
			return img
		}

		newClImgFn := func(id, name string) *vmopv1.ClusterVirtualMachineImage {
			img := builder.DummyClusterVirtualMachineImage(id)
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

var _ = DescribeTable("DetermineHardwareVersion",
	func(
		vm vmopv1.VirtualMachine,
		configSpec vimtypes.VirtualMachineConfigSpec,
		imgStatus vmopv1.VirtualMachineImageStatus,
		expected vimtypes.HardwareVersion,
	) {
		Ω(vmopv1util.DetermineHardwareVersion(vm, configSpec, imgStatus)).Should(Equal(expected))
	},
	Entry(
		"empty inputs",
		vmopv1.VirtualMachine{},
		vimtypes.VirtualMachineConfigSpec{},
		vmopv1.VirtualMachineImageStatus{},
		vimtypes.HardwareVersion(0),
	),
	Entry(
		"spec.minHardwareVersion is 11",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				MinHardwareVersion: 11,
			},
		},
		vimtypes.VirtualMachineConfigSpec{},
		vmopv1.VirtualMachineImageStatus{},
		vimtypes.HardwareVersion(11),
	),
	Entry(
		"spec.minHardwareVersion is 11, configSpec.version is 13",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				MinHardwareVersion: 11,
			},
		},
		vimtypes.VirtualMachineConfigSpec{
			Version: "vmx-13",
		},
		vmopv1.VirtualMachineImageStatus{},
		vimtypes.HardwareVersion(13),
	),
	Entry(
		"spec.minHardwareVersion is 11, configSpec.version is invalid",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				MinHardwareVersion: 11,
			},
		},
		vimtypes.VirtualMachineConfigSpec{
			Version: "invalid",
		},
		vmopv1.VirtualMachineImageStatus{},
		vimtypes.HardwareVersion(11),
	),
	Entry(
		"spec.minHardwareVersion is 11, configSpec has pci pass-through",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				MinHardwareVersion: 11,
			},
		},
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualPCIPassthrough{},
				},
			},
		},
		vmopv1.VirtualMachineImageStatus{},
		pkgconst.MinSupportedHWVersionForPCIPassthruDevices,
	),
	Entry(
		"spec.minHardwareVersion is 11, vm has pvc",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				MinHardwareVersion: 11,
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{},
						},
					},
				},
			},
		},
		vimtypes.VirtualMachineConfigSpec{},
		vmopv1.VirtualMachineImageStatus{},
		pkgconst.MinSupportedHWVersionForPVC,
	),
	Entry(
		"spec.minHardwareVersion is 11, configSpec has pci pass-through, image version is 20",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				MinHardwareVersion: 11,
			},
		},
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualPCIPassthrough{},
				},
			},
		},
		vmopv1.VirtualMachineImageStatus{
			HardwareVersion: &[]int32{20}[0],
		},
		vimtypes.HardwareVersion(20),
	),
	Entry(
		"spec.minHardwareVersion is 11, vm has pvc, image version is 20",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				MinHardwareVersion: 11,
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{},
						},
					},
				},
			},
		},
		vimtypes.VirtualMachineConfigSpec{},
		vmopv1.VirtualMachineImageStatus{
			HardwareVersion: &[]int32{20}[0],
		},
		vimtypes.HardwareVersion(20),
	),
	Entry(
		"spec.minHardwareVersion is 11, vm has vTPM, image version is 10",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				MinHardwareVersion: 11,
			},
		},
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualTPM{},
				},
			},
		},
		vmopv1.VirtualMachineImageStatus{
			HardwareVersion: &[]int32{10}[0],
		},
		pkgconst.MinSupportedHWVersionForVTPM,
	),
)

var _ = DescribeTable("HasPVC",
	func(
		vm vmopv1.VirtualMachine,
		expected bool,
	) {
		Ω(vmopv1util.HasPVC(vm)).Should(Equal(expected))
	},
	Entry(
		"spec.volumes is empty",
		vmopv1.VirtualMachine{},
		false,
	),
	Entry(
		"spec.volumes is non-empty with no pvcs",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						Name: "hello",
					},
				},
			},
		},
		false,
	),
	Entry(
		"spec.volumes is non-empty with at least one pvc",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						Name: "hello",
					},
					{
						Name: "world",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{},
						},
					},
				},
			},
		},
		true,
	),
)

var _ = DescribeTable("IsClasslessVM",
	func(
		vm vmopv1.VirtualMachine,
		expected bool,
	) {
		Ω(vmopv1util.IsClasslessVM(vm)).Should(Equal(expected))
	},
	Entry(
		"spec.className is empty",
		vmopv1.VirtualMachine{},
		true,
	),
	Entry(
		"spec.className is non-empty",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				ClassName: "small",
			},
		},
		false,
	),
)

var _ = DescribeTable("IsImageLessVM",
	func(
		vm vmopv1.VirtualMachine,
		expected bool,
	) {
		Ω(vmopv1util.IsImagelessVM(vm)).Should(Equal(expected))
	},
	Entry(
		"spec.image is nil and spec.imageName is empty",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Image:     nil,
				ImageName: "",
			},
		},
		true,
	),
	Entry(
		"spec.image is not nil and spec.imageName is empty",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Image:     &vmopv1.VirtualMachineImageRef{},
				ImageName: "",
			},
		},
		false,
	),
	Entry(
		"spec.image is nil and spec.imageName is non-empty",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Image:     nil,
				ImageName: "non-empty",
			},
		},
		false,
	),
	Entry(
		"spec.image is not nil and spec.imageName is non-empty",
		vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Image:     &vmopv1.VirtualMachineImageRef{},
				ImageName: "non-empty",
			},
		},
		false,
	),
)

var _ = DescribeTable("ImageRefsEqual",
	func(
		ref1 *vmopv1.VirtualMachineImageRef,
		ref2 *vmopv1.VirtualMachineImageRef,
		expected bool,
	) {
		Ω(vmopv1util.ImageRefsEqual(ref1, ref2)).Should(Equal(expected))
	},
	Entry(
		"both refs nil",
		nil,
		nil,
		true,
	),
	Entry(
		"ref1 is nil, ref2 is not",
		nil,
		&vmopv1.VirtualMachineImageRef{
			Name: "dummy",
		},
		false,
	),
	Entry(
		"ref1 is not nil, ref2 is nil",
		&vmopv1.VirtualMachineImageRef{
			Name: "dummy",
		},
		nil,
		false,
	),
	Entry(
		"both not nil, one containing an extra field",
		&vmopv1.VirtualMachineImageRef{
			Name: "dummy",
		},
		&vmopv1.VirtualMachineImageRef{
			Name: "dummy",
			Kind: "ClusterVirtualMachineImage",
		},
		false,
	),
	Entry(
		"both not nil, same values",
		&vmopv1.VirtualMachineImageRef{
			Name: "dummy",
		},
		&vmopv1.VirtualMachineImageRef{
			Name: "dummy",
		},
		true,
	),
)

var _ = Describe("SyncStorageUsageForNamespace", func() {
	var (
		ctx          context.Context
		namespace    string
		storageClass string
		chanEvent    chan event.GenericEvent
	)
	BeforeEach(func() {
		ctx = cource.NewContext()
		namespace = "my-namespace"
		storageClass = "my-storage-class"
		chanEvent = spqutil.FromContext(ctx)
	})
	JustBeforeEach(func() {
		vmopv1util.SyncStorageUsageForNamespace(ctx, namespace, storageClass)
	})
	When("namespace is empty", func() {
		BeforeEach(func() {
			namespace = ""
		})
		Specify("no event should be received", func() {
			Consistently(chanEvent).ShouldNot(Receive())
		})
	})
	When("storageClassName is empty", func() {
		BeforeEach(func() {
			storageClass = ""
		})
		Specify("no event should be received", func() {
			Consistently(chanEvent).ShouldNot(Receive())
		})
	})
	When("namespace and storageClassName are both non-empty", func() {
		Specify("an event should be received", func() {
			e := <-chanEvent
			Expect(e).ToNot(BeNil())
			Expect(e.Object).ToNot(BeNil())
			Expect(e.Object.GetNamespace()).To(Equal(namespace))
			Expect(e.Object.GetName()).To(Equal(storageClass))
		})
	})
})

var _ = Describe("EncryptionClassToVirtualMachineMapper", func() {
	const (
		encryptionClassName = "my-encryption-class"
		keyProviderID       = "my-key-provider-id"
		keyID               = "my-key-id"
		namespaceName       = "fake"
	)

	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		withObjs  []ctrlclient.Object
		withFuncs interceptor.Funcs
		obj       ctrlclient.Object
		mapFn     handler.MapFunc
		mapFnCtx  context.Context
		mapFnObj  ctrlclient.Object
		reqs      []reconcile.Request
	)
	BeforeEach(func() {
		reqs = nil
		withObjs = nil
		withFuncs = interceptor.Funcs{}

		ctx = context.Background()
		mapFnCtx = ctx

		obj = &byokv1.EncryptionClass{
			ObjectMeta: metav1.ObjectMeta{
				Name:      encryptionClassName,
				Namespace: namespaceName,
			},
			Spec: byokv1.EncryptionClassSpec{
				KeyProvider: keyProviderID,
				KeyID:       keyID,
			},
		}
		mapFnObj = obj
	})
	JustBeforeEach(func() {
		withObjs = append(withObjs, obj)
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
		Expect(k8sClient).ToNot(BeNil())
	})
	When("panic is expected", func() {
		When("ctx is nil", func() {
			JustBeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				Expect(func() {
					_ = vmopv1util.EncryptionClassToVirtualMachineMapper(
						ctx,
						k8sClient)
				}).To(PanicWith("context is nil"))
			})
		})
		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				Expect(func() {
					_ = vmopv1util.EncryptionClassToVirtualMachineMapper(
						ctx,
						k8sClient)
				}).To(PanicWith("k8sClient is nil"))
			})
		})
		Context("mapFn", func() {
			JustBeforeEach(func() {
				mapFn = vmopv1util.EncryptionClassToVirtualMachineMapper(
					ctx,
					k8sClient)
				Expect(mapFn).ToNot(BeNil())
			})
			When("ctx is nil", func() {
				BeforeEach(func() {
					mapFnCtx = nil
				})
				It("should panic", func() {
					Expect(func() {
						_ = mapFn(mapFnCtx, mapFnObj)
					}).To(PanicWith("context is nil"))
				})
			})
			When("object is nil", func() {
				BeforeEach(func() {
					mapFnObj = nil
				})
				It("should panic", func() {
					Expect(func() {
						_ = mapFn(mapFnCtx, mapFnObj)
					}).To(PanicWith("object is nil"))
				})
			})
			When("object is invalid", func() {
				BeforeEach(func() {
					mapFnObj = &vmopv1.VirtualMachine{}
				})
				It("should panic", func() {
					Expect(func() {
						_ = mapFn(mapFnCtx, mapFnObj)
					}).To(PanicWith(fmt.Sprintf("object is %T", mapFnObj)))
				})
			})
		})
	})
	When("panic is not expected", func() {
		JustBeforeEach(func() {
			mapFn = vmopv1util.EncryptionClassToVirtualMachineMapper(
				ctx,
				k8sClient)
			Expect(mapFn).ToNot(BeNil())
			reqs = mapFn(mapFnCtx, mapFnObj)
			_ = reqs
		})
		When("there is an error listing vms", func() {
			BeforeEach(func() {
				withFuncs.List = func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					list ctrlclient.ObjectList,
					opts ...ctrlclient.ListOption) error {

					if _, ok := list.(*vmopv1.VirtualMachineList); ok {
						return errors.New("fake")
					}
					return client.List(ctx, list, opts...)
				}
			})
			Specify("no reconcile requests should be returned", func() {
				Expect(reqs).To(BeEmpty())
			})
		})
		When("there are no matching vms", func() {
			BeforeEach(func() {
				withObjs = append(withObjs,
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-1",
						},
					},
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-2",
						},
						Spec: vmopv1.VirtualMachineSpec{
							Crypto: &vmopv1.VirtualMachineCryptoSpec{},
						},
					},
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-3",
						},
						Spec: vmopv1.VirtualMachineSpec{
							Crypto: &vmopv1.VirtualMachineCryptoSpec{
								EncryptionClassName: encryptionClassName + "1",
							},
						},
					},
				)
			})
			Specify("no reconcile requests should be returned", func() {
				Expect(reqs).To(BeEmpty())
			})
		})
		When("there is a single matching vm", func() {
			BeforeEach(func() {
				withObjs = append(withObjs,
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-1",
						},
					},
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-2",
						},
						Spec: vmopv1.VirtualMachineSpec{
							Crypto: &vmopv1.VirtualMachineCryptoSpec{},
						},
					},
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-3",
						},
						Spec: vmopv1.VirtualMachineSpec{
							Crypto: &vmopv1.VirtualMachineCryptoSpec{
								EncryptionClassName: encryptionClassName,
							},
						},
					},
				)
			})
			Specify("one reconcile request should be returned", func() {
				Expect(reqs).To(ConsistOf(
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: namespaceName,
							Name:      "vm-3",
						},
					},
				))
			})
		})
		When("there are multiple matching vms", func() {
			BeforeEach(func() {
				withObjs = append(withObjs,
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-1",
						},
					},
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-2",
						},
						Spec: vmopv1.VirtualMachineSpec{
							Crypto: &vmopv1.VirtualMachineCryptoSpec{},
						},
					},
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-3",
						},
						Spec: vmopv1.VirtualMachineSpec{
							Crypto: &vmopv1.VirtualMachineCryptoSpec{
								EncryptionClassName: encryptionClassName,
							},
						},
					},
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-4",
						},
						Spec: vmopv1.VirtualMachineSpec{
							Crypto: &vmopv1.VirtualMachineCryptoSpec{
								EncryptionClassName: encryptionClassName,
							},
						},
					},
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-5",
						},
						Spec: vmopv1.VirtualMachineSpec{
							Crypto: &vmopv1.VirtualMachineCryptoSpec{
								EncryptionClassName: encryptionClassName,
							},
						},
					},
				)
			})
			Specify("an equal number of requests should be returned", func() {
				Expect(reqs).To(ConsistOf(
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: namespaceName,
							Name:      "vm-4",
						},
					},
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: namespaceName,
							Name:      "vm-5",
						},
					},
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: namespaceName,
							Name:      "vm-3",
						},
					},
				))
			})
		})
	})
})
