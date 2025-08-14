// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	imgregv1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha2"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
	pkgcnd "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = XDescribe("AddToManagerV1A2",
	Label(
		testlabels.Controller,
		testlabels.EnvTest,
		testlabels.API,
	),
	func() {
		var (
			parentCtx context.Context
			vcSimCtx  *builder.IntegrationTestContextForVCSim
		)

		BeforeEach(func() {
			parentCtx = pkgcfg.NewContextWithDefaultConfig()
		})

		JustBeforeEach(func() {
			vcSimCtx = builder.NewIntegrationTestContextForVCSim(
				parentCtx,
				builder.VCSimTestConfig{},
				func(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
					return utils.AddToManagerV1A2(ctx, mgr, &imgregv1.ContentLibraryItem{})
				},
				func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
					return nil
				},
				nil)
		})

		It("should not return an error", func() {
			Expect(vcSimCtx).ToNot(BeNil())
			vcSimCtx.BeforeEach()
		})

		When("FSS FastDeploy is enabled", func() {
			BeforeEach(func() {
				pkgcfg.UpdateContext(parentCtx, func(config *pkgcfg.Config) {
					config.Features.FastDeploy = true
				})
			})
			It("should not return an error", func() {
				Expect(vcSimCtx).ToNot(BeNil())
				vcSimCtx.BeforeEach()
			})
		})
	})

