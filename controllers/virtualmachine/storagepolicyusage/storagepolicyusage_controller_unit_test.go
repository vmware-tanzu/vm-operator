// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyusage_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/storagepolicyusage"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"ReconcileSPU",
		Label(
			testlabels.Controller,
		),
		unitTestsReconcile,
	)
	Describe(
		"ReconcileSPUForVM",
		Label(
			testlabels.Controller,
		),
		unitTestsReconcileSPUForVM,
	)
	Describe(
		"ReconcileSPUForVMSnapshot",
		Label(
			testlabels.Controller,
		),
		unitTestsReconcileSPUForVMSnapshot,
	)
}

func unitTestsReconcile() {
	Context("with VMSnapshots feature flag enabled", func() {
		const (
			fake      = "fake"
			namespace = "default"
			name      = "my-storage-class"
		)

		var (
			reconciler       *storagepolicyusage.Reconciler
			ctx              *builder.UnitTestContextForController
			inNamespace      string
			inName           string
			withFuncs        interceptor.Funcs
			withObjects      []ctrlclient.Object
			err              error
			zeroQuantity     resource.Quantity
			vm1              *vmopv1.VirtualMachine
			vmSnapshot1      *vmopv1.VirtualMachineSnapshot
			size10GB         resource.Quantity
			spuForVM         spqv1.StoragePolicyUsage
			spuForVMSnapshot spqv1.StoragePolicyUsage
		)

		assertReportedTotalsWithOffsetIgnoreError := func(
			spu spqv1.StoragePolicyUsage,
			expReserved, expUsed resource.Quantity,
			offset int) {

			assertReportedTotalsWithOffset(spu, nil, nil, expReserved, expUsed, offset)
		}

		BeforeEach(func() {
			withObjects = nil
			withFuncs = interceptor.Funcs{}
			inNamespace = namespace
			inName = name
			zeroQuantity = resource.MustParse("0Gi")
			size10GB = resource.MustParse("10Gi")

			spuForVM = spqv1.StoragePolicyUsage{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      spqutil.StoragePolicyUsageNameForVM(name),
				},
				Spec: spqv1.StoragePolicyUsageSpec{
					StoragePolicyId:  fake,
					StorageClassName: name,
				},
			}
			spuForVMSnapshot = spqv1.StoragePolicyUsage{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(name),
				},
				Spec: spqv1.StoragePolicyUsageSpec{
					StoragePolicyId:  fake,
					StorageClassName: name,
				},
			}
			vm1 = builder.DummyBasicVirtualMachine("vm1", inNamespace)
			vm1.Spec.StorageClass = inName
			vm1.Status = vmopv1.VirtualMachineStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vmopv1.VirtualMachineConditionCreated,
						Status: metav1.ConditionTrue,
					},
				},
				Storage: &vmopv1.VirtualMachineStorageStatus{
					Total: ptr.To(size10GB),
				},
			}
			vmSnapshot1 = builder.DummyVirtualMachineSnapshot(inNamespace, "snapshot1", "vm1")
			vmSnapshot1.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey] = constants.CSIVSphereVolumeSyncAnnotationValueCompleted
			vmSnapshot1.Status.Storage = &vmopv1.VirtualMachineSnapshotStorageStatus{
				Used: &size10GB,
				Requested: []vmopv1.VirtualMachineSnapshotStorageStatusRequested{
					{
						StorageClass: inName,
						Total:        &size10GB,
					},
				},
			}
			withObjects = append(withObjects, vm1, vmSnapshot1)
		})

		JustBeforeEach(func() {
			ctx = suite.NewUnitTestContextForController()

			// Replace the client with one that has the indexed field.
			ctx.Client = ctrlfake.NewClientBuilder().
				WithScheme(builder.NewScheme()).
				WithIndex(
					&vmopv1.VirtualMachine{},
					"spec.storageClass",
					func(rawObj ctrlclient.Object) []string {
						vm := rawObj.(*vmopv1.VirtualMachine)
						return []string{vm.Spec.StorageClass}
					}).
				WithObjects(withObjects...).
				WithInterceptorFuncs(withFuncs).
				WithStatusSubresource(builder.KnownObjectTypes()...).
				Build()

			reconciler = storagepolicyusage.NewReconciler(
				pkgcfg.UpdateContext(
					ctx,
					func(config *pkgcfg.Config) {
						config.Features.PodVMOnStretchedSupervisor = true
						// Enable VMSnapshots feature.
						config.Features.VMSnapshots = true
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

		When("there is an error listing VM resources", func() {
			BeforeEach(func() {
				withFuncs.List = func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					list ctrlclient.ObjectList,
					opts ...ctrlclient.ListOption) error {

					if _, ok := list.(*vmopv1.VirtualMachineList); ok {
						return errors.New(fake)
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

		When("there are StoragePolicyUsage resources for VM and VMSnapshot", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &spuForVM, &spuForVMSnapshot)
			})
			JustBeforeEach(func() {
				Expect(ctx.Client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: spqutil.StoragePolicyUsageNameForVM(name)}, &spuForVM)).To(Succeed())
				Expect(ctx.Client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: spqutil.StoragePolicyUsageNameForVMSnapshot(name)}, &spuForVMSnapshot)).To(Succeed())
			})
			It("should report the correct usage", func() {
				Expect(err).To(Succeed())
				assertReportedTotalsWithOffsetIgnoreError(spuForVM, zeroQuantity, size10GB, 1)
				assertReportedTotalsWithOffsetIgnoreError(spuForVMSnapshot, zeroQuantity, size10GB, 1)
			})

			Context("There are VM with different storage class", func() {
				BeforeEach(func() {
					vm3 := builder.DummyBasicVirtualMachine("vm3", inNamespace)
					vm3.Spec.StorageClass = inName + "1"
					vm3.Status = vmopv1.VirtualMachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:   vmopv1.VirtualMachineConditionCreated,
								Status: metav1.ConditionTrue,
							},
						},
						Storage: &vmopv1.VirtualMachineStorageStatus{
							Total: ptr.To(size10GB),
						},
					}
				})
				Specify("the reported information should only include VMs that use the same storage class", func() {
					assertReportedTotalsWithOffsetIgnoreError(spuForVM, zeroQuantity, size10GB, 1)
				})
			})
		})

		When("there is no StoragePolicyUsage resource for VM, while there is one for VMSnapshot", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &spuForVMSnapshot)
			})
			JustBeforeEach(func() {
				Expect(ctx.Client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: spqutil.StoragePolicyUsageNameForVMSnapshot(name)}, &spuForVMSnapshot)).To(Succeed())
			})
			It("should report the correct usage for SPU for VMSnapshot, returns error for VM", func() {
				Expect(err).To(MatchError(
					fmt.Sprintf(
						"failed to get StoragePolicyUsage %s/%s-vm-usage: "+
							"storagepolicyusages.cns.vmware.com \"%s-vm-usage\" not found",
						inNamespace, inName, inName)))
				assertReportedTotalsWithOffsetIgnoreError(spuForVMSnapshot, zeroQuantity, size10GB, 1)
			})
		})

		When("there is no StoragePolicyUsage resource for VMSnapshot, while there is one for VM", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &spuForVM)
			})
			JustBeforeEach(func() {
				Expect(ctx.Client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: spqutil.StoragePolicyUsageNameForVM(name)}, &spuForVM)).To(Succeed())
			})
			It("should report the correct usage for SPU for VM, returns error", func() {
				Expect(err).To(MatchError(
					fmt.Sprintf(
						"failed to get StoragePolicyUsage %s/%s-vmsnapshot-usage: "+
							"storagepolicyusages.cns.vmware.com \"%s-vmsnapshot-usage\" not found",
						inNamespace, inName, inName)))
				assertReportedTotalsWithOffsetIgnoreError(spuForVM, zeroQuantity, size10GB, 1)
			})
		})

		When("there is no StoragePolicyUsage resource for VM and VMSnapshot", func() {
			It("should return error for both VM and VMSnapshot", func() {
				Expect(err.Error()).To(ContainSubstring(
					fmt.Sprintf(
						"failed to get StoragePolicyUsage %s/%s-vm-usage: "+
							"storagepolicyusages.cns.vmware.com \"%s-vm-usage\" not found",
						inNamespace, inName, inName)))
				Expect(err.Error()).To(ContainSubstring(
					fmt.Sprintf(
						"failed to get StoragePolicyUsage %s/%s-vmsnapshot-usage: "+
							"storagepolicyusages.cns.vmware.com \"%s-vmsnapshot-usage\" not found",
						inNamespace, inName, inName)))
			})
		})
	})

	Context("with VMSnapshots feature flag disabled", func() {
		const (
			fake      = "fake"
			namespace = "default"
			name      = "my-storage-class"
		)

		var (
			reconciler  *storagepolicyusage.Reconciler
			ctx         *builder.UnitTestContextForController
			inNamespace string
			inName      string
			withObjects []ctrlclient.Object
			vmSnapshot1 *vmopv1.VirtualMachineSnapshot
			err         error
			vm1         *vmopv1.VirtualMachine
			size10GB    = resource.MustParse("10Gi")
		)

		BeforeEach(func() {
			withObjects = nil
			inNamespace = namespace
			inName = name
		})

		JustBeforeEach(func() {
			ctx = suite.NewUnitTestContextForController(withObjects...)
			// Replace the client with one that has the indexed field.
			ctx.Client = ctrlfake.NewClientBuilder().
				WithScheme(builder.NewScheme()).
				WithIndex(
					&vmopv1.VirtualMachine{},
					"spec.storageClass",
					func(rawObj ctrlclient.Object) []string {
						vm := rawObj.(*vmopv1.VirtualMachine)
						return []string{vm.Spec.StorageClass}
					}).
				WithObjects(withObjects...).
				WithInterceptorFuncs(interceptor.Funcs{}).
				WithStatusSubresource(builder.KnownObjectTypes()...).
				Build()

			reconciler = storagepolicyusage.NewReconciler(
				pkgcfg.UpdateContext(
					ctx,
					func(config *pkgcfg.Config) {
						config.Features.PodVMOnStretchedSupervisor = true
						config.Features.VMSnapshots = false
					},
				),
				ctx.Client,
				ctx.Logger,
				ctx.Recorder,
			)

			err = reconciler.ReconcileNormal(ctx, inNamespace, inName)
		})

		When("a VirtualMachineSnapshot is found", func() {
			BeforeEach(func() {
				vm1 = builder.DummyBasicVirtualMachine("vm1", inNamespace)
				vm1.Spec.StorageClass = inName
				vmSnapshot1 = builder.DummyVirtualMachineSnapshot(inNamespace, "snapshot1", "vm1")
				vmSnapshot1.Status.Storage = &vmopv1.VirtualMachineSnapshotStorageStatus{
					Used: &size10GB,
					Requested: []vmopv1.VirtualMachineSnapshotStorageStatusRequested{
						{
							StorageClass: inName,
							Total:        &size10GB,
						},
					},
				}
				withObjects = append(withObjects, vm1, vmSnapshot1)
			})
			When("there is a StoragePolicyUsage resource for VMSnapshot (it shouldn't exist)", func() {
				var (
					spuForVM         spqv1.StoragePolicyUsage
					spuForVMSnapshot spqv1.StoragePolicyUsage
				)
				BeforeEach(func() {
					spuForVM = spqv1.StoragePolicyUsage{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      spqutil.StoragePolicyUsageNameForVM(name),
						},
						Spec: spqv1.StoragePolicyUsageSpec{
							StoragePolicyId:  fake,
							StorageClassName: name,
						},
					}
					spuForVMSnapshot = spqv1.StoragePolicyUsage{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(name),
						},
						Spec: spqv1.StoragePolicyUsageSpec{
							StoragePolicyId:  fake,
							StorageClassName: name,
						},
					}
					withObjects = append(
						withObjects,
						&spuForVM,
						&spuForVMSnapshot,
					)
				})
				AfterEach(func() {
					spuForVM = spqv1.StoragePolicyUsage{}
					spuForVMSnapshot = spqv1.StoragePolicyUsage{}
				})
				JustBeforeEach(func() {
					Expect(ctx.Client.Get(
						ctx,
						ctrlclient.ObjectKey{
							Namespace: namespace,
							Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(name),
						},
						&spuForVMSnapshot,
					)).To(Succeed())
				})

				It("should not update the SPU for VMSnapshot since the feature is disabled", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(spuForVMSnapshot.Status.ResourceTypeLevelQuotaUsage).To(BeNil())
				})
			})
		})
	})
}

