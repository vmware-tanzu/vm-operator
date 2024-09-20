// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyusage_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/storagepolicyusage"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	const (
		fake             = "fake"
		namespace        = "default"
		name             = "my-storage-class"
		storageQuotaName = "my-storage-quota"
		storagePolicyID  = "my-storage-policy"
	)

	var (
		reconciler                *storagepolicyusage.Reconciler
		ctx                       *builder.UnitTestContextForController
		inNamespace               string
		inName                    string
		withFuncs                 interceptor.Funcs
		withObjects               []ctrlclient.Object
		skipGetStoragePolicyUsage bool
		err                       error
		zeroQuantity              resource.Quantity
		vm1                       *vmopv1.VirtualMachine
		vm2                       *vmopv1.VirtualMachine
	)

	BeforeEach(func() {
		withObjects = nil
		withFuncs = interceptor.Funcs{}
		inNamespace = namespace
		inName = name
		skipGetStoragePolicyUsage = false
		zeroQuantity = resource.MustParse("0Gi")
	})

	JustBeforeEach(func() {
		if vm1 != nil {
			withObjects = append(withObjects, vm1)
		}
		if vm2 != nil {
			withObjects = append(withObjects, vm2)
		}

		ctx = suite.NewUnitTestContextForControllerWithFuncs(
			withFuncs, withObjects...)

		reconciler = storagepolicyusage.NewReconciler(
			pkgcfg.UpdateContext(
				ctx,
				func(config *pkgcfg.Config) {
					config.Features.PodVMOnStretchedSupervisor = true
				},
			),
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
		)

		_, err = reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: inNamespace,
				Name:      inName,
			}})
	})

	assertReportedTotalsWithOffset := func(
		spu spqv1.StoragePolicyUsage,
		actErr, expErr error,
		expReserved, expUsed resource.Quantity,
		offset int) {

		if expErr != nil {
			ExpectWithOffset(offset, actErr).To(HaveOccurred(), "err should have occurred")
			ExpectWithOffset(offset, actErr).To(MatchError(expErr.Error()), "errors do not match")
			return
		}

		ExpectWithOffset(offset, actErr).ToNot(HaveOccurred(), "err should not have occurred")
		ExpectWithOffset(offset, spu.Status.ResourceTypeLevelQuotaUsage).ToNot(BeNil(), "spu status should be non-nil")
		ExpectWithOffset(offset, spu.Status.ResourceTypeLevelQuotaUsage.Reserved).ToNot(BeNil(), "reserved should be non-nil")
		ExpectWithOffset(offset, spu.Status.ResourceTypeLevelQuotaUsage.Reserved.Value()).To(Equal(expReserved.Value()), fmt.Sprintf("reserved should be %s", &expReserved))
		ExpectWithOffset(offset, spu.Status.ResourceTypeLevelQuotaUsage.Used).ToNot(BeNil(), "used should be non-nil")
		ExpectWithOffset(offset, spu.Status.ResourceTypeLevelQuotaUsage.Used.Value()).To(Equal(expUsed.Value()), fmt.Sprintf("used should be %s", &expUsed))
	}

	assertReportedErr := func(
		spu spqv1.StoragePolicyUsage,
		actErr, expErr error) {

		assertReportedTotalsWithOffset(
			spu,
			actErr,
			expErr,
			zeroQuantity,
			zeroQuantity,
			2)
	}

	assertReportedTotals := func(
		spu spqv1.StoragePolicyUsage,
		actErr, expErr error,
		expReserved, expUsed resource.Quantity) {

		assertReportedTotalsWithOffset(
			spu,
			actErr,
			expErr,
			expReserved,
			expUsed,
			2)
	}

	assertZeroReportedTotals := func(
		spu spqv1.StoragePolicyUsage,
		actErr, expErr error) {

		assertReportedTotalsWithOffset(
			spu,
			actErr,
			expErr,
			zeroQuantity,
			zeroQuantity,
			2)
	}

	errFailedToGetImg := func(vm vmopv1.VirtualMachine) error {
		namespace := ""
		if vm.Spec.Image.Kind == "VirtualMachineImage" {
			namespace = vm.Namespace
		}
		return fmt.Errorf(
			`failed to report reserved capacity for "%s/%s": failed to get %s %s/%s: %s`,
			vm.Namespace,
			vm.Name,
			vm.Spec.Image.Kind,
			namespace,
			vm.Spec.Image.Name,
			fake)
	}

	When("there is no StoragePolicyUsage resource", func() {
		It("should no-op", func() {
			Expect(err).To(MatchError(
				fmt.Sprintf(
					"failed to get StoragePolicyUsage %[1]s/%[2]s-vm-usage: "+
						"storagepolicyusages.cns.vmware.com \"%[2]s-vm-usage\" not found",
					inNamespace, inName)))
		})
	})

	When("there is a StoragePolicyUsage resource", func() {
		var (
			spu spqv1.StoragePolicyUsage
		)
		BeforeEach(func() {
			withObjects = append(
				withObjects,
				&spqv1.StoragePolicyUsage{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      spqutil.StoragePolicyUsageName(name),
					},
					Spec: spqv1.StoragePolicyUsageSpec{
						StoragePolicyId:  fake,
						StorageClassName: name,
					},
				})
		})
		AfterEach(func() {
			spu = spqv1.StoragePolicyUsage{}
		})
		JustBeforeEach(func() {
			if !skipGetStoragePolicyUsage {
				Expect(ctx.Client.Get(
					ctx,
					ctrlclient.ObjectKey{
						Namespace: namespace,
						Name:      spqutil.StoragePolicyUsageName(name),
					},
					&spu,
				)).To(Succeed())
			}
		})
		When("there is an error getting the StoragePolicyUsage resource the first time", func() {
			BeforeEach(func() {
				skipGetStoragePolicyUsage = true
				numCalls := 0
				withFuncs.Get = func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					key ctrlclient.ObjectKey,
					obj ctrlclient.Object,
					opts ...ctrlclient.GetOption) error {

					if _, ok := obj.(*spqv1.StoragePolicyUsage); ok {
						if numCalls == 0 {
							return fmt.Errorf(fake)
						}
						numCalls++
					}

					return client.Get(ctx, key, obj, opts...)
				}
			})
			It("should return an error", func() {
				Expect(err).To(MatchError(
					fmt.Sprintf(
						"failed to get StoragePolicyUsage %s/%s-vm-usage: %s",
						inNamespace, inName, fake)))
			})
		})
		When("there is an error listing VM resources", func() {
			BeforeEach(func() {
				withFuncs.List = func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					list ctrlclient.ObjectList,
					opts ...ctrlclient.ListOption) error {

					if _, ok := list.(*vmopv1.VirtualMachineList); ok {
						return fmt.Errorf(fake)
					}

					return client.List(ctx, list, opts...)
				}
			})
			It("should return an error", func() {
				Expect(err).To(MatchError(
					fmt.Sprintf(
						"failed to list VMs in namespace %s: %s",
						inNamespace, fake)))
			})
		})
		When("there are no VM resources", func() {
			Specify("the usage should remain empty", func() {
				assertZeroReportedTotals(spu, err, nil)
			})
		})
		When("there are VM resources", func() {
			BeforeEach(func() {
				vm1 = &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "vm-1",
					},
					Spec: vmopv1.VirtualMachineSpec{
						Image: &vmopv1.VirtualMachineImageRef{
							Kind: "VirtualMachineImage",
							Name: "my-vmi",
						},
					},
					Status: vmopv1.VirtualMachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:   vmopv1.VirtualMachineConditionCreated,
								Status: metav1.ConditionTrue,
							},
						},
						Storage: &vmopv1.VirtualMachineStorageStatus{
							Committed:   ptr.To(resource.MustParse("10Gi")),
							Uncommitted: ptr.To(resource.MustParse("20Gi")),
							Unshared:    ptr.To(resource.MustParse("5Gi")),
						},
						Volumes: []vmopv1.VirtualMachineVolumeStatus{
							{
								Name:  "vm-1",
								Type:  vmopv1.VirtualMachineStorageDiskTypeClassic,
								Limit: ptr.To(resource.MustParse("20Gi")),
								Used:  ptr.To(resource.MustParse("10Gi")),
							},
							{
								Name:  "vol-1",
								Type:  vmopv1.VirtualMachineStorageDiskTypeManaged,
								Limit: ptr.To(resource.MustParse("10Gi")),
								Used:  ptr.To(resource.MustParse("2Gi")),
							},
						},
					},
				}

				vm2 = &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "vm-2",
					},
					Spec: vmopv1.VirtualMachineSpec{
						Image: &vmopv1.VirtualMachineImageRef{
							Kind: "VirtualMachineImage",
							Name: "my-vmi",
						},
					},
					Status: vmopv1.VirtualMachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:   vmopv1.VirtualMachineConditionCreated,
								Status: metav1.ConditionTrue,
							},
						},
						Storage: &vmopv1.VirtualMachineStorageStatus{
							Committed:   ptr.To(resource.MustParse("5Gi")),
							Uncommitted: ptr.To(resource.MustParse("10Gi")),
							Unshared:    ptr.To(resource.MustParse("2Gi")),
						},
						Volumes: []vmopv1.VirtualMachineVolumeStatus{
							{
								Name:  "vm-2",
								Type:  vmopv1.VirtualMachineStorageDiskTypeClassic,
								Limit: ptr.To(resource.MustParse("10Gi")),
								Used:  ptr.To(resource.MustParse("1Gi")),
							},
							{
								Name:  "vol-2",
								Type:  vmopv1.VirtualMachineStorageDiskTypeManaged,
								Limit: ptr.To(resource.MustParse("10Gi")),
								Used:  ptr.To(resource.MustParse("0.5Gi")),
							},
						},
					},
				}
			})

			When("there is an error getting the StoragePolicyUsage resource the second time", func() {
				BeforeEach(func() {
					skipGetStoragePolicyUsage = true
					numCalls := 0
					withFuncs.Get = func(
						ctx context.Context,
						client ctrlclient.WithWatch,
						key ctrlclient.ObjectKey,
						obj ctrlclient.Object,
						opts ...ctrlclient.GetOption) error {

						if _, ok := obj.(*spqv1.StoragePolicyUsage); ok {
							if numCalls == 1 {
								return fmt.Errorf(fake)
							}
							numCalls++
						}

						return client.Get(ctx, key, obj, opts...)
					}
				})
				It("should return an error", func() {
					Expect(err).To(MatchError(
						fmt.Sprintf(
							"failed to get StoragePolicyUsage %s/%s-vm-usage: %s",
							inNamespace, inName, fake)))
				})
			})
			When("there is an error patching the StoragePolicyUsage resource", func() {
				BeforeEach(func() {
					withFuncs.SubResourcePatch = func(
						ctx context.Context,
						client ctrlclient.Client,
						subResourceName string,
						obj ctrlclient.Object,
						patch ctrlclient.Patch,
						opts ...ctrlclient.SubResourcePatchOption) error {

						if _, ok := obj.(*spqv1.StoragePolicyUsage); ok {
							return fmt.Errorf(fake)
						}

						return client.Status().Patch(ctx, obj, patch, opts...)
					}
				})
				It("should return an error", func() {
					Expect(err).To(MatchError(
						fmt.Sprintf(
							"failed to patch StoragePolicyUsage %s/%s-vm-usage: %s",
							inNamespace, inName, fake)))
				})
			})
			Context("that are being deleted", func() {
				BeforeEach(func() {
					vm1.DeletionTimestamp = ptr.To(metav1.Now())
					vm1.Finalizers = []string{"fake.com/finalizer"}
				})
				Specify("the reported information should only include non-deleted VMs", func() {
					assertReportedTotals(spu, err, nil, zeroQuantity, resource.MustParse("1.5Gi"))
				})
			})
			Context("that do not have a true created condition", func() {
				BeforeEach(func() {
					vm1.Status.Conditions[0].Status = metav1.ConditionFalse
				})
				Context("that do not have an image ref", func() {
					BeforeEach(func() {
						vm1.Spec.Image = nil
					})
					Context("that do not have a created condition", func() {
						BeforeEach(func() {
							vm1.Status.Conditions = nil
						})
						Specify("the reported information should include VMs with a true created condition", func() {
							assertReportedTotals(spu, err, nil, zeroQuantity, resource.MustParse("1.5Gi"))
						})
					})
					Context("that have a false created condition", func() {
						Specify("the reported information should include VMs with a true created condition", func() {
							assertReportedTotals(spu, err, nil, zeroQuantity, resource.MustParse("1.5Gi"))
						})
					})
				})
				Context("that do have an image ref", func() {
					Context("that reference a VirtualMachineImage", func() {
						BeforeEach(func() {
							withObjects = append(
								withObjects,
								&vmopv1.VirtualMachineImage{
									ObjectMeta: metav1.ObjectMeta{
										Namespace: vm1.Namespace,
										Name:      vm1.Spec.Image.Name,
									},
									Status: vmopv1.VirtualMachineImageStatus{
										Disks: []vmopv1.VirtualMachineImageDiskInfo{
											{
												Capacity: ptr.To(resource.MustParse("20Gi")),
											},
											{
												Capacity: ptr.To(resource.MustParse("50Gi")),
											},
										},
									},
								},
							)
						})
						Specify("the reported reserved data should include VMs that do not have true created condition and used data should include VMs with a true created condition", func() {
							assertReportedTotals(spu, err, nil, resource.MustParse("70Gi"), resource.MustParse("1.5Gi"))
						})
						When("there is an error getting the VirtualMachineImage", func() {
							Context("the error is NotFound", func() {
								BeforeEach(func() {
									withFuncs.Get = func(
										ctx context.Context,
										client ctrlclient.WithWatch,
										key ctrlclient.ObjectKey,
										obj ctrlclient.Object,
										opts ...ctrlclient.GetOption) error {

										if _, ok := obj.(*vmopv1.VirtualMachineImage); ok {
											return apierrors.NewNotFound(
												vmopv1.GroupVersion.WithResource("VirtualMachineImage").GroupResource(),
												obj.GetName())
										}

										return client.Get(ctx, key, obj, opts...)
									}
								})
								Specify("the NotFound error should be ignored", func() {
									assertReportedTotals(spu, err, nil, zeroQuantity, resource.MustParse("1.5Gi"))
								})
							})
							Context("the error is not NotFound", func() {
								BeforeEach(func() {
									withFuncs.Get = func(
										ctx context.Context,
										client ctrlclient.WithWatch,
										key ctrlclient.ObjectKey,
										obj ctrlclient.Object,
										opts ...ctrlclient.GetOption) error {

										if _, ok := obj.(*vmopv1.VirtualMachineImage); ok {
											return fmt.Errorf(fake)
										}

										return client.Get(ctx, key, obj, opts...)
									}
								})
								Specify("an error should occur", func() {
									assertReportedErr(spu, err, errFailedToGetImg(*vm1))
								})
							})
						})
					})
					Context("that reference a ClusterVirtualMachineImage", func() {
						BeforeEach(func() {
							vm1.Spec.Image.Kind = "ClusterVirtualMachineImage"
							withObjects = append(
								withObjects,
								&vmopv1.ClusterVirtualMachineImage{
									ObjectMeta: metav1.ObjectMeta{
										Name: vm1.Spec.Image.Name,
									},
									Status: vmopv1.VirtualMachineImageStatus{
										Disks: []vmopv1.VirtualMachineImageDiskInfo{
											{
												Capacity: ptr.To(resource.MustParse("20Gi")),
											},
											{
												Capacity: ptr.To(resource.MustParse("50Gi")),
											},
										},
									},
								},
							)
						})
						Specify("the reported reserved data should include VMs that do not have true created condition and used data should include VMs with a true created condition", func() {
							assertReportedTotals(spu, err, nil, resource.MustParse("70Gi"), resource.MustParse("1.5Gi"))
						})
					})
				})
			})
			Context("that have a true created condition", func() {
				Context("that have no storage status", func() {
					BeforeEach(func() {
						vm2.Status.Storage = nil
					})
					Context("that do not have an image ref", func() {
						BeforeEach(func() {
							vm2.Spec.Image = nil
						})
						Specify("the reported information should include VMs with a storage status", func() {
							assertReportedTotals(spu, err, nil, zeroQuantity, resource.MustParse("3Gi"))
						})
					})
					Context("that do have an image ref", func() {
						Context("that reference a VirtualMachineImage", func() {
							BeforeEach(func() {
								withObjects = append(
									withObjects,
									&vmopv1.VirtualMachineImage{
										ObjectMeta: metav1.ObjectMeta{
											Namespace: vm2.Namespace,
											Name:      vm2.Spec.Image.Name,
										},
										Status: vmopv1.VirtualMachineImageStatus{
											Disks: []vmopv1.VirtualMachineImageDiskInfo{
												{
													Capacity: ptr.To(resource.MustParse("10Gi")),
												},
												{
													Capacity: ptr.To(resource.MustParse("40Gi")),
												},
											},
										},
									},
								)
							})
							Specify("the reported reserved data should include VMs that do not have true created condition and used data should include VMs with a true created condition", func() {
								assertReportedTotals(spu, err, nil, resource.MustParse("50Gi"), resource.MustParse("3Gi"))
							})
						})
						Context("that reference a ClusterVirtualMachineImage", func() {
							BeforeEach(func() {
								vm2.Spec.Image.Kind = "ClusterVirtualMachineImage"
								withObjects = append(
									withObjects,
									&vmopv1.ClusterVirtualMachineImage{
										ObjectMeta: metav1.ObjectMeta{
											Name: vm2.Spec.Image.Name,
										},
										Status: vmopv1.VirtualMachineImageStatus{
											Disks: []vmopv1.VirtualMachineImageDiskInfo{
												{
													Capacity: ptr.To(resource.MustParse("10Gi")),
												},
												{
													Capacity: ptr.To(resource.MustParse("40Gi")),
												},
											},
										},
									},
								)
							})
							Specify("the reported reserved data should include VMs that do not have true created condition and used data should include VMs with a true created condition", func() {
								assertReportedTotals(spu, err, nil, resource.MustParse("50Gi"), resource.MustParse("3Gi"))
							})
							When("there is an error getting the ClusterVirtualMachineImage", func() {
								Context("the error is NotFound", func() {
									BeforeEach(func() {
										withFuncs.Get = func(
											ctx context.Context,
											client ctrlclient.WithWatch,
											key ctrlclient.ObjectKey,
											obj ctrlclient.Object,
											opts ...ctrlclient.GetOption) error {

											if _, ok := obj.(*vmopv1.ClusterVirtualMachineImage); ok {
												return apierrors.NewNotFound(
													vmopv1.GroupVersion.WithResource("ClusterVirtualMachineImage").GroupResource(),
													obj.GetName())
											}

											return client.Get(ctx, key, obj, opts...)
										}
									})
									Specify("the NotFound error should be ignored", func() {
										assertReportedTotals(spu, err, nil, zeroQuantity, resource.MustParse("3Gi"))
									})
								})
								Context("the error is not NotFound", func() {
									BeforeEach(func() {
										withFuncs.Get = func(
											ctx context.Context,
											client ctrlclient.WithWatch,
											key ctrlclient.ObjectKey,
											obj ctrlclient.Object,
											opts ...ctrlclient.GetOption) error {

											if _, ok := obj.(*vmopv1.ClusterVirtualMachineImage); ok {
												return fmt.Errorf(fake)
											}

											return client.Get(ctx, key, obj, opts...)
										}
									})
									Specify("an error should occur", func() {
										assertReportedErr(spu, err, errFailedToGetImg(*vm2))
									})
								})
							})
						})
					})
				})
				Context("that have a storage status", func() {
					Context("with no PVCs", func() {
						BeforeEach(func() {
							vm1.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{vm1.Status.Volumes[0]}
							vm2.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{vm2.Status.Volumes[0]}
						})
						Specify("the reported information should reflect the VM data only", func() {
							assertReportedTotals(spu, err, nil, zeroQuantity, resource.MustParse("7Gi"))
						})
					})
					Context("with PVCs", func() {
						Specify("the reported information should reflect the VM data only", func() {
							assertReportedTotals(spu, err, nil, zeroQuantity, resource.MustParse("4.5Gi"))
						})
					})
				})
			})
		})
	})
}
