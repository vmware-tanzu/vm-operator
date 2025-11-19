// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package register_test

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/find"
	pbmmethods "github.com/vmware/govmomi/pbm/methods"
	pbmsim "github.com/vmware/govmomi/pbm/simulator"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	unmanagedvolsfill "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/volumes/unmanaged/backfill"
	unmanagedvolsreg "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/volumes/unmanaged/register"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("New", func() {
	It("should return a reconciler", func() {
		Expect(unmanagedvolsreg.New()).ToNot(BeNil())
	})
})

var _ = Describe("Name", func() {
	It("should return 'unmanagedvolumes-register'", func() {
		Expect(unmanagedvolsreg.New().Name()).To(Equal("unmanagedvolumes-register"))
	})
})

var _ = Describe("OnResult", func() {
	It("should return nil", func() {
		var ctx context.Context
		Expect(unmanagedvolsreg.New().OnResult(ctx, nil, mo.VirtualMachine{}, nil)).To(Succeed())
	})
})

var _ = Describe("Reconcile", func() {

	var (
		r          vmconfig.Reconciler
		ctx        context.Context
		k8sClient  ctrlclient.Client
		vimClient  *vim25.Client
		moVM       mo.VirtualMachine
		vm         *vmopv1.VirtualMachine
		withObjs   []ctrlclient.Object
		withFuncs  interceptor.Funcs
		configSpec *vimtypes.VirtualMachineConfigSpec
		httpPrefix string
		vcSimCtx   *builder.TestContextForVCSim
	)

	BeforeEach(func() {
		r = unmanagedvolsreg.New()

		vcSimCtx = builder.NewTestContextForVCSim(
			ctxop.WithContext(pkgcfg.NewContextWithDefaultConfig()), builder.VCSimTestConfig{})
		ctx = vmconfig.WithContext(vcSimCtx)
		ctx = logr.NewContext(ctx, klog.Background())
		vimClient = vcSimCtx.VCClient.Client
		ctx = pkgctx.WithFinder(ctx, find.NewFinder(vimClient))

		httpPrefix = fmt.Sprintf("https://%s", vimClient.URL().Host)

		moVM = mo.VirtualMachine{
			Config:   &vimtypes.VirtualMachineConfigInfo{},
			LayoutEx: &vimtypes.VirtualMachineFileLayoutEx{},
		}

		configSpec = &vimtypes.VirtualMachineConfigSpec{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "my-namespace",
				Name:        "my-vm",
				Annotations: map[string]string{},
			},
			Spec: vmopv1.VirtualMachineSpec{
				StorageClass: "my-storage-class-1",
			},
			Status: vmopv1.VirtualMachineStatus{
				UniqueID: "vm-1",
			},
		}

		withFuncs = interceptor.Funcs{}
		withObjs = []ctrlclient.Object{}
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
	})

	Context("a panic is expected", func() {
		When("ctx is nil", func() {
			JustBeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})
		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("k8sClient is nil"))
			})
		})
		When("vimClient is nil", func() {
			JustBeforeEach(func() {
				vimClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vimClient is nil"))
			})
		})
		When("vm is nil", func() {
			JustBeforeEach(func() {
				vm = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})
	})

	When("no panic is expected", func() {

		var mockProfileResults []pbmtypes.PbmQueryProfileResult

		BeforeEach(func() {
			withObjs = append(withObjs,
				&storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-storage-class-1-latebinding",
					},
					Parameters: map[string]string{
						"storagePolicyID": "profile-123",
					},
				},
				&storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-storage-class-1",
					},
					Parameters: map[string]string{
						"storagePolicyID": "profile-123",
					},
				})

			mockProfileResults = []pbmtypes.PbmQueryProfileResult{
				{
					Object: pbmtypes.PbmServerObjectRef{
						Key: "vm-1:300",
					},
					ProfileId: []pbmtypes.PbmProfileId{
						{
							UniqueId: "profile-123",
						},
					},
				},
				{
					Object: pbmtypes.PbmServerObjectRef{
						Key: "vm-1:301",
					},
					ProfileId: []pbmtypes.PbmProfileId{
						{
							UniqueId: "profile-123",
						},
					},
				},
				{
					Object: pbmtypes.PbmServerObjectRef{
						Key: "vm-1:302",
					},
					ProfileId: []pbmtypes.PbmProfileId{
						{
							UniqueId: "profile-123",
						},
					},
				},
				{
					Object: pbmtypes.PbmServerObjectRef{
						Key: "vm-1:303",
					},
					ProfileId: []pbmtypes.PbmProfileId{
						{
							UniqueId: "profile-123",
						},
					},
				},
				{
					Object: pbmtypes.PbmServerObjectRef{
						Key: "vm-1:304",
					},
					ProfileId: []pbmtypes.PbmProfileId{
						{
							UniqueId: "profile-123",
						},
					},
				},
				{
					Object: pbmtypes.PbmServerObjectRef{
						Key: "vm-1:305",
					},
					ProfileId: []pbmtypes.PbmProfileId{
						{
							UniqueId: "profile-123",
						},
					},
				},
				{
					Object: pbmtypes.PbmServerObjectRef{
						Key: "vm-1:306",
					},
					ProfileId: []pbmtypes.PbmProfileId{
						{
							UniqueId: "profile-123",
						},
					},
				},
			}
		})

		JustBeforeEach(func() {
			vcSimCtx.SimulatorContext().For("/pbm").Map.Handler = func(
				ctx *simulator.Context,
				m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

				if m.Name == "PbmQueryAssociatedProfiles" {
					return &fakeProfileManager{
						ProfileManager: &pbmsim.ProfileManager{},
						Result:         mockProfileResults,
					}, nil
				}

				return nil, nil
			}
		})

		Context("early return conditions", func() {
			When("moVM.Config is nil", func() {
				JustBeforeEach(func() {
					moVM.Config = nil
				})
				It("should return nil without error", func() {
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(Succeed())
				})
			})

			When("moVM.Config.Hardware.Device is empty", func() {
				JustBeforeEach(func() {
					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{},
						},
					}
				})
				It("should return nil without error", func() {
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(Succeed())
				})
			})
		})

		Context("PVC creation and management", func() {

			When("VM has unmanaged disks with no sharing", func() {
				JustBeforeEach(func() {
					Expect(unmanagedvolsfill.Reconcile(
						ctx,
						nil,
						nil,
						vm,
						moVM,
						nil)).To(MatchError(unmanagedvolsfill.ErrPendingBackfill))
					Expect(unmanagedvolsfill.Reconcile(
						ctx,
						nil,
						nil,
						vm,
						moVM,
						nil)).To(Succeed())
				})

				BeforeEach(func() {
					// Start with empty volumes - let Reconcile add them
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 200,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/regular-disk.vmdk",
								},
								Uuid: "disk-uuid-456",
							},
						},
						CapacityInBytes: 2 * 1024 * 1024 * 1024,
					}

					scsiController := &vimtypes.VirtualSCSIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 200,
							},
							BusNumber: 1,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								scsiController,
								disk,
							},
						},
					}
				})

				It("should add volumes to VM spec and return ErrPendingBackfill then ErrPendingRegister", func() {
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(MatchError(unmanagedvolsreg.ErrPendingRegister))

					claimName := vmopv1util.FindByTargetID(
						vmopv1.VirtualControllerTypeSCSI,
						1, 0, vm.Spec.Volumes...).PersistentVolumeClaim.ClaimName

					var pvc corev1.PersistentVolumeClaim
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      claimName,
					}, &pvc)).To(Succeed())

					// Verify PVC properties
					Expect(pvc.Spec.StorageClassName).ToNot(BeNil())
					Expect(*pvc.Spec.StorageClassName).To(Equal("my-storage-class-1"))
					Expect(pvc.Spec.DataSourceRef).ToNot(BeNil())
					Expect(pvc.Spec.DataSourceRef.Kind).To(Equal("VirtualMachine"))
					Expect(pvc.Spec.DataSourceRef.Name).To(Equal(vm.Name))
					Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))
					expectedStorage := *kubeutil.BytesToResource(2 * 1024 * 1024 * 1024)
					Expect(pvc.Spec.Resources.Requests[corev1.ResourceStorage].Equal(expectedStorage)).To(BeTrue())

					// Verify CnsRegisterVolume was created
					crv := &cnsv1alpha1.CnsRegisterVolume{}
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      claimName,
					}, crv)).To(Succeed())

					// Verify CnsRegisterVolume properties
					Expect(crv.Spec.PvcName).To(Equal(claimName))

					Expect(crv.Spec.DiskURLPath).To(Equal(
						fmt.Sprintf(
							"%s/folder/vm1/regular-disk.vmdk?dcPath=%%2FDC0&dsName=LocalDS_0",
							httpPrefix)))
					Expect(crv.Spec.AccessMode).To(Equal(corev1.ReadWriteOnce))
					Expect(crv.Labels["vmoperator.vmware.com/created-by"]).To(Equal(vm.Name))
					Expect(crv.OwnerReferences).To(HaveLen(1))
					Expect(crv.OwnerReferences[0].Kind).To(Equal("VirtualMachine"))
					Expect(crv.OwnerReferences[0].Name).To(Equal(vm.Name))
					Expect(*crv.OwnerReferences[0].Controller).To(BeTrue())
				})
			})

			When("VM has unmanaged disk with MultiWriter sharing", func() {
				JustBeforeEach(func() {
					Expect(unmanagedvolsfill.Reconcile(
						ctx,
						nil,
						nil,
						vm,
						moVM,
						nil)).To(MatchError(unmanagedvolsfill.ErrPendingBackfill))
					Expect(unmanagedvolsfill.Reconcile(
						ctx,
						nil,
						nil,
						vm,
						moVM,
						nil)).To(Succeed())
				})

				BeforeEach(func() {
					// Start with empty volumes - let Reconcile add them
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 200,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/multiwriter-disk.vmdk",
								},
								Uuid:    "disk-uuid-456",
								Sharing: string(vimtypes.VirtualDiskSharingSharingMultiWriter),
							},
						},
						CapacityInBytes: 2 * 1024 * 1024 * 1024,
					}

					scsiController := &vimtypes.VirtualSCSIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 200,
							},
							BusNumber: 1,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								scsiController,
								disk,
							},
						},
					}
				})

				It("should add volumes to VM spec and return ErrPendingVolumes", func() {
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(MatchError(unmanagedvolsreg.ErrPendingRegister))

					claimName := vmopv1util.FindByTargetID(
						vmopv1.VirtualControllerTypeSCSI,
						1, 0, vm.Spec.Volumes...).PersistentVolumeClaim.ClaimName

					var pvc corev1.PersistentVolumeClaim
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      claimName,
					}, &pvc)).To(Succeed())

					// Verify PVC properties
					Expect(pvc.Spec.StorageClassName).ToNot(BeNil())
					Expect(*pvc.Spec.StorageClassName).To(Equal("my-storage-class-1"))
					Expect(pvc.Spec.DataSourceRef).ToNot(BeNil())
					Expect(pvc.Spec.DataSourceRef.Kind).To(Equal("VirtualMachine"))
					Expect(pvc.Spec.DataSourceRef.Name).To(Equal(vm.Name))
					Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteMany))
					expectedStorage := *kubeutil.BytesToResource(2 * 1024 * 1024 * 1024)
					Expect(pvc.Spec.Resources.Requests[corev1.ResourceStorage].Equal(expectedStorage)).To(BeTrue())

					// Verify CnsRegisterVolume was created
					crv := &cnsv1alpha1.CnsRegisterVolume{}
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      claimName,
					}, crv)).To(Succeed())

					// Verify CnsRegisterVolume properties
					Expect(crv.Spec.PvcName).To(Equal(claimName))

					Expect(crv.Spec.DiskURLPath).To(Equal(
						fmt.Sprintf(
							"%s/folder/vm1/multiwriter-disk.vmdk?dcPath=%%2FDC0&dsName=LocalDS_0",
							httpPrefix)))
					Expect(crv.Spec.AccessMode).To(Equal(corev1.ReadWriteMany))
					Expect(crv.Labels["vmoperator.vmware.com/created-by"]).To(Equal(vm.Name))
					Expect(crv.OwnerReferences).To(HaveLen(1))
					Expect(crv.OwnerReferences[0].Kind).To(Equal("VirtualMachine"))
					Expect(crv.OwnerReferences[0].Name).To(Equal(vm.Name))
					Expect(*crv.OwnerReferences[0].Controller).To(BeTrue())
				})
			})

			When("PVC exists and is bound", func() {
				BeforeEach(func() {
					// Set up VM with volumes already in spec
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-789",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-134e95b6",
									},
								},
							},
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					// Create bound PVC
					boundPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-134e95b6",
							Namespace: vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							DataSourceRef: &corev1.TypedObjectReference{
								APIGroup: &vmopv1.GroupVersion.Group,
								Kind:     "VirtualMachine",
								Name:     vm.Name,
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					}

					// Create existing CnsRegisterVolume to be cleaned up
					existingCRV := &cnsv1alpha1.CnsRegisterVolume{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-134e95b6",
							Namespace: vm.Namespace,
							Labels: map[string]string{
								"vmoperator.vmware.com/created-by": vm.Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: cnsv1alpha1.CnsRegisterVolumeSpec{
							PvcName:     "my-vm-134e95b6",
							DiskURLPath: "[LocalDS_0] vm1/disk.vmdk",
							AccessMode:  corev1.ReadWriteOnce,
						},
					}

					withObjs = append(withObjs, boundPVC, existingCRV)

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-789",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}
				})

				It("should clean up CnsRegisterVolume and return nil", func() {
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(Succeed())

					claimName := vmopv1util.FindByTargetID(
						vmopv1.VirtualControllerTypeIDE,
						0, 0, vm.Spec.Volumes...).PersistentVolumeClaim.ClaimName

					// Verify CnsRegisterVolume was deleted
					Expect(apierrors.IsNotFound(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      claimName,
					}, &cnsv1alpha1.CnsRegisterVolume{}))).To(BeTrue())
				})
			})

			When("PVC exists but needs patching", func() {
				BeforeEach(func() {
					// Set up VM with volumes already in spec
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-patch",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-f3069b5c",
									},
								},
							},
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					// Create PVC without OwnerRef and DataSourceRef
					existingPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-f3069b5c",
							Namespace: vm.Namespace,
							// No OwnerReferences
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							// No DataSourceRef
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(10 * 1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimPending,
						},
					}

					withObjs = append(withObjs, existingPVC)

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-patch",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}
				})

				It("should patch PVC with OwnerRef and DataSourceRef", func() {
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(Succeed())
					Expect(configSpec.DeviceChange).To(HaveLen(1))

					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(MatchError(unmanagedvolsreg.ErrPendingRegister))

					// Verify PVC was patched
					pvc := &corev1.PersistentVolumeClaim{}
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-f3069b5c",
					}, pvc)).To(Succeed())

					// Verify OwnerRef was added
					Expect(pvc.OwnerReferences).To(HaveLen(1))
					Expect(pvc.OwnerReferences[0].Kind).To(Equal("VirtualMachine"))
					Expect(pvc.OwnerReferences[0].Name).To(Equal(vm.Name))

					// Verify DataSourceRef was added
					Expect(pvc.Spec.DataSourceRef).ToNot(BeNil())
					Expect(pvc.Spec.DataSourceRef.Kind).To(Equal("VirtualMachine"))
					Expect(pvc.Spec.DataSourceRef.Name).To(Equal(vm.Name))

					// Verify CnsRegisterVolume was created
					crv := &cnsv1alpha1.CnsRegisterVolume{}
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-f3069b5c",
					}, crv)).To(Succeed())
				})
			})

			When("PVC exists with DataSourceRef but nil APIGroup", func() {
				BeforeEach(func() {
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-nil-apigroup",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-09e1c91f",
									},
								},
							},
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					// Create PVC with DataSourceRef but nil APIGroup
					existingPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-09e1c91f",
							Namespace: vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
								},
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							DataSourceRef: &corev1.TypedObjectReference{
								APIGroup: nil, // nil APIGroup
								Kind:     "VirtualMachine",
								Name:     vm.Name,
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimPending,
						},
					}

					withObjs = append(withObjs, existingPVC)

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-nil-apigroup",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}
				})

				It("should patch PVC with correct APIGroup", func() {
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(MatchError(unmanagedvolsreg.ErrPendingRegister))

					// Verify PVC was patched
					pvc := &corev1.PersistentVolumeClaim{}
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-09e1c91f",
					}, pvc)).To(Succeed())

					// Verify DataSourceRef was updated with correct APIGroup
					Expect(pvc.Spec.DataSourceRef).ToNot(BeNil())
					Expect(pvc.Spec.DataSourceRef.APIGroup).ToNot(BeNil())
					Expect(*pvc.Spec.DataSourceRef.APIGroup).To(Equal(vmopv1.GroupVersion.Group))
					Expect(pvc.Spec.DataSourceRef.Kind).To(Equal("VirtualMachine"))
					Expect(pvc.Spec.DataSourceRef.Name).To(Equal(vm.Name))
				})
			})

			When("PVC exists with wrong DataSourceRef APIGroup", func() {
				BeforeEach(func() {
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-wrong-apigroup",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-7e8b2d45",
									},
								},
							},
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					wrongAPIGroup := "wrong.api.group"
					// Create PVC with wrong APIGroup
					existingPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-7e8b2d45",
							Namespace: vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
								},
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							DataSourceRef: &corev1.TypedObjectReference{
								APIGroup: &wrongAPIGroup,
								Kind:     "VirtualMachine",
								Name:     vm.Name,
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimPending,
						},
					}

					withObjs = append(withObjs, existingPVC)

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-wrong-apigroup",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}
				})

				It("should patch PVC with correct APIGroup", func() {
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(MatchError(unmanagedvolsreg.ErrPendingRegister))

					// Verify PVC was patched
					pvc := &corev1.PersistentVolumeClaim{}
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-7e8b2d45",
					}, pvc)).To(Succeed())

					// Verify DataSourceRef was updated with correct APIGroup
					Expect(pvc.Spec.DataSourceRef).ToNot(BeNil())
					Expect(pvc.Spec.DataSourceRef.APIGroup).ToNot(BeNil())
					Expect(*pvc.Spec.DataSourceRef.APIGroup).To(Equal(vmopv1.GroupVersion.Group))
					Expect(pvc.Spec.DataSourceRef.Kind).To(Equal("VirtualMachine"))
					Expect(pvc.Spec.DataSourceRef.Name).To(Equal(vm.Name))
				})
			})

			When("PVC is pending and CnsRegisterVolume exists", func() {
				BeforeEach(func() {
					// Set up VM with volumes already in spec
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-pending",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-0c2d85ea",
									},
								},
							},
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					// Create pending PVC
					pendingPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-0c2d85ea",
							Namespace: vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							DataSourceRef: &corev1.TypedObjectReference{
								APIGroup: &vmopv1.GroupVersion.Group,
								Kind:     "VirtualMachine",
								Name:     vm.Name,
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimPending,
						},
					}

					withObjs = append(withObjs, pendingPVC)

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-pending",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}
				})

				It("should return ErrPendingVolumes for pending PVC", func() {
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(MatchError(unmanagedvolsreg.ErrPendingRegister))
				})
			})

			When("multiple reconciliation cycles with state changes", func() {
				BeforeEach(func() {
					// Set up VM with volumes already in spec
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name:                "disk-uuid-cycle",
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Type:     vmopv1.VolumeTypeClassic,
							DiskUUID: "disk-uuid-cycle",
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-cycle",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}
				})

				It("should handle complete reconciliation cycle", func() {
					// First reconciliation: should create PVC and CnsRegisterVolume.
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(MatchError(unmanagedvolsreg.ErrPendingRegister))

					claimName := vmopv1util.FindByTargetID(
						vmopv1.VirtualControllerTypeIDE,
						0, 0, vm.Spec.Volumes...).PersistentVolumeClaim.ClaimName

					// Verify PVC was created.
					pvc := &corev1.PersistentVolumeClaim{}
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      claimName,
					}, pvc)).To(Succeed())

					// Set the PVC status to Pending for the test
					pvc.Status.Phase = corev1.ClaimPending
					Expect(k8sClient.Status().Update(ctx, pvc)).To(Succeed())

					// Verify CnsRegisterVolume was created
					crv := &cnsv1alpha1.CnsRegisterVolume{}
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      claimName,
					}, crv)).To(Succeed())

					// Simulate PVC becoming bound
					pvc.Status.Phase = corev1.ClaimBound
					Expect(k8sClient.Status().Update(ctx, pvc)).To(Succeed())

					// Verify the status was not yet pruned.
					Expect(vm.Status.Volumes).To(HaveLen(1))
					Expect(vm.Status.Volumes[0].Type).To(Equal(vmopv1.VolumeTypeClassic))

					vm.Status.Volumes = append(vm.Status.Volumes,
						vmopv1.VirtualMachineVolumeStatus{
							Type:     vmopv1.VolumeTypeManaged,
							DiskUUID: "disk-uuid-cycle",
						})

					// Second reconciliation: should clean up CnsRegisterVolume
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(Succeed())

					// Verify CnsRegisterVolume was deleted
					Expect(apierrors.IsNotFound(k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      claimName,
					}, &cnsv1alpha1.CnsRegisterVolume{}))).To(BeTrue())

					// Verify the status was pruned.
					Expect(vm.Status.Volumes).To(HaveLen(1))
					Expect(vm.Status.Volumes[0].Type).To(Equal(vmopv1.VolumeTypeManaged))

					// Third reconciliation: should not modify status.
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(Succeed())

					// Verify the status was unchanged.
					Expect(vm.Status.Volumes).To(HaveLen(1))
					Expect(vm.Status.Volumes[0].Type).To(Equal(vmopv1.VolumeTypeManaged))
				})
			})
		})

		Context("error handling", func() {
			When("k8sClient.Get fails for PVC", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-87ce75a6",
									},
								},
							},
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-error",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}

					// Set up interceptor to return error on Get
					withFuncs = interceptor.Funcs{
						Get: func(ctx context.Context, client ctrlclient.WithWatch, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
							if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
								return fmt.Errorf("simulated get error")
							}
							return client.Get(ctx, key, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := unmanagedvolsreg.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to get pvc"))
				})
			})

			When("k8sClient.Create fails for PVC", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name:                "disk-uuid-create-error",
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-create-error",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}

					// Set up interceptor to return error on Create
					withFuncs = interceptor.Funcs{
						Create: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
							if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
								return fmt.Errorf("simulated create error")
							}
							return client.Create(ctx, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := unmanagedvolsreg.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to create pvc"))
				})
			})

			When("k8sClient.Create fails for CnsRegisterVolume", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name:                "disk-uuid-crv-error",
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-crv-error",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}

					// Set up interceptor to return error on Create for CnsRegisterVolume
					withFuncs = interceptor.Funcs{
						Create: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
							if _, ok := obj.(*cnsv1alpha1.CnsRegisterVolume); ok {
								return fmt.Errorf("simulated crv create error")
							}
							return client.Create(ctx, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := unmanagedvolsreg.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to ensure CnsRegisterVolume"))
				})
			})

			When("k8sClient.Get fails for CnsRegisterVolume", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name:                "disk-uuid-crv-get-error",
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-crv-get-error",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}

					// Set up interceptor to return error on Get for CnsRegisterVolume
					withFuncs = interceptor.Funcs{
						Get: func(ctx context.Context, client ctrlclient.WithWatch, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
							if _, ok := obj.(*cnsv1alpha1.CnsRegisterVolume); ok {
								return fmt.Errorf("simulated crv get error")
							}
							return client.Get(ctx, key, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := unmanagedvolsreg.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to get CnsRegisterVolume"))
				})
			})

			When("k8sClient.Patch fails for PVC", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-patch-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-cd73132d",
									},
								},
							},
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					// Create existing PVC without OwnerRef and DataSourceRef
					existingPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-cd73132d",
							Namespace: vm.Namespace,
							// No OwnerReferences
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							// No DataSourceRef
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimPending,
						},
					}

					withObjs = append(withObjs, existingPVC)

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-patch-error",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}

					// Set up interceptor to return error on Patch
					withFuncs = interceptor.Funcs{
						Patch: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
							if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
								return fmt.Errorf("simulated patch error")
							}
							return client.Patch(ctx, obj, patch, opts...)
						},
					}
				})

				It("should return error", func() {
					err := unmanagedvolsreg.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to patch pvc"))
				})
			})

			When("k8sClient.Delete fails for CnsRegisterVolume cleanup", func() {
				BeforeEach(func() {
					// Set up VM with bound PVC
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-delete-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-9eb41331",
									},
								},
							},
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					// Create bound PVC
					boundPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-9eb41331",
							Namespace: vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							DataSourceRef: &corev1.TypedObjectReference{
								APIGroup: &vmopv1.GroupVersion.Group,
								Kind:     "VirtualMachine",
								Name:     vm.Name,
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					}

					// Create existing CnsRegisterVolume to be cleaned up
					existingCRV := &cnsv1alpha1.CnsRegisterVolume{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-9eb41331",
							Namespace: vm.Namespace,
							Labels: map[string]string{
								"vmoperator.vmware.com/created-by": vm.Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: cnsv1alpha1.CnsRegisterVolumeSpec{
							PvcName:     "my-vm-9eb41331",
							DiskURLPath: "[LocalDS_0] vm1/disk.vmdk",
							AccessMode:  corev1.ReadWriteOnce,
						},
					}

					withObjs = append(withObjs, boundPVC, existingCRV)

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-delete-error",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}

					// Set up interceptor to return error on Delete
					withFuncs = interceptor.Funcs{
						Delete: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.DeleteOption) error {
							if _, ok := obj.(*cnsv1alpha1.CnsRegisterVolume); ok {
								return fmt.Errorf("simulated delete error")
							}
							return client.Delete(ctx, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := unmanagedvolsreg.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to delete CnsRegisterVolume"))
				})
			})

			When("k8sClient.DeleteAllOf fails for CnsRegisterVolume cleanup", func() {
				BeforeEach(func() {
					// Set up VM with bound PVC
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-delete-all-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-1aeeec80",
									},
								},
							},
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					// Create bound PVC
					boundPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-1aeeec80",
							Namespace: vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							DataSourceRef: &corev1.TypedObjectReference{
								APIGroup: &vmopv1.GroupVersion.Group,
								Kind:     "VirtualMachine",
								Name:     vm.Name,
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					}

					withObjs = append(withObjs, boundPVC)

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-delete-all-error",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}

					// Set up interceptor to return error on DeleteAllOf
					withFuncs = interceptor.Funcs{
						DeleteAllOf: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.DeleteAllOfOption) error {
							if _, ok := obj.(*cnsv1alpha1.CnsRegisterVolume); ok {
								return fmt.Errorf("simulated delete all error")
							}
							return client.DeleteAllOf(ctx, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := unmanagedvolsreg.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to delete CnsRegisterVolume"))
				})
			})

			When("k8sClient.Status().Patch fails for PVC status update", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name:                "disk-uuid-status-update-error",
							ControllerType:      vmopv1.VirtualControllerTypeIDE,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(0)),
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-status-update-error",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}

					// Set up interceptor to return error on Status().Update
					withFuncs = interceptor.Funcs{
						SubResourcePatch: func(ctx context.Context, client ctrlclient.Client, subResourceName string, obj ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.SubResourcePatchOption) error {
							if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
								return fmt.Errorf("simulated status update error")
							}
							return client.Status().Patch(ctx, obj, patch, opts...)
						},
					}
				})

				It("should handle status update error gracefully", func() {
					// This test verifies that status update errors do not break
					// the reconciliation.
					// The Reconcile function does not directly call
					// Status().Update, and this test ensures the interceptor is
					// working correctly.
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(MatchError(unmanagedvolsreg.ErrPendingRegister))
				})
			})
		})

		When("VM has condition already set to true", func() {
			BeforeEach(func() {
				// Set condition to true
				vm.Status.Conditions = []metav1.Condition{
					{
						Type:   "VirtualMachineUnmanagedVolumesRegister",
						Status: metav1.ConditionTrue,
					},
				}

				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           300,
						ControllerKey: 100,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[LocalDS_0] vm1/disk.vmdk",
							},
							Uuid: "disk-uuid-skip",
						},
					},
					CapacityInBytes: 1024 * 1024 * 1024,
				}

				ideController := &vimtypes.VirtualIDEController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
						BusNumber: 0,
					},
				}

				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							ideController,
							disk,
						},
					},
				}
			})

			JustBeforeEach(func() {
				Expect(unmanagedvolsfill.Reconcile(
					ctx,
					nil,
					nil,
					vm,
					moVM,
					nil)).To(MatchError(unmanagedvolsfill.ErrPendingBackfill))
				Expect(unmanagedvolsfill.Reconcile(
					ctx,
					nil,
					nil,
					vm,
					moVM,
					nil)).To(Succeed())
			})

			It("should not return early", func() {
				Expect(unmanagedvolsreg.Reconcile(
					ctx,
					k8sClient,
					vimClient,
					vm,
					moVM,
					configSpec)).To(MatchError(unmanagedvolsreg.ErrPendingRegister))

				Expect(pkgcond.IsFalse(vm, unmanagedvolsreg.Condition)).To(BeTrue())
			})
		})

		When("a disk is missing a storage policy", func() {

			var disk *vimtypes.VirtualDisk

			BeforeEach(func() {
				// Clear mockProfileResults to ensure disk has no policy
				mockProfileResults = []pbmtypes.PbmQueryProfileResult{}

				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name:                "disk-no-policy",
						ControllerType:      vmopv1.VirtualControllerTypeIDE,
						ControllerBusNumber: ptr.To(int32(0)),
						UnitNumber:          ptr.To(int32(0)),
					},
				}

				disk = &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           300,
						ControllerKey: 100,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[LocalDS_0] vm1/disk.vmdk",
							},
							Uuid: "disk-uuid-no-policy",
						},
					},
					CapacityInBytes: 1024 * 1024 * 1024,
				}

				ideController := &vimtypes.VirtualIDEController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
						BusNumber: 0,
					},
				}

				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							ideController,
							disk,
						},
					},
				}
			})

			When("there is not an entry for the disk in the configSpec", func() {
				It("should add storage policy to configSpec and return nil", func() {
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(Succeed())

					// Verify configSpec has device change
					Expect(configSpec.DeviceChange).To(HaveLen(1))
					dc := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
					Expect(dc.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationEdit))
					Expect(dc.Profile).To(HaveLen(1))
					dp := dc.Profile[0].(*vimtypes.VirtualMachineDefinedProfileSpec)
					Expect(dp.ProfileId).To(Equal("profile-123"))
				})
			})

			When("there is already an entry for the disk in the configSpec", func() {
				BeforeEach(func() {
					// Pre-add a device change for the same disk
					configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
						&vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
							Device:    disk,
							Profile:   []vimtypes.BaseVirtualMachineProfileSpec{},
						},
					}
				})

				It("should update existing device change with storage policy", func() {
					Expect(unmanagedvolsreg.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(Succeed())

					// Verify configSpec still has one device change but with profile added
					Expect(configSpec.DeviceChange).To(HaveLen(1))
					dc := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
					Expect(dc.Profile).To(HaveLen(1))
					dp := dc.Profile[0].(*vimtypes.VirtualMachineDefinedProfileSpec)
					Expect(dp.ProfileId).To(Equal("profile-123"))
				})
			})
		})

		When("pbm client creation fails", func() {
			JustBeforeEach(func() {
				vcSimCtx.SimulatorContext().For("/pbm").Map.Handler = func(
					ctx *simulator.Context,
					m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

					if m.Name == "PbmRetrieveServiceContent" {
						return nil, &vimtypes.RuntimeFault{
							MethodFault: vimtypes.MethodFault{
								FaultCause: &vimtypes.LocalizedMethodFault{
									Fault:            &vimtypes.SystemError{},
									LocalizedMessage: "PBM query failed",
								},
							},
						}
					}

					return nil, nil
				}
			})

			BeforeEach(func() {
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "disk-pbm-fail",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-vm-pbm-fail",
								},
							},
						},
						ControllerType:      vmopv1.VirtualControllerTypeIDE,
						ControllerBusNumber: ptr.To(int32(0)),
						UnitNumber:          ptr.To(int32(0)),
					},
				}

				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           300,
						ControllerKey: 100,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[LocalDS_0] vm1/disk.vmdk",
							},
							Uuid: "disk-uuid-pbm-fail",
						},
					},
					CapacityInBytes: 1024 * 1024 * 1024,
				}

				ideController := &vimtypes.VirtualIDEController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
						BusNumber: 0,
					},
				}

				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							ideController,
							disk,
						},
					},
				}
			})

			It("should return error", func() {
				err := unmanagedvolsreg.Reconcile(
					ctx,
					k8sClient,
					vimClient,
					vm,
					moVM,
					configSpec)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get pbm client"))
			})
		})

		When("pbm QueryAssociatedProfiles fails", func() {
			JustBeforeEach(func() {
				Expect(unmanagedvolsfill.Reconcile(
					ctx,
					nil,
					nil,
					vm,
					moVM,
					nil)).To(MatchError(unmanagedvolsfill.ErrPendingBackfill))
				Expect(unmanagedvolsfill.Reconcile(
					ctx,
					nil,
					nil,
					vm,
					moVM,
					nil)).To(Succeed())

				// Configure PBM mock to return error
				vcSimCtx.SimulatorContext().For("/pbm").Map.Handler = func(
					ctx *simulator.Context,
					m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

					if m.Name == "PbmQueryAssociatedProfiles" {
						return nil, &vimtypes.RuntimeFault{
							MethodFault: vimtypes.MethodFault{
								FaultCause: &vimtypes.LocalizedMethodFault{
									Fault:            &vimtypes.SystemError{},
									LocalizedMessage: "PBM query failed",
								},
							},
						}
					}

					return nil, nil
				}
			})

			BeforeEach(func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           300,
						ControllerKey: 100,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[LocalDS_0] vm1/disk.vmdk",
							},
							Uuid: "disk-uuid-pbm-query-fail",
						},
					},
					CapacityInBytes: 1024 * 1024 * 1024,
				}

				ideController := &vimtypes.VirtualIDEController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
						BusNumber: 0,
					},
				}

				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							ideController,
							disk,
						},
					},
				}
			})

			It("should return error", func() {
				err := unmanagedvolsreg.Reconcile(
					ctx,
					k8sClient,
					vimClient,
					vm,
					moVM,
					configSpec)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to query associated profiles"))
			})
		})

		When("k8sClient.List fails for StorageClasses", func() {
			JustBeforeEach(func() {
				Expect(unmanagedvolsfill.Reconcile(
					ctx,
					nil,
					nil,
					vm,
					moVM,
					nil)).To(MatchError(unmanagedvolsfill.ErrPendingBackfill))
				Expect(unmanagedvolsfill.Reconcile(
					ctx,
					nil,
					nil,
					vm,
					moVM,
					nil)).To(Succeed())
			})

			BeforeEach(func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           300,
						ControllerKey: 100,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[LocalDS_0] vm1/disk.vmdk",
							},
							Uuid: "disk-uuid-list-fail",
						},
					},
					CapacityInBytes: 1024 * 1024 * 1024,
				}

				ideController := &vimtypes.VirtualIDEController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
						BusNumber: 0,
					},
				}

				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							ideController,
							disk,
						},
					},
				}

				withFuncs = interceptor.Funcs{
					List: func(ctx context.Context, client ctrlclient.WithWatch, list ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
						if _, ok := list.(*storagev1.StorageClassList); ok {
							return fmt.Errorf("simulated list error")
						}
						return client.List(ctx, list, opts...)
					},
				}
			})

			It("should return error", func() {
				err := unmanagedvolsreg.Reconcile(
					ctx,
					k8sClient,
					vimClient,
					vm,
					moVM,
					configSpec)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to list storage classes"))
			})
		})

		When("GetDatastoreURLFromDatastorePath fails", func() {
			JustBeforeEach(func() {
				Expect(unmanagedvolsfill.Reconcile(
					ctx,
					nil,
					nil,
					vm,
					moVM,
					nil)).To(MatchError(unmanagedvolsfill.ErrPendingBackfill))
				Expect(unmanagedvolsfill.Reconcile(
					ctx,
					nil,
					nil,
					vm,
					moVM,
					nil)).To(Succeed())
			})

			BeforeEach(func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           300,
						ControllerKey: 100,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								// Invalid datastore path
								FileName: "invalid-ds-path",
							},
							Uuid: "disk-uuid-invalid-path",
						},
					},
					CapacityInBytes: 1024 * 1024 * 1024,
				}

				ideController := &vimtypes.VirtualIDEController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
						BusNumber: 0,
					},
				}

				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							ideController,
							disk,
						},
					},
				}
			})

			It("should return error", func() {
				err := unmanagedvolsreg.Reconcile(
					ctx,
					k8sClient,
					vimClient,
					vm,
					moVM,
					configSpec)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get datastore url"))
			})
		})

		When("PVC creation fails", func() {
			JustBeforeEach(func() {
				Expect(unmanagedvolsfill.Reconcile(
					ctx,
					nil,
					nil,
					vm,
					moVM,
					nil)).To(MatchError(unmanagedvolsfill.ErrPendingBackfill))
				Expect(unmanagedvolsfill.Reconcile(
					ctx,
					nil,
					nil,
					vm,
					moVM,
					nil)).To(Succeed())
			})
			BeforeEach(func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           300,
						ControllerKey: 100,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[LocalDS_0] vm1/disk.vmdk",
							},
							Uuid: "disk-uuid-create-fail",
						},
					},
					CapacityInBytes: 1024 * 1024 * 1024,
				}

				ideController := &vimtypes.VirtualIDEController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
						BusNumber: 0,
					},
				}

				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							ideController,
							disk,
						},
					},
				}

				withFuncs = interceptor.Funcs{
					Create: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
						if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
							return fmt.Errorf("simulated create error")
						}
						return client.Create(ctx, obj, opts...)
					},
				}
			})

			It("should return error from PVC creation", func() {
				err := unmanagedvolsreg.Reconcile(
					ctx,
					k8sClient,
					vimClient,
					vm,
					moVM,
					configSpec)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create pvc"))
				Expect(err.Error()).To(ContainSubstring("simulated create error"))
			})
		})

		When("PVC patch fails", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "disk-uuid-patch-fail",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-vm-patch-fail",
								},
							},
						},
						ControllerType:      vmopv1.VirtualControllerTypeIDE,
						ControllerBusNumber: ptr.To(int32(0)),
						UnitNumber:          ptr.To(int32(0)),
					},
				}

				// Create PVC without proper OwnerRef/DataSourceRef
				existingPVC := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-vm-patch-fail",
						Namespace: vm.Namespace,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
							},
						},
					},
				}

				withObjs = append(withObjs, existingPVC)

				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           300,
						ControllerKey: 100,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[LocalDS_0] vm1/disk.vmdk",
							},
							Uuid: "disk-uuid-patch-fail",
						},
					},
					CapacityInBytes: 1024 * 1024 * 1024,
				}

				ideController := &vimtypes.VirtualIDEController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
						BusNumber: 0,
					},
				}

				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							ideController,
							disk,
						},
					},
				}

				withFuncs = interceptor.Funcs{
					Patch: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
						if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
							return fmt.Errorf("simulated patch error")
						}
						return client.Patch(ctx, obj, patch, opts...)
					},
				}
			})

			It("should return error from PVC patch", func() {
				err := unmanagedvolsreg.Reconcile(
					ctx,
					k8sClient,
					vimClient,
					vm,
					moVM,
					configSpec)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to patch pvc"))
				Expect(err.Error()).To(ContainSubstring("simulated patch error"))
			})
		})

		When("ensurePVCForUnmanagedDisk encounters general error", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name:                "disk-0",
						ControllerType:      vmopv1.VirtualControllerTypeIDE,
						ControllerBusNumber: ptr.To(int32(0)),
						UnitNumber:          ptr.To(int32(0)),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "disk0",
								},
							},
						},
					},
				}

				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           300,
						ControllerKey: 100,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[LocalDS_0] vm1/disk.vmdk",
							},
							Uuid: "disk-uuid-general-get-error",
						},
					},
					CapacityInBytes: 1024 * 1024 * 1024,
				}

				ideController := &vimtypes.VirtualIDEController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
						BusNumber: 0,
					},
				}

				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							ideController,
							disk,
						},
					},
				}

				withFuncs = interceptor.Funcs{
					Get: func(ctx context.Context, client ctrlclient.WithWatch, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
						if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
							return fmt.Errorf("simulated general error")
						}
						return client.Get(ctx, key, obj, opts...)
					},
				}
			})

			It("should return error from ensurePVCForUnmanagedDisk", func() {
				err := unmanagedvolsreg.Reconcile(
					ctx,
					k8sClient,
					vimClient,
					vm,
					moVM,
					configSpec)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get pvc"))
				Expect(err.Error()).To(ContainSubstring("simulated general error"))
			})
		})
	})
})

type fakeProfileManager struct {
	*pbmsim.ProfileManager
	Result []pbmtypes.PbmQueryProfileResult
}

func (m *fakeProfileManager) PbmQueryAssociatedProfiles(
	req *pbmtypes.PbmQueryAssociatedProfiles) soap.HasFault {

	body := new(pbmmethods.PbmQueryAssociatedProfilesBody)
	body.Res = new(pbmtypes.PbmQueryAssociatedProfilesResponse)
	body.Res.Returnval = m.Result

	return body
}