func unitTestsReconcileSPUForVM() {
	const (
		fake      = "fake"
		namespace = "default"
		name      = "my-storage-class"
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
		size10GB                  resource.Quantity
		size20GB                  resource.Quantity
		size40GB                  resource.Quantity
		size50GB                  resource.Quantity
		size70GB                  resource.Quantity
	)

	BeforeEach(func() {
		withObjects = nil
		withFuncs = interceptor.Funcs{}
		inNamespace = namespace
		inName = name
		skipGetStoragePolicyUsage = false
		zeroQuantity = resource.MustParse("0Gi")
		size10GB = resource.MustParse("10Gi")
		size20GB = resource.MustParse("20Gi")
		size40GB = resource.MustParse("40Gi")
		size50GB = resource.MustParse("50Gi")
		size70GB = resource.MustParse("70Gi")
	})

	JustBeforeEach(func() {
		if vm1 != nil {
			withObjects = append(withObjects, vm1)
		}
		if vm2 != nil {
			withObjects = append(withObjects, vm2)
		}

		ctx = suite.NewUnitTestContextForController()
		ctx.Client = ctrlfake.NewClientBuilder().
			WithScheme(builder.NewScheme()).
			WithObjects(withObjects...).
			WithInterceptorFuncs(withFuncs).
			WithStatusSubresource(builder.KnownObjectTypes()...).
			Build()
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

		listOfVMs := []vmopv1.VirtualMachine{}
		if vm1 != nil {
			listOfVMs = append(listOfVMs, *vm1)
		}
		if vm2 != nil {
			listOfVMs = append(listOfVMs, *vm2)
		}

		err = reconciler.ReconcileSPUForVM(ctx, inNamespace, inName, listOfVMs)
	})

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
						Name:      spqutil.StoragePolicyUsageNameForVM(name),
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
						Name:      spqutil.StoragePolicyUsageNameForVM(name),
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
							return errors.New(fake)
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
						StorageClass: name,
					},
					Status: vmopv1.VirtualMachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:   vmopv1.VirtualMachineConditionCreated,
								Status: metav1.ConditionTrue,
							},
						},
						Storage: &vmopv1.VirtualMachineStorageStatus{
							Total: ptr.To(size10GB),
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
						StorageClass: name,
					},
					Status: vmopv1.VirtualMachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:   vmopv1.VirtualMachineConditionCreated,
								Status: metav1.ConditionTrue,
							},
						},
						Storage: &vmopv1.VirtualMachineStorageStatus{
							Total: ptr.To(size10GB),
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
								return errors.New(fake)
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
							return errors.New(fake)
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
					assertReportedTotals(spu, err, nil, zeroQuantity, size10GB)
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
							assertReportedTotals(spu, err, nil, zeroQuantity, size10GB)
						})
					})
					Context("that have a false created condition", func() {
						Specify("the reported information should include VMs with a true created condition", func() {
							assertReportedTotals(spu, err, nil, zeroQuantity, size10GB)
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
												Capacity: ptr.To(size20GB),
											},
											{
												Capacity: ptr.To(size50GB),
											},
										},
									},
								},
							)
						})
						Specify("the reported reserved data should include VMs that do not have true created condition and used data should include VMs with a true created condition", func() {
							assertReportedTotals(spu, err, nil, size70GB, size10GB)
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
									assertReportedTotals(spu, err, nil, zeroQuantity, size10GB)
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
											return errors.New(fake)
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
												Capacity: ptr.To(size20GB),
											},
											{
												Capacity: ptr.To(size50GB),
											},
										},
									},
								},
							)
						})
						Specify("the reported reserved data should include VMs that do not have true created condition and used data should include VMs with a true created condition", func() {
							assertReportedTotals(spu, err, nil, size70GB, size10GB)
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
							assertReportedTotals(spu, err, nil, zeroQuantity, size10GB)
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
													Capacity: ptr.To(size10GB),
												},
												{
													Capacity: ptr.To(size40GB),
												},
											},
										},
									},
								)
							})
							Specify("the reported reserved data should include VMs that do not have true created condition and used data should include VMs with a true created condition", func() {
								assertReportedTotals(spu, err, nil, size50GB, size10GB)
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
													Capacity: ptr.To(size10GB),
												},
												{
													Capacity: ptr.To(size40GB),
												},
											},
										},
									},
								)
							})
							Specify("the reported reserved data should include VMs that do not have true created condition and used data should include VMs with a true created condition", func() {
								assertReportedTotals(spu, err, nil, size50GB, size10GB)
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
										assertReportedTotals(spu, err, nil, zeroQuantity, size10GB)
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
												return errors.New(fake)
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
			})
		})
	})
}

