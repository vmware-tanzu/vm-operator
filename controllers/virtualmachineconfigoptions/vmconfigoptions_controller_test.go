// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineconfigoptions_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	vmconfigoptions "github.com/vmware-tanzu/vm-operator/controllers/virtualmachineconfigoptions"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {}

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.API,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	const (
		objName        = "vmx-22-config"
		clusterMoID    = "domain-c1234"
		hardwareVersion = "vmx-22"
	)

	var (
		initObjects    []client.Object
		ctx            *builder.UnitTestContextForController
		reconciler     *vmconfigoptions.Reconciler
		fakeVMProvider *providerfake.VMProvider

		configOptions *vimv1.VirtualMachineConfigOptions
		configTarget  *vimv1.ConfigTarget
		objKey        types.NamespacedName
	)

	BeforeEach(func() {
		initObjects = nil

		configTarget = &vimv1.ConfigTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster-config-target",
			},
			Spec: vimv1.ConfigTargetSpec{
				ID: vimv1.ManagedObjectID{
					ID: clusterMoID,
				},
			},
		}

		configOptions = &vimv1.VirtualMachineConfigOptions{
			ObjectMeta: metav1.ObjectMeta{
				Name: objName,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: vimv1.GroupVersion.String(),
						Kind:       "ConfigTarget",
						Name:       configTarget.Name,
						UID:        configTarget.UID,
					},
				},
			},
			Spec: vimv1.VirtualMachineConfigOptionsSpec{
				HardwareVersion: hardwareVersion,
			},
		}

		objKey = types.NamespacedName{Name: objName}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
		reconciler = vmconfigoptions.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		reconciler = nil
		fakeVMProvider = nil
	})

	doReconcile := func() (reconcile.Result, error) {
		return reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{NamespacedName: objKey})
	}

	Context("Reconcile", func() {
		When("VirtualMachineConfigOptions does not exist", func() {
			It("should not error", func() {
				_, err := doReconcile()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("VirtualMachineConfigOptions has no owner references", func() {
			BeforeEach(func() {
				configOptions.OwnerReferences = nil
				initObjects = append(initObjects, configOptions)
			})

			It("should requeue with a delay", func() {
				// First reconcile adds the finalizer and returns early.
				_, err := doReconcile()
				Expect(err).ToNot(HaveOccurred())
				// Second reconcile finds no ConfigTarget owner and requeues.
				result, err := doReconcile()
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			})
		})

		When("VirtualMachineConfigOptions and ConfigTarget exist", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, configTarget, configOptions)
			})

			When("QueryConfigOptionEx returns an error", func() {
				JustBeforeEach(func() {
					fakeVMProvider.QueryConfigOptionExFn = func(_ context.Context, _, _ string) (*vimtypes.VirtualMachineConfigOption, error) {
						return nil, errors.New("vc unavailable")
					}
				})

				It("should mark Ready=False with QueryFailed reason and return error", func() {
					// First reconcile adds finalizer; second surfaces the error.
					_, _ = doReconcile()
					_, err := doReconcile()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("vc unavailable"))

					current := &vimv1.VirtualMachineConfigOptions{}
					Expect(ctx.Client.Get(ctx, objKey, current)).To(Succeed())
					Expect(pkgcond.IsFalse(current, vimv1.ReadyConditionType)).To(BeTrue())
					Expect(pkgcond.GetReason(current, vimv1.ReadyConditionType)).To(Equal("QueryFailed"))
				})
			})

			When("reconcile succeeds", func() {
				JustBeforeEach(func() {
					fakeVMProvider.QueryConfigOptionExFn = func(_ context.Context, _, _ string) (*vimtypes.VirtualMachineConfigOption, error) {
						return &vimtypes.VirtualMachineConfigOption{
							Description:         "Test config option",
							GuestOSDefaultIndex: 0,
							GuestOSDescriptor: []vimtypes.GuestOsDescriptor{
								{
									Id:               "otherLinux64Guest",
									FullName:         "Other Linux (64-bit)",
									Family:           "linuxGuest",
									SupportedMaxCPUs: 64,
								},
								{
									Id:               "windows9Server64Guest",
									FullName:         "Microsoft Windows Server 2016 (64-bit)",
									Family:           "windowsGuest",
									SupportedMaxCPUs: 128,
								},
							},
						}, nil
					}
				})

				It("should populate status.guestOSIdentifiers and mark Ready=True", func() {
					// First reconcile adds finalizer.
					_, err := doReconcile()
					Expect(err).ToNot(HaveOccurred())
					// Second reconcile performs the full reconciliation.
					_, err = doReconcile()
					Expect(err).ToNot(HaveOccurred())

					current := &vimv1.VirtualMachineConfigOptions{}
					Expect(ctx.Client.Get(ctx, objKey, current)).To(Succeed())
					Expect(pkgcond.IsTrue(current, vimv1.ReadyConditionType)).To(BeTrue())
					Expect(current.Status.GuestOSIdentifiers).To(HaveLen(2))
					Expect(current.Status.GuestOSIdentifiers).To(ContainElements(
						vimv1.VirtualMachineGuestOSIdentifier("otherLinux64Guest"),
						vimv1.VirtualMachineGuestOSIdentifier("windows9Server64Guest"),
					))
					Expect(current.Status.Description).To(Equal("Test config option"))
					Expect(current.Status.ObservedGeneration).To(Equal(current.Generation))
				})
			})
		})

		When("ReconcileDelete", func() {
			BeforeEach(func() {
				configOptions.Finalizers = []string{vmconfigoptions.Finalizer}
				initObjects = append(initObjects, configTarget, configOptions)
			})

			It("should remove the finalizer", func() {
				// Trigger deletion after context is ready; fake client sets DeletionTimestamp.
				Expect(ctx.Client.Delete(cource.WithContext(ctx), configOptions)).To(Succeed())

				_, err := doReconcile()
				Expect(err).ToNot(HaveOccurred())

				// After the finalizer is removed the fake client may garbage-collect
				// the object immediately, so accept both NotFound and an object with
				// no finalizer.
				current := &vimv1.VirtualMachineConfigOptions{}
				getErr := ctx.Client.Get(ctx, objKey, current)
				if getErr == nil {
					Expect(current.Finalizers).ToNot(ContainElement(vmconfigoptions.Finalizer))
				} else {
					Expect(getErr).To(MatchError(ContainSubstring("not found")))
				}
			})
		})
	})
}
