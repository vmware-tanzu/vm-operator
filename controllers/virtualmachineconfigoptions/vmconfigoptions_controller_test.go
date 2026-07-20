// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineconfigoptions_test

import (
	"context"
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmconfigoptions "github.com/vmware-tanzu/vm-operator/controllers/virtualmachineconfigoptions"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
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

	Describe(
		"Reconcile against vcsim",
		Label(
			testlabels.Controller,
			testlabels.API,
			testlabels.EnvTest,
			testlabels.VCSim,
		),
		vcsimTestsReconcile,
	)
}

func unitTestsReconcile() {
	const (
		objName         = "vmx-22-config"
		clusterMoID     = "domain-c1234"
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
						Kind:       vmconfigoptions.ConfigTargetKind,
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

			It("should requeue with a delay and mark Ready=False with ConfigTargetNotFound", func() {
				// First reconcile adds the finalizer and returns early.
				_, err := doReconcile()
				Expect(err).ToNot(HaveOccurred())
				// Second reconcile finds no ConfigTarget owner and requeues.
				result, err := doReconcile()
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				current := &vimv1.VirtualMachineConfigOptions{}
				Expect(ctx.Client.Get(ctx, objKey, current)).To(Succeed())
				Expect(pkgcond.IsFalse(current, vimv1.ReadyConditionType)).To(BeTrue())
				Expect(pkgcond.GetReason(current, vimv1.ReadyConditionType)).To(Equal(vmconfigoptions.ConfigTargetNotFoundReason))
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
					Expect(pkgcond.GetReason(current, vimv1.ReadyConditionType)).To(Equal(vmconfigoptions.QueryFailedReason))
				})
			})

			When("reconcile succeeds", func() {
				JustBeforeEach(func() {
					fakeVMProvider.QueryConfigOptionExFn = func(_ context.Context, _, _ string) (*vimtypes.VirtualMachineConfigOption, error) {
						return &vimtypes.VirtualMachineConfigOption{
							Description:                      "Test config option",
							GuestOSDefaultIndex:              1,
							SupportedMonitorType:             []string{"debug", "release"},
							SupportedOvfEnvironmentTransport: []string{"com.vmware.guestInfo"},
							SupportedOvfInstallTransport:     []string{"iso"},
							GuestOSDescriptor: []vimtypes.GuestOsDescriptor{
								{
									Id:                          "otherLinux64Guest",
									FullName:                    "Other Linux (64-bit)",
									Family:                      "linuxGuest",
									SupportedMaxCPUs:            64,
									SupportedMaxMemMB:           8192,
									SupportedMinMemMB:           512,
									RecommendedMemMB:            2048,
									NumSupportedPhysicalSockets: 4,
									NumSupportedCoresPerSocket:  8,
									RecommendedColorDepth:       24,
									SupportedNumDisks:           16,
									RecommendedDiskController:   "ParaVirtualSCSIController",
									RecommendedSCSIController:   "VirtualLsiLogicController",
									RecommendedCdromController:  "VirtualIDEController",
									RecommendedEthernetCard:     "VirtualVmxnet3",
									SupportsWakeOnLan:           true,
									SmcRequired:                 true,
									SmcRecommended:              true,
									UsbRecommended:              true,
									SupportsVMI:                 true,
									SupportedForCreate:          true,
									SupportLevel:                "deprecated",
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

				It("should populate status.guestOSDefaultIndex and the supported transport/monitor lists", func() {
					_, err := doReconcile()
					Expect(err).ToNot(HaveOccurred())
					_, err = doReconcile()
					Expect(err).ToNot(HaveOccurred())

					current := &vimv1.VirtualMachineConfigOptions{}
					Expect(ctx.Client.Get(ctx, objKey, current)).To(Succeed())
					Expect(current.Status.GuestOSDefaultIndex).To(Equal(int32(1)))
					Expect(current.Status.SupportedMonitorTypes).To(Equal([]string{"debug", "release"}))
					Expect(current.Status.SupportedOvfEnvironmentTransports).To(Equal([]string{"com.vmware.guestInfo"}))
					Expect(current.Status.SupportedOvfInstallTransports).To(Equal([]string{"iso"}))
				})

				It("should map SupportLevel and Family from their vSphere wire values", func() {
					_, err := doReconcile()
					Expect(err).ToNot(HaveOccurred())
					_, err = doReconcile()
					Expect(err).ToNot(HaveOccurred())

					guestOptions := &vimv1.VirtualMachineGuestOptions{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: "otherlinux64guest"}, guestOptions)).To(Succeed())
					Expect(guestOptions.Status.Family).To(Equal(vimv1.VirtualMachineGuestOSFamilyLinux))
					Expect(guestOptions.Status.HardwareVersions).To(HaveLen(1))
					Expect(guestOptions.Status.HardwareVersions[0].SupportLevel).To(Equal(vimv1.SupportLevelDeprecated))
				})

				It("should populate the remaining hardware-version status fields", func() {
					_, err := doReconcile()
					Expect(err).ToNot(HaveOccurred())
					_, err = doReconcile()
					Expect(err).ToNot(HaveOccurred())

					guestOptions := &vimv1.VirtualMachineGuestOptions{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: "otherlinux64guest"}, guestOptions)).To(Succeed())
					Expect(guestOptions.Status.HardwareVersions).To(HaveLen(1))
					hwStatus := guestOptions.Status.HardwareVersions[0]

					Expect(hwStatus.SupportedMaxCPUs).To(Equal(int32(64)))
					Expect(hwStatus.NumSupportedPhysicalSockets).To(Equal(int32(4)))
					Expect(hwStatus.NumSupportedCoresPerSocket).To(Equal(int32(8)))
					Expect(hwStatus.RecommendedColorDepth).To(Equal(int32(24)))
					Expect(hwStatus.SupportedNumDisks).To(Equal(int32(16)))
					Expect(hwStatus.RecommendedDiskController).To(Equal(vimv1.VirtualControllerType("ParaVirtualSCSIController")))
					Expect(hwStatus.RecommendedSCSIController).To(Equal(vimv1.VirtualControllerType("VirtualLsiLogicController")))
					Expect(hwStatus.RecommendedCdromController).To(Equal(vimv1.VirtualControllerType("VirtualIDEController")))
					Expect(hwStatus.RecommendedEthernetCard).To(Equal(vimv1.EthernetCardType("VirtualVmxnet3")))
					Expect(hwStatus.SupportsWakeOnLan).To(BeTrue())
					Expect(hwStatus.SmcRequired).To(BeTrue())
					Expect(hwStatus.SmcRecommended).To(BeTrue())
					Expect(hwStatus.UsbRecommended).To(BeTrue())
					Expect(hwStatus.SupportsVMI).To(BeTrue())
					Expect(hwStatus.SupportedForCreate).To(BeTrue())
				})
			})

			When("QueryConfigOptionEx reports no config option for the requested hardware version", func() {
				JustBeforeEach(func() {
					fakeVMProvider.QueryConfigOptionExFn = func(_ context.Context, _, _ string) (*vimtypes.VirtualMachineConfigOption, error) {
						return nil, nil
					}
				})

				It("should mark Ready=False with NotFound reason", func() {
					// First reconcile adds finalizer.
					_, err := doReconcile()
					Expect(err).ToNot(HaveOccurred())
					// Second reconcile queries the provider and finds nothing.
					_, err = doReconcile()
					Expect(err).ToNot(HaveOccurred())

					current := &vimv1.VirtualMachineConfigOptions{}
					Expect(ctx.Client.Get(ctx, objKey, current)).To(Succeed())
					Expect(pkgcond.IsFalse(current, vimv1.ReadyConditionType)).To(BeTrue())
					Expect(pkgcond.GetReason(current, vimv1.ReadyConditionType)).To(Equal(vmconfigoptions.NotFoundReason))
					Expect(current.Status.ObservedGeneration).To(Equal(current.Generation))
				})
			})

		})

		When("a ConfigTarget owner reference names a ConfigTarget that no longer exists", func() {
			BeforeEach(func() {
				configOptions.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: vimv1.GroupVersion.String(),
						Kind:       vmconfigoptions.ConfigTargetKind,
						Name:       "deleted-config-target",
						UID:        "deleted-uid",
					},
				}
				// Note: configTarget itself is intentionally not added to
				// initObjects, since the owner reference must point to a
				// ConfigTarget that no longer exists in the cluster.
				initObjects = append(initObjects, configOptions)
			})

			It("should mark Ready=False with QueryFailed reason and return an error", func() {
				// First reconcile adds finalizer.
				_, err := doReconcile()
				Expect(err).ToNot(HaveOccurred())
				// Second reconcile fails to resolve the owning ConfigTarget.
				_, err = doReconcile()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get ConfigTarget"))

				current := &vimv1.VirtualMachineConfigOptions{}
				Expect(ctx.Client.Get(ctx, objKey, current)).To(Succeed())
				Expect(pkgcond.IsFalse(current, vimv1.ReadyConditionType)).To(BeTrue())
				Expect(pkgcond.GetReason(current, vimv1.ReadyConditionType)).To(Equal(vmconfigoptions.QueryFailedReason))
			})
		})

		When("two VirtualMachineConfigOptions for different hardware versions report the same guest OS", func() {
			const otherHardwareVersion = "vmx-21"

			var otherConfigOptions *vimv1.VirtualMachineConfigOptions

			BeforeEach(func() {
				otherConfigOptions = &vimv1.VirtualMachineConfigOptions{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vmx-21-config",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: vimv1.GroupVersion.String(),
								Kind:       vmconfigoptions.ConfigTargetKind,
								Name:       configTarget.Name,
								UID:        configTarget.UID,
							},
						},
					},
					Spec: vimv1.VirtualMachineConfigOptionsSpec{
						HardwareVersion: otherHardwareVersion,
					},
				}
				initObjects = append(initObjects, configTarget, configOptions, otherConfigOptions)
			})

			JustBeforeEach(func() {
				fakeVMProvider.QueryConfigOptionExFn = func(_ context.Context, _, _ string) (*vimtypes.VirtualMachineConfigOption, error) {
					return &vimtypes.VirtualMachineConfigOption{
						GuestOSDescriptor: []vimtypes.GuestOsDescriptor{
							{
								Id:       "otherLinux64Guest",
								FullName: "Other Linux (64-bit)",
								Family:   "linuxGuest",
							},
						},
					}, nil
				}
			})

			It("should accumulate a distinct status.hardwareVersions entry per hardware version", func() {
				otherObjKey := types.NamespacedName{Name: otherConfigOptions.Name}
				doReconcileOther := func() (reconcile.Result, error) {
					return reconciler.Reconcile(cource.WithContext(ctx), reconcile.Request{NamespacedName: otherObjKey})
				}

				// First reconcile of each adds its finalizer; second performs
				// the full reconciliation and fan-out.
				_, err := doReconcile()
				Expect(err).ToNot(HaveOccurred())
				_, err = doReconcile()
				Expect(err).ToNot(HaveOccurred())
				_, err = doReconcileOther()
				Expect(err).ToNot(HaveOccurred())
				_, err = doReconcileOther()
				Expect(err).ToNot(HaveOccurred())

				guestOptions := &vimv1.VirtualMachineGuestOptions{}
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: "otherlinux64guest"}, guestOptions)).To(Succeed())
				Expect(guestOptions.Status.HardwareVersions).To(HaveLen(2))

				var hardwareVersions []string
				for _, hv := range guestOptions.Status.HardwareVersions {
					hardwareVersions = append(hardwareVersions, hv.HardwareVersion)
				}
				Expect(hardwareVersions).To(ContainElements(hardwareVersion, otherHardwareVersion))
			})
		})

		When("a guest OS identifier is not DNS-subdomain-safe", func() {
			const longGuestID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-tobetrimmed"

			BeforeEach(func() {
				initObjects = append(initObjects, configTarget, configOptions)
			})

			JustBeforeEach(func() {
				fakeVMProvider.QueryConfigOptionExFn = func(_ context.Context, _, _ string) (*vimtypes.VirtualMachineConfigOption, error) {
					return &vimtypes.VirtualMachineConfigOption{
						GuestOSDescriptor: []vimtypes.GuestOsDescriptor{
							{
								Id:       "Other_Linux!!64Guest",
								FullName: "Other Linux (64-bit), sanitized",
								Family:   "linuxGuest",
							},
							{
								Id:       longGuestID,
								FullName: "Guest with an over-length identifier",
								Family:   "linuxGuest",
							},
						},
					}, nil
				}
			})

			It("should replace non-alphanumeric-hyphen characters and truncate to 63 characters", func() {
				_, err := doReconcile()
				Expect(err).ToNot(HaveOccurred())
				_, err = doReconcile()
				Expect(err).ToNot(HaveOccurred())

				sanitized := &vimv1.VirtualMachineGuestOptions{}
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: "other-linux-64guest"}, sanitized)).To(Succeed())
				Expect(sanitized.Spec.ID).To(Equal(vimv1.VirtualMachineGuestOSIdentifier("Other_Linux!!64Guest")))

				Expect(len(longGuestID)).To(BeNumerically(">", 63))
				truncatedName := strings.Repeat("a", 62)
				Expect(len(truncatedName)).To(BeNumerically("<=", 63))
				truncated := &vimv1.VirtualMachineGuestOptions{}
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: truncatedName}, truncated)).To(Succeed())
				Expect(truncated.Spec.ID).To(Equal(vimv1.VirtualMachineGuestOSIdentifier(longGuestID)))
			})
		})
		When("VirtualMachineConfigOptions is co-owned by multiple ConfigTargets", func() {
			const (
				higherClusterMoID = "domain-c9999"
				lowerClusterMoID  = "domain-c0001"
			)

			var (
				configTargetHigher *vimv1.ConfigTarget
				configTargetLower  *vimv1.ConfigTarget
				queriedClusterMoID string
			)

			BeforeEach(func() {
				configTargetHigher = &vimv1.ConfigTarget{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-config-target-higher"},
					Spec:       vimv1.ConfigTargetSpec{ID: vimv1.ManagedObjectID{ID: higherClusterMoID}},
				}
				configTargetLower = &vimv1.ConfigTarget{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-config-target-lower"},
					Spec:       vimv1.ConfigTargetSpec{ID: vimv1.ManagedObjectID{ID: lowerClusterMoID}},
				}

				// Order the owner references so the lexicographically lowest
				// cluster MoID (lowerClusterMoID) is listed last, proving
				// selection isn't just "the first owner ref".
				configOptions.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: vimv1.GroupVersion.String(),
						Kind:       vmconfigoptions.ConfigTargetKind,
						Name:       configTarget.Name,
						UID:        configTarget.UID,
					},
					{
						APIVersion: vimv1.GroupVersion.String(),
						Kind:       vmconfigoptions.ConfigTargetKind,
						Name:       configTargetHigher.Name,
						UID:        configTargetHigher.UID,
					},
					{
						APIVersion: vimv1.GroupVersion.String(),
						Kind:       vmconfigoptions.ConfigTargetKind,
						Name:       configTargetLower.Name,
						UID:        configTargetLower.UID,
					},
				}

				initObjects = append(initObjects, configTarget, configOptions, configTargetHigher, configTargetLower)

				queriedClusterMoID = ""
			})

			JustBeforeEach(func() {
				fakeVMProvider.QueryConfigOptionExFn = func(_ context.Context, clusterMoID, _ string) (*vimtypes.VirtualMachineConfigOption, error) {
					queriedClusterMoID = clusterMoID
					return &vimtypes.VirtualMachineConfigOption{}, nil
				}
			})

			It("deterministically queries the lexicographically lowest cluster MoID", func() {
				// First reconcile adds finalizer.
				_, err := doReconcile()
				Expect(err).ToNot(HaveOccurred())
				// Second reconcile queries the vSphere provider.
				_, err = doReconcile()
				Expect(err).ToNot(HaveOccurred())

				Expect(queriedClusterMoID).To(Equal(lowerClusterMoID))
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

// vcsimTestsReconcile exercises the reconciler against a real vcsim-backed
// vSphere provider so the QueryConfigOptionEx wrapper's govmomi call path
// (PropertyCollector RetrieveOne for the EnvironmentBrowser MoRef, then the
// SOAP QueryConfigOptionEx RPC) is exercised end-to-end, not just mocked away
// via the fake provider used by unitTestsReconcile above.
func vcsimTestsReconcile() {
	var (
		vcsimCtx      *builder.TestContextForVCSim
		reconciler    *vmconfigoptions.Reconciler
		configTarget  *vimv1.ConfigTarget
		configOptions *vimv1.VirtualMachineConfigOptions
		objReq        ctrl.Request
	)

	BeforeEach(func() {
		vcsimCtx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		provider := vsphere.NewVSphereVMProviderFromClient(vcsimCtx, vcsimCtx.Client, vcsimCtx.Recorder)

		reconciler = vmconfigoptions.NewReconciler(
			vcsimCtx,
			vcsimCtx.Client,
			ctrl.Log.WithName("virtualmachineconfigoptions"),
			vcsimCtx.Recorder,
			provider)

		ccr := vcsimCtx.GetFirstClusterFromFirstZone()
		Expect(ccr).ToNot(BeNil())

		configTarget = &vimv1.ConfigTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name: ccr.Reference().Value,
			},
			Spec: vimv1.ConfigTargetSpec{
				ID: vimv1.ManagedObjectID{ID: ccr.Reference().Value},
			},
		}
		Expect(vcsimCtx.Client.Create(vcsimCtx, configTarget)).To(Succeed())

		configOptions = &vimv1.VirtualMachineConfigOptions{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vmx-19",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: vimv1.GroupVersion.String(),
						Kind:       vmconfigoptions.ConfigTargetKind,
						Name:       configTarget.Name,
						UID:        configTarget.UID,
					},
				},
			},
			Spec: vimv1.VirtualMachineConfigOptionsSpec{
				HardwareVersion: "vmx-19",
			},
		}
		Expect(vcsimCtx.Client.Create(vcsimCtx, configOptions)).To(Succeed())

		objReq = ctrl.Request{
			NamespacedName: types.NamespacedName{Name: configOptions.Name},
		}
	})

	AfterEach(func() {
		vcsimCtx.AfterEach()
	})

	When("the owning ConfigTarget names a real vcsim cluster", func() {
		It("populates status from the real EnvironmentBrowser result and marks Ready=True", func() {
			// First reconcile adds the finalizer.
			_, err := reconciler.Reconcile(vcsimCtx, objReq)
			Expect(err).ToNot(HaveOccurred())
			// Second reconcile queries the real EnvironmentBrowser via vcsim.
			_, err = reconciler.Reconcile(vcsimCtx, objReq)
			Expect(err).ToNot(HaveOccurred())

			var current vimv1.VirtualMachineConfigOptions
			Expect(vcsimCtx.Client.Get(vcsimCtx, client.ObjectKeyFromObject(configOptions), &current)).To(Succeed())
			Expect(pkgcond.IsTrue(&current, vimv1.ReadyConditionType)).To(BeTrue())
			Expect(current.Status.ObservedGeneration).To(Equal(current.Generation))
			Expect(current.Status.GuestOSIdentifiers).ToNot(BeEmpty())

			var guestOptionsList vimv1.VirtualMachineGuestOptionsList
			Expect(vcsimCtx.Client.List(vcsimCtx, &guestOptionsList)).To(Succeed())
			Expect(guestOptionsList.Items).ToNot(BeEmpty())

			// Every guest OS identifier reported in status should have a
			// corresponding VirtualMachineGuestOptions with a matching
			// hardware-version status entry -- this is the actual fan-out,
			// not just "something got created".
			for _, guestID := range current.Status.GuestOSIdentifiers {
				var match *vimv1.VirtualMachineGuestOptions
				for i := range guestOptionsList.Items {
					if guestOptionsList.Items[i].Spec.ID == guestID {
						match = &guestOptionsList.Items[i]
						break
					}
				}
				Expect(match).ToNot(BeNil(), "expected a VirtualMachineGuestOptions for guest ID %q", guestID)
				Expect(match.Status.Family).ToNot(BeEmpty())

				var hwEntry *vimv1.VirtualMachineGuestOptionsHardwareVersionStatus
				for i := range match.Status.HardwareVersions {
					if match.Status.HardwareVersions[i].HardwareVersion == configOptions.Spec.HardwareVersion {
						hwEntry = &match.Status.HardwareVersions[i]
						break
					}
				}
				Expect(hwEntry).ToNot(BeNil(), "expected a %q hardwareVersions entry for guest ID %q",
					configOptions.Spec.HardwareVersion, guestID)
			}
		})
	})
}