func unitTestsReconcileSPUForVMSnapshot() {
	const (
		fake      = "fake"
		namespace = "default"
		name      = "my-storage-class"
	)

	var (
		reconciler   *storagepolicyusage.Reconciler
		ctx          *builder.UnitTestContextForController
		inNamespace  string
		inName       string
		withObjects  []ctrlclient.Object
		vmSnapshot1  *vmopv1.VirtualMachineSnapshot
		vmSnapshot2  *vmopv1.VirtualMachineSnapshot
		err          error
		vm1          *vmopv1.VirtualMachine
		vm2          *vmopv1.VirtualMachine
		zeroQuantity = resource.MustParse("0Gi")
		size10GB     = resource.MustParse("10Gi")
		size20GB     = resource.MustParse("20Gi")
		size30GB     = resource.MustParse("30Gi")
	)

	BeforeEach(func() {
		withObjects = nil
		inNamespace = namespace
		inName = name
	})

	JustBeforeEach(func() {
		if vm1 != nil {
			withObjects = append(withObjects, vm1)
		}
		if vm2 != nil {
			withObjects = append(withObjects, vm2)
		}
		if vmSnapshot1 != nil {
			withObjects = append(withObjects, vmSnapshot1)
		}
		if vmSnapshot2 != nil {
			withObjects = append(withObjects, vmSnapshot2)
		}

		ctx = suite.NewUnitTestContextForController(withObjects...)
		reconciler = storagepolicyusage.NewReconciler(
			pkgcfg.UpdateContext(
				ctx,
				func(config *pkgcfg.Config) {
					config.Features.PodVMOnStretchedSupervisor = true
					config.Features.VMSnapshots = true
				},
			),
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
		)

		listOfVMs := []vmopv1.VirtualMachine{}
		if vm1 != nil && vm1.Spec.StorageClass == inName {
			listOfVMs = append(listOfVMs, *vm1)
		}
		if vm2 != nil && vm2.Spec.StorageClass == inName {
			listOfVMs = append(listOfVMs, *vm2)
		}

		err = reconciler.ReconcileSPUForVMSnapshot(ctx, inNamespace, inName, listOfVMs)
	})

	When("there is no StoragePolicyUsage resource", func() {
		It("should no-op", func() {
			Expect(err).To(MatchError(
				fmt.Sprintf(
					"failed to get StoragePolicyUsage %[1]s/%[2]s-vmsnapshot-usage: "+
						"storagepolicyusages.cns.vmware.com \"%[2]s-vmsnapshot-usage\" not found",
					inNamespace, inName)))
		})
	})

	When("there is a StoragePolicyUsage resource for VMSnapshot", func() {
		var (
			spu spqv1.StoragePolicyUsage
		)
		BeforeEach(func() {
			withObjects = append(
				withObjects,
				&spqv1.StoragePolicyUsage{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(name),
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
			Expect(ctx.Client.Get(
				ctx,
				ctrlclient.ObjectKey{
					Namespace: namespace,
					Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(name),
				},
				&spu,
			)).To(Succeed())
		})

		When("no VirtualMachineSnapshot is found", func() {
			It("should report the snapshot's reserved capacity with 0Gi", func() {
				assertReportedTotalsWithOffset(spu, err, nil, zeroQuantity, zeroQuantity, 1)
			})
		})

		When("a VirtualMachineSnapshot is found", func() {
			BeforeEach(func() {
				vm1 = builder.DummyBasicVirtualMachine("vm1", inNamespace)
				vm1.Spec.StorageClass = inName
				vmSnapshot1 = builder.DummyVirtualMachineSnapshot(inNamespace, "snapshot1", "vm1")
				vmSnapshot1.Status.Storage = &vmopv1.VirtualMachineSnapshotStorageStatus{
					Used: &size10GB,
					Requested: []vmopv1.VirtualMachineSnapshotStorageStatusRequested{
						{
							StorageClass: inName,
							Total:        &size10GB,
						},
					},
				}
			})
			When("CSI driver has not completed the sync of the volume", func() {
				BeforeEach(func() {
					vmSnapshot1.Annotations = map[string]string{
						constants.CSIVSphereVolumeSyncAnnotationKey: constants.CSIVSphereVolumeSyncAnnotationValueRequested,
					}
				})

				It("should report the snapshot's reserved capacity with VM's classic disk capacity, and ignore the used capacity", func() {
					assertReportedTotalsWithOffset(spu, err, nil, size10GB, zeroQuantity, 1)
				})

				When("CSIVSphereVolumeSyncAnnotation is not set", func() {
					BeforeEach(func() {
						vmSnapshot1.Annotations = nil
					})

					It("should report the snapshot's reserved capacity with VM's classic disk capacity, and used capacity with 0Gi", func() {
						assertReportedTotalsWithOffset(spu, err, nil, size10GB, zeroQuantity, 1)
					})
				})

				When("VMSnapshot has empty status.requested", func() {
					BeforeEach(func() {
						vmSnapshot1.Status.Storage.Requested = nil
					})
					It("should ignore the snapshot when calculating the reserved capacity", func() {
						assertReportedTotalsWithOffset(spu, err, nil, zeroQuantity, zeroQuantity, 1)
					})
				})

				When("VMRef of the VirtualMachineSnapshot is not set", func() {
					BeforeEach(func() {
						vmSnapshot1.Spec.VMRef = nil
					})
					It("should return an error", func() {
						Expect(err).To(HaveOccurred())
					})
				})

				When("The requested storage class doesn't match the SPU's storage class", func() {
					BeforeEach(func() {
						vmSnapshot1.Status.Storage.Requested[0].StorageClass = "different-storage-class"
					})
					It("should ignore the snapshot when calculating the reserved and used capacity", func() {
						assertReportedTotalsWithOffset(spu, err, nil, zeroQuantity, zeroQuantity, 1)
					})
				})
			})

			When("CSI driver has completed syncing the volume on the VM", func() {
				BeforeEach(func() {
					vmSnapshot1.Annotations = map[string]string{
						constants.CSIVSphereVolumeSyncAnnotationKey: constants.CSIVSphereVolumeSyncAnnotationValueCompleted,
					}
				})

				It("should ignore the reserved capacity, and report the used capacity with VM's used capacity", func() {
					assertReportedTotalsWithOffset(spu, err, nil, zeroQuantity, size10GB, 1)
				})

				When("VMRef of the VirtualMachineSnapshot is not set", func() {
					BeforeEach(func() {
						vmSnapshot1.Spec.VMRef = nil
					})
					It("should return an error", func() {
						Expect(err).To(HaveOccurred())
					})
				})

				When("VMSnapshot has empty status.used", func() {
					BeforeEach(func() {
						vmSnapshot1.Status.Storage.Used = nil
					})
					It("should report the snapshot's reserved capacity with 0Gi, and used capacity with 0Gi", func() {
						assertReportedTotalsWithOffset(spu, err, nil, zeroQuantity, zeroQuantity, 1)
					})
				})
			})
		})

		When("Two VirtualMachineSnapshots are found", func() {
			BeforeEach(func() {
				vm1 = builder.DummyBasicVirtualMachine("vm1", inNamespace)
				vm1.Spec.StorageClass = inName
				vmSnapshot1 = builder.DummyVirtualMachineSnapshot(inNamespace, "snapshot1", vm1.Name)
				vmSnapshot1.Status.Storage = &vmopv1.VirtualMachineSnapshotStorageStatus{
					Used: &size10GB,
					Requested: []vmopv1.VirtualMachineSnapshotStorageStatusRequested{
						{
							StorageClass: inName,
							Total:        &size10GB,
						},
					},
				}
				vm2 = builder.DummyBasicVirtualMachine("vm2", inNamespace)
				vm2.Spec.StorageClass = inName
				vmSnapshot2 = builder.DummyVirtualMachineSnapshot(inNamespace, "snapshot2", vm2.Name)
				vmSnapshot2.Status.Storage = &vmopv1.VirtualMachineSnapshotStorageStatus{
					Used: &size20GB,
					Requested: []vmopv1.VirtualMachineSnapshotStorageStatusRequested{
						{
							StorageClass: inName,
							Total:        &size20GB,
						},
					},
				}
			})

			When("One VMSnapshot has empty status.requested", func() {
				BeforeEach(func() {
					vmSnapshot1.Status.Storage.Requested = nil
				})
				It("should skip the snapshot with empty status.requested", func() {
					assertReportedTotalsWithOffset(spu, err, nil, size20GB, zeroQuantity, 1)
				})
			})

			When("CSI driver has not completed the sync of the volume on both VMs", func() {
				BeforeEach(func() {
					vmSnapshot1.Annotations = map[string]string{
						constants.CSIVSphereVolumeSyncAnnotationKey: constants.CSIVSphereVolumeSyncAnnotationValueRequested,
					}
					vmSnapshot2.Annotations = map[string]string{
						constants.CSIVSphereVolumeSyncAnnotationKey: constants.CSIVSphereVolumeSyncAnnotationValueRequested,
					}
				})
				It("should report the snapshot's reserved capacity with sum of the two VMs' reserved capacity, ignore the used capacity", func() {
					assertReportedTotalsWithOffset(spu, err, nil, size30GB, zeroQuantity, 1)
				})
			})

			When("CSI driver has completed the sync of the volume on both VMs", func() {
				BeforeEach(func() {
					vmSnapshot1.Annotations = map[string]string{
						constants.CSIVSphereVolumeSyncAnnotationKey: constants.CSIVSphereVolumeSyncAnnotationValueCompleted,
					}
					vmSnapshot2.Annotations = map[string]string{
						constants.CSIVSphereVolumeSyncAnnotationKey: constants.CSIVSphereVolumeSyncAnnotationValueCompleted,
					}
				})

				It("should ignore the reserved capacity, and report the used capacity with sum of the two VMs' used capacity", func() {
					assertReportedTotalsWithOffset(spu, err, nil, zeroQuantity, size30GB, 1)
				})

				When("vmSnapshot2 has no used capacity reported by controller yet", func() {
					BeforeEach(func() {
						vmSnapshot1.Status.Storage.Used = nil
					})
					It("should report the snapshot's reserved capacity with 0Gi, and used capacity with the used capacity of vmSnapshot2", func() {
						assertReportedTotalsWithOffset(spu, err, nil, zeroQuantity, size20GB, 1)
					})
				})

				When("vmSnapshot1's VMRef has different storage class", func() {
					BeforeEach(func() {
						vm1.Spec.StorageClass = "different-storage-class"
					})
					It("should only report the reserved and used capacity of the snapshot with the same storage class", func() {
						assertReportedTotalsWithOffset(spu, err, nil, zeroQuantity, size20GB, 1)
					})
				})
			})

			When("CSI driver has not completed the sync of the volume on one VM, but completed on the other", func() {
				BeforeEach(func() {
					vmSnapshot1.Annotations = map[string]string{
						constants.CSIVSphereVolumeSyncAnnotationKey: constants.CSIVSphereVolumeSyncAnnotationValueRequested,
					}
					vmSnapshot2.Annotations = map[string]string{
						constants.CSIVSphereVolumeSyncAnnotationKey: constants.CSIVSphereVolumeSyncAnnotationValueCompleted,
					}
				})

				It("should report the snapshot's reserved capacity with not completed VM's reserved capacity, and used capacity with completed VM's used capacity", func() {
					assertReportedTotalsWithOffset(spu, err, nil, size10GB, size20GB, 1)
				})
			})
		})
	})
}

func assertReportedTotalsWithOffset(
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