var _ = XDescribe("Reconcile",
	Label(
		testlabels.Controller,
		testlabels.API,
	),
	func() {
		const firmwareValue = "my-firmware"

		var (
			ctx *builder.UnitTestContextForController

			reconciler     *utils.ReconcilerV1A2
			fakeVMProvider *providerfake.VMProvider

			cliObj    client.Object
			cliSpec   *imgregv1.ContentLibraryItemSpec
			cliStatus *imgregv1.ContentLibraryItemStatus
			req       ctrl.Request

			vmiName  string
			vmicName string

			finalizer string
		)

		BeforeEach(func() {
			ctx = builder.NewUnitTestContextForController(nil)
			fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
		})

		JustBeforeEach(func() {
			var err error
			vmiName, err = utils.GetImageFieldNameFromItem(cliObj.GetName())
			Expect(err).ToNot(HaveOccurred())

			Expect(ctx.Client.Create(ctx, cliObj)).To(Succeed())
			cliObj, cliSpec, cliStatus = getV1A2CLI(ctx, req.Namespace, req.Name)
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
			cliObj = nil
			reconciler = nil
			fakeVMProvider.Reset()
		})

		Context("Namespace-scoped", func() {

			BeforeEach(func() {
				reconciler = utils.NewReconcilerV1A2(
					ctx,
					ctx.Client,
					ctx.Logger,
					ctx.Recorder,
					ctx.VMProvider,
					"ContentLibraryItem",
				)

				fakeVMProvider.SyncVirtualMachineImageFn = func(ctx context.Context, _, vmiObj client.Object) error {
					// Verify ovfcache is in context and does not panic.
					_, _ = ovfcache.GetOVFEnvelope(ctx, "", "")

					vmi := vmiObj.(*vmopv1.VirtualMachineImage)

					// Use Firmware field to verify the provider function is called.
					vmi.Status.Firmware = firmwareValue
					return nil
				}

				o := utils.DummyV1A2ContentLibraryItem(
					utils.ItemFieldNamePrefix+"-dummy", "dummy-ns")
				cliObj, cliSpec, cliStatus = o, &o.Spec, &o.Status
				finalizer, _ = utils.GetAppropriateFinalizers(cliObj)
				vmicName = pkgutil.VMIName(cliSpec.ID)

				// Add the finalizer so Reconcile does not return early.
				cliObj.SetFinalizers([]string{finalizer})

				req = ctrl.Request{}
				req.Namespace = cliObj.GetNamespace()
				req.Name = cliObj.GetName()
			})

			Context("ReconcileNormal", func() {

				When("Library item resource doesn't have the VMOP finalizer", func() {
					BeforeEach(func() {
						cliObj.SetFinalizers(nil)
					})

					It("should add the finalizer", func() {
						_, err := reconciler.Reconcile(context.Background(), req)
						Expect(err).ToNot(HaveOccurred())
						cliObj, _, _ = getV1A2CLI(ctx, req.Namespace, req.Name)

						Expect(cliObj.GetFinalizers()).To(ContainElement(finalizer))
					})
				})

				When("Library item resource is Not Ready", func() {
					BeforeEach(func() {
						cliStatus.Conditions = []metav1.Condition{
							{
								Type:   imgregv1.ReadyCondition,
								Status: metav1.ConditionFalse,
							},
						}
					})

					It("should mark image resource condition as provider not ready", func() {
						_, err := reconciler.Reconcile(context.Background(), req)
						Expect(err).ToNot(HaveOccurred())

						_, _, vmiStatus := getVMI(ctx, req.Namespace, vmiName)
						condition := pkgcnd.Get(vmiStatus, vmopv1.ReadyConditionType)
						Expect(condition).ToNot(BeNil())
						Expect(condition.Status).To(Equal(metav1.ConditionFalse))
						Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageProviderNotReadyReason))
					})
				})

				When("Library item has TKG labels", func() {
					var cliLabels map[string]string

					BeforeEach(func() {
						cliLabels = cliObj.GetLabels()
						if cliLabels == nil {
							cliLabels = map[string]string{}
						}
					})

					When("Multiple Content Library feature is disabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.TKGMultipleCL = false
							})

							cliLabels[utils.TKGServiceTypeLabelKeyPrefix+"1"] = ""
							cliLabels[utils.TKGServiceTypeLabelKeyPrefix+"2"] = ""
							cliLabels[utils.MultipleCLServiceTypeLabelKeyPrefix+"1"] = ""
							cliLabels[utils.MultipleCLServiceTypeLabelKeyPrefix+"2"] = ""

							cliObj.SetLabels(cliLabels)
						})

						It("should not copy the feature or non-feature labels to the vmi", func() {
							_, err := reconciler.Reconcile(context.Background(), req)
							Expect(err).ToNot(HaveOccurred())

							vmiObj, _, _ := getVMI(ctx, req.Namespace, vmiName)
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.TKGServiceTypeLabelKeyPrefix + "1"))
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.TKGServiceTypeLabelKeyPrefix + "2"))
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.MultipleCLServiceTypeLabelKeyPrefix + "1"))
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.MultipleCLServiceTypeLabelKeyPrefix + "2"))
						})
					})
					When("Multiple Content Library feature is enabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.TKGMultipleCL = true
							})

							cliLabels[utils.TKGServiceTypeLabelKeyPrefix+"1"] = ""
							cliLabels[utils.TKGServiceTypeLabelKeyPrefix+"2"] = ""
							cliLabels[utils.MultipleCLServiceTypeLabelKeyPrefix+"1"] = ""
							cliLabels[utils.MultipleCLServiceTypeLabelKeyPrefix+"2"] = ""

							cliObj.SetLabels(cliLabels)
						})

						JustBeforeEach(func() {
							vmiObj := newVMI(
								ctx,
								req.Namespace,
								vmiName,
								vmopv1.VirtualMachineImageStatus{
									ProviderContentVersion: "stale",
									Firmware:               "should-be-updated",
								})
							vmiLabels := vmiObj.GetLabels()
							if vmiLabels == nil {
								vmiLabels = map[string]string{}
							}
							vmiLabels[utils.TKGServiceTypeLabelKeyPrefix+"3"] = ""
							vmiLabels[utils.MultipleCLServiceTypeLabelKeyPrefix+"3"] = ""
							vmiObj.SetLabels(vmiLabels)
							Expect(ctx.Client.Update(ctx, vmiObj)).To(Succeed())
						})

						It("should not copy the feature or non-feature labels to the vmi", func() {
							_, err := reconciler.Reconcile(context.Background(), req)
							Expect(err).ToNot(HaveOccurred())

							vmiObj, _, _ := getVMI(ctx, req.Namespace, vmiName)
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.TKGServiceTypeLabelKeyPrefix + "1"))
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.TKGServiceTypeLabelKeyPrefix + "2"))
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.MultipleCLServiceTypeLabelKeyPrefix + "1"))
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.MultipleCLServiceTypeLabelKeyPrefix + "2"))
						})
					})
				})

				When("Library item resource is not security compliant", func() {

					BeforeEach(func() {
						cliStatus.SecurityCompliance = ptr.To(false)
					})

					It("should mark image resource condition as provider security not compliant", func() {
						_, err := reconciler.Reconcile(context.Background(), req)
						Expect(err).ToNot(HaveOccurred())

						_, _, vmiStatus := getVMI(ctx, req.Namespace, vmiName)
						condition := pkgcnd.Get(vmiStatus, vmopv1.ReadyConditionType)
						Expect(condition).ToNot(BeNil())
						Expect(condition.Status).To(Equal(metav1.ConditionFalse))
						Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageProviderSecurityNotCompliantReason))
					})
				})

				When("SyncVirtualMachineImage returns an error", func() {

					BeforeEach(func() {
						fakeVMProvider.SyncVirtualMachineImageFn = func(ctx context.Context, _, _ client.Object) error {
							// Verify ovfcache is in context and does not panic.
							_, _ = ovfcache.GetOVFEnvelope(ctx, "", "")

							return fmt.Errorf("sync-error")
						}
					})

					It("should mark image resource condition synced failed", func() {
						_, err := reconciler.Reconcile(context.Background(), req)
						Expect(err).To(MatchError("sync-error"))

						_, _, vmiStatus := getVMI(ctx, req.Namespace, vmiName)
						condition := pkgcnd.Get(vmiStatus, vmopv1.ReadyConditionType)
						Expect(condition).ToNot(BeNil())
						Expect(condition.Status).To(Equal(metav1.ConditionFalse))
						Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageNotSyncedReason))
					})

					When("error is ErrVMICacheNotReady", func() {
						JustBeforeEach(func() {
							fakeVMProvider.SyncVirtualMachineImageFn = func(ctx context.Context, _, _ client.Object) error {
								// Verify ovfcache is in context and does not panic.
								_, _ = ovfcache.GetOVFEnvelope(ctx, "", "")

								return fmt.Errorf("failed with %w",
									pkgerr.VMICacheNotReadyError{Name: vmicName})
							}
						})
						It("should place a label on the library item resource", func() {
							_, err := reconciler.Reconcile(context.Background(), req)

							var e pkgerr.VMICacheNotReadyError
							ExpectWithOffset(1, errors.As(err, &e)).To(BeTrue())
							ExpectWithOffset(1, e.Name).To(Equal(vmicName))

							cliObj, _, _ = getV1A2CLI(ctx, req.Namespace, req.Name)
							ExpectWithOffset(1, cliObj.GetLabels()).To(HaveKeyWithValue(
								pkgconst.VMICacheLabelKey, vmicName))

							_, _, vmiStatus := getVMI(ctx, req.Namespace, vmiName)
							condition := pkgcnd.Get(vmiStatus, vmopv1.ReadyConditionType)
							Expect(condition).To(BeNil())
						})
					})
				})

				When("Library item resource is ready and security complaint", func() {

					JustBeforeEach(func() {
						// The dummy library item should meet these requirements.
						var readyCond *metav1.Condition
						for _, c := range cliStatus.Conditions {
							if c.Type == imgregv1.ReadyCondition {
								c := c
								readyCond = &c
								break
							}
						}
						Expect(readyCond).ToNot(BeNil())
						Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))

						Expect(cliStatus.SecurityCompliance).To(Equal(ptr.To(true)))
					})

					When("Image resource has not been created yet", func() {

						It("should create a new image resource syncing up with the library item resource", func() {
							_, err := reconciler.Reconcile(context.Background(), req)
							Expect(err).ToNot(HaveOccurred())
							cliObj, cliSpec, cliStatus = getV1A2CLI(ctx, req.Namespace, req.Name)

							vmiObj, vmiSpec, vmiStatus := getVMI(ctx, req.Namespace, vmiName)
							assertVMImageFromV1A2CLItem(cliObj, *cliSpec, *cliStatus, vmiObj, *vmiSpec, *vmiStatus)
							Expect(vmiStatus.Firmware).To(Equal(firmwareValue))
						})
					})

					When("Image resource is exists but not up-to-date", func() {

						JustBeforeEach(func() {
							newVMI(
								ctx,
								req.Namespace,
								vmiName,
								vmopv1.VirtualMachineImageStatus{
									ProviderContentVersion: "stale",
									Firmware:               "should-be-updated",
								})
						})

						It("should update the existing image resource with the library item resource", func() {
							cliStatus.ContentVersion += UpdatedSuffix
							_, err := reconciler.Reconcile(context.Background(), req)
							Expect(err).ToNot(HaveOccurred())
							cliObj, cliSpec, cliStatus = getV1A2CLI(ctx, req.Namespace, req.Name)

							vmiObj, vmiSpec, vmiStatus := getVMI(ctx, req.Namespace, vmiName)
							assertVMImageFromV1A2CLItem(cliObj, *cliSpec, *cliStatus, vmiObj, *vmiSpec, *vmiStatus)
							Expect(vmiStatus.Firmware).To(Equal(firmwareValue))
						})
					})

					When("Image resource is created and already up-to-date", func() {

						JustBeforeEach(func() {
							newVMI(
								ctx,
								req.Namespace,
								vmiName,
								vmopv1.VirtualMachineImageStatus{
									ProviderContentVersion: cliStatus.ContentVersion,
									Firmware:               "should-be-updated",
								})
						})

						It("should still update the image resource status from the library item resource", func() {
							_, err := reconciler.Reconcile(context.Background(), req)
							Expect(err).ToNot(HaveOccurred())
							cliObj, cliSpec, cliStatus = getV1A2CLI(ctx, req.Namespace, req.Name)

							vmiObj, vmiSpec, vmiStatus := getVMI(ctx, req.Namespace, vmiName)
							assertVMImageFromV1A2CLItem(cliObj, *cliSpec, *cliStatus, vmiObj, *vmiSpec, *vmiStatus)
							Expect(vmiStatus.Firmware).To(Equal(firmwareValue))
						})
					})
				})

				When("Image resource is created and already up-to-date and Status.Disk is not empty", func() {

					JustBeforeEach(func() {
						newVMI(
							ctx,
							req.Namespace,
							vmiName,
							vmopv1.VirtualMachineImageStatus{
								ProviderContentVersion: cliStatus.ContentVersion,
								Disks:                  make([]vmopv1.VirtualMachineImageDiskInfo, 1),
								Firmware:               "should-not-be-updated",
							})
					})

					BeforeEach(func() {
						fakeVMProvider.SyncVirtualMachineImageFn = func(_ context.Context, _, vmiObj client.Object) error {
							// This should not be called since the content versions match and disks isn't empty.
							vmi := vmiObj.(*vmopv1.VirtualMachineImage)
							vmi.Status.Firmware = firmwareValue
							return fmt.Errorf("sync-error")
						}
					})

					It("should skip updating the ClusterVirtualMachineImage with library item", func() {
						_, err := reconciler.Reconcile(context.Background(), req)
						Expect(err).ToNot(HaveOccurred())
						cliObj, cliSpec, cliStatus = getV1A2CLI(ctx, req.Namespace, req.Name)

						vmiObj, vmiSpec, vmiStatus := getVMI(ctx, req.Namespace, vmiName)
						assertVMImageFromV1A2CLItem(cliObj, *cliSpec, *cliStatus, vmiObj, *vmiSpec, *vmiStatus)
						Expect(vmiStatus.Firmware).To(Equal("should-not-be-updated"))
					})
				})
			})

			Context("ReconcileDelete", func() {
				It("should remove the finalizer from the library item resource", func() {
					Expect(cliObj.GetFinalizers()).To(ContainElement(finalizer))
					cliObj.SetDeletionTimestamp(ptr.To(metav1.Now()))
					Expect(reconciler.ReconcileDelete(ctx, logr.Discard(), cliObj, vmiName)).To(Succeed())
					Expect(cliObj.GetFinalizers()).ToNot(ContainElement(finalizer))
				})
			})

		})

		Context("Cluster-scoped", func() {

			BeforeEach(func() {
				reconciler = utils.NewReconcilerV1A2(
					ctx,
					ctx.Client,
					ctx.Logger,
					ctx.Recorder,
					ctx.VMProvider,
					"ClusterContentLibraryItem",
				)

				fakeVMProvider.SyncVirtualMachineImageFn = func(ctx context.Context, _, vmiObj client.Object) error {
					// Verify ovfcache is in context and does not panic.
					_, _ = ovfcache.GetOVFEnvelope(ctx, "", "")

					vmi := vmiObj.(*vmopv1.ClusterVirtualMachineImage)

					// Use Firmware field to verify the provider function is called.
					vmi.Status.Firmware = firmwareValue
					return nil
				}

				o := utils.DummyV1A2ClusterContentLibraryItem(
					utils.ItemFieldNamePrefix + "-dummy")
				cliObj, cliSpec, cliStatus = o, &o.Spec, &o.Status
				finalizer, _ = utils.GetAppropriateFinalizers(cliObj)

				// Add the finalizer so Reconcile does not return early.
				cliObj.SetFinalizers([]string{finalizer})

				req = ctrl.Request{}
				req.Namespace = cliObj.GetNamespace()
				req.Name = cliObj.GetName()
			})

			Context("ReconcileNormal", func() {

				When("Library item has TKG labels", func() {
					var cliLabels map[string]string

					BeforeEach(func() {
						cliLabels = cliObj.GetLabels()
						if cliLabels == nil {
							cliLabels = map[string]string{}
						}
					})

					When("Multiple Content Library feature is disabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.TKGMultipleCL = false
							})

							cliLabels[utils.TKGServiceTypeLabelKeyPrefix+"1"] = ""
							cliLabels[utils.TKGServiceTypeLabelKeyPrefix+"2"] = ""
							cliLabels[utils.MultipleCLServiceTypeLabelKeyPrefix+"1"] = ""
							cliLabels[utils.MultipleCLServiceTypeLabelKeyPrefix+"2"] = ""

							cliObj.SetLabels(cliLabels)
						})

						JustBeforeEach(func() {
							vmiObj := newVMI(
								ctx,
								req.Namespace,
								vmiName,
								vmopv1.VirtualMachineImageStatus{
									ProviderContentVersion: "stale",
									Firmware:               "should-be-updated",
								})
							vmiLabels := vmiObj.GetLabels()
							if vmiLabels == nil {
								vmiLabels = map[string]string{}
							}
							vmiLabels[utils.TKGServiceTypeLabelKeyPrefix+"3"] = ""
							vmiLabels[utils.MultipleCLServiceTypeLabelKeyPrefix+"3"] = ""
							vmiObj.SetLabels(vmiLabels)
							Expect(ctx.Client.Update(ctx, vmiObj)).To(Succeed())
						})

						It("should copy the non-feature labels to the vmi", func() {
							_, err := reconciler.Reconcile(context.Background(), req)
							Expect(err).ToNot(HaveOccurred())

							vmiObj, _, _ := getVMI(ctx, req.Namespace, vmiName)
							Expect(vmiObj.GetLabels()).To(HaveKey(utils.TKGServiceTypeLabelKeyPrefix + "1"))
							Expect(vmiObj.GetLabels()).To(HaveKey(utils.TKGServiceTypeLabelKeyPrefix + "2"))
							Expect(vmiObj.GetLabels()).To(HaveKey(utils.TKGServiceTypeLabelKeyPrefix + "3"))
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.MultipleCLServiceTypeLabelKeyPrefix + "1"))
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.MultipleCLServiceTypeLabelKeyPrefix + "2"))
							Expect(vmiObj.GetLabels()).To(HaveKey(utils.MultipleCLServiceTypeLabelKeyPrefix + "3"))
						})
					})
					When("Multiple Content Library feature is enabled", func() {
						BeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.TKGMultipleCL = true
							})

							cliLabels[utils.TKGServiceTypeLabelKeyPrefix+"1"] = ""
							cliLabels[utils.TKGServiceTypeLabelKeyPrefix+"2"] = ""
							cliLabels[utils.MultipleCLServiceTypeLabelKeyPrefix+"1"] = ""
							cliLabels[utils.MultipleCLServiceTypeLabelKeyPrefix+"2"] = ""

							cliObj.SetLabels(cliLabels)
						})

						JustBeforeEach(func() {
							vmiObj := newVMI(
								ctx,
								req.Namespace,
								vmiName,
								vmopv1.VirtualMachineImageStatus{
									ProviderContentVersion: "stale",
									Firmware:               "should-be-updated",
								})
							vmiLabels := vmiObj.GetLabels()
							if vmiLabels == nil {
								vmiLabels = map[string]string{}
							}
							vmiLabels[utils.TKGServiceTypeLabelKeyPrefix+"3"] = ""
							vmiLabels[utils.MultipleCLServiceTypeLabelKeyPrefix+"3"] = ""
							vmiObj.SetLabels(vmiLabels)
							Expect(ctx.Client.Update(ctx, vmiObj)).To(Succeed())
						})

						It("should copy the feature labels to the vmi", func() {
							_, err := reconciler.Reconcile(context.Background(), req)
							Expect(err).ToNot(HaveOccurred())

							vmiObj, _, _ := getVMI(ctx, req.Namespace, vmiName)
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.TKGServiceTypeLabelKeyPrefix + "1"))
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.TKGServiceTypeLabelKeyPrefix + "2"))
							Expect(vmiObj.GetLabels()).To(HaveKey(utils.TKGServiceTypeLabelKeyPrefix + "3"))
							Expect(vmiObj.GetLabels()).To(HaveKey(utils.MultipleCLServiceTypeLabelKeyPrefix + "1"))
							Expect(vmiObj.GetLabels()).To(HaveKey(utils.MultipleCLServiceTypeLabelKeyPrefix + "2"))
							Expect(vmiObj.GetLabels()).ToNot(HaveKey(utils.MultipleCLServiceTypeLabelKeyPrefix + "3"))
						})
					})
				})

				When("Library item resource is ready and security complaint", func() {
					JustBeforeEach(func() {
						// The dummy library item should meet these requirements.
						var readyCond *metav1.Condition
						for _, c := range cliStatus.Conditions {
							if c.Type == imgregv1.ReadyCondition {
								c := c
								readyCond = &c
								break
							}
						}
						Expect(readyCond).ToNot(BeNil())
						Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
						Expect(cliStatus.SecurityCompliance).To(Equal(ptr.To(true)))
					})

					When("Image resource has not been created yet", func() {

						It("should create a new image resource syncing up with the library item resource", func() {
							_, err := reconciler.Reconcile(context.Background(), req)
							Expect(err).ToNot(HaveOccurred())
							cliObj, cliSpec, cliStatus = getV1A2CLI(ctx, req.Namespace, req.Name)

							vmiObj, vmiSpec, vmiStatus := getVMI(ctx, req.Namespace, vmiName)
							assertVMImageFromV1A2CLItem(cliObj, *cliSpec, *cliStatus, vmiObj, *vmiSpec, *vmiStatus)
							Expect(vmiStatus.Firmware).To(Equal(firmwareValue))
						})
					})

					When("Image resource is exists but not up-to-date", func() {
						JustBeforeEach(func() {
							newVMI(
								ctx,
								req.Namespace,
								vmiName,
								vmopv1.VirtualMachineImageStatus{
									ProviderContentVersion: "stale",
									Firmware:               "should-be-updated",
								})
						})
						It("should update the existing image resource with the library item resource", func() {
							cliStatus.ContentVersion += UpdatedSuffix
							_, err := reconciler.Reconcile(context.Background(), req)
							Expect(err).ToNot(HaveOccurred())
							cliObj, cliSpec, cliStatus = getV1A2CLI(ctx, req.Namespace, req.Name)

							vmiObj, vmiSpec, vmiStatus := getVMI(ctx, req.Namespace, vmiName)
							assertVMImageFromV1A2CLItem(cliObj, *cliSpec, *cliStatus, vmiObj, *vmiSpec, *vmiStatus)
							Expect(vmiStatus.Firmware).To(Equal(firmwareValue))
						})
					})
				})
			})

			Context("ReconcileDelete", func() {
				It("should remove the finalizer from the library item resource", func() {
					Expect(cliObj.GetFinalizers()).To(ContainElement(finalizer))
					cliObj.SetDeletionTimestamp(ptr.To(metav1.Now()))
					Expect(reconciler.ReconcileDelete(ctx, logr.Discard(), cliObj, vmiName)).To(Succeed())
					Expect(cliObj.GetFinalizers()).ToNot(ContainElement(finalizer))
				})
			})
		})

	})

func getV1A2CLI(
	ctx *builder.UnitTestContextForController,
	namespace, name string) (client.Object, *imgregv1.ContentLibraryItemSpec, *imgregv1.ContentLibraryItemStatus) {

	var (
		obj    client.Object
		spec   *imgregv1.ContentLibraryItemSpec
		status *imgregv1.ContentLibraryItemStatus
		key    = client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}
	)

	if namespace != "" {
		var o imgregv1.ContentLibraryItem
		ExpectWithOffset(1, ctx.Client.Get(ctx, key, &o)).To(Succeed())
		obj, spec, status = &o, &o.Spec, &o.Status
	} else {
		var o imgregv1.ClusterContentLibraryItem
		ExpectWithOffset(1, ctx.Client.Get(ctx, key, &o)).To(Succeed())
		obj, spec, status = &o, &o.Spec, &o.Status
	}

	return obj, spec, status
}

func assertVMImageFromV1A2CLItem(
	cliObj client.Object,
	cliSpec imgregv1.ContentLibraryItemSpec,
	cliStatus imgregv1.ContentLibraryItemStatus,
	vmiObj client.Object,
	vmiSpec vmopv1.VirtualMachineImageSpec,
	vmiStatus vmopv1.VirtualMachineImageStatus) {

	Expect(metav1.IsControlledBy(vmiObj, cliObj)).To(BeTrue())

	By("Expected VMImage Spec", func() {
		Expect(vmiSpec.ProviderRef.Name).To(Equal(cliObj.GetName()))
		cliGVK := cliObj.GetObjectKind().GroupVersionKind()
		Expect(vmiSpec.ProviderRef.APIVersion).To(Equal(cliGVK.GroupVersion().String()))
		Expect(vmiSpec.ProviderRef.Kind).To(Equal(cliGVK.Kind))
	})

	By("Expected VMImage Status", func() {
		Expect(vmiStatus.Name).To(Equal(cliStatus.Name))
		Expect(vmiStatus.ProviderItemID).To(BeEquivalentTo(cliSpec.ID))
		Expect(vmiStatus.ProviderContentVersion).To(Equal(cliStatus.ContentVersion))
		Expect(vmiStatus.Type).To(BeEquivalentTo(cliStatus.Type))
		Expect(pkgcnd.IsTrue(vmiStatus, vmopv1.ReadyConditionType)).To(BeTrue())
	})
}
