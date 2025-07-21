// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	dummyImage              = "dummy-image"
	dummyClass              = "dummy-class"
	storageClass            = "my-storage-class"
	finalizer               = "vmoperator.vmware.com/virtualmachinegroup"
	virtualMachineKind      = "VirtualMachine"
	virtualMachineGroupKind = "VirtualMachineGroup"
)

var _ = Describe(
	"Reconcile",
	Label(
		testlabels.Controller,
		testlabels.EnvTest,
		testlabels.API,
	),
	func() {
		var (
			ctx *builder.IntegrationTestContext

			vm1Key, vm2Key           types.NamespacedName
			vmGroup1Key, vmGroup2Key types.NamespacedName
		)

		BeforeEach(func() {
			ctx = suite.NewIntegrationTestContext()

			vmGroup1Key = types.NamespacedName{
				Name:      "vmgroup-1",
				Namespace: ctx.Namespace,
			}
			vmGroup2Key = types.NamespacedName{
				Name:      "vmgroup-2",
				Namespace: ctx.Namespace,
			}

			vm1Key = types.NamespacedName{
				Name:      "vm-1",
				Namespace: ctx.Namespace,
			}
			vm2Key = types.NamespacedName{
				Name:      "vm-2",
				Namespace: ctx.Namespace,
			}

			// Create VM and VM Group objects used in the tests.
			vmGroup1 := &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vmGroup1Key.Namespace,
					Name:      vmGroup1Key.Name,
				},
			}
			Expect(ctx.Client.Create(ctx, vmGroup1)).To(Succeed())

			vm1 := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm1Key.Namespace,
					Name:      vm1Key.Name,
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:    dummyImage,
					ClassName:    dummyClass,
					StorageClass: storageClass,
				},
			}
			Expect(ctx.Client.Create(ctx, vm1)).To(Succeed())

			vm2 := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm2Key.Namespace,
					Name:      vm2Key.Name,
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:    dummyImage,
					ClassName:    dummyClass,
					StorageClass: storageClass,
				},
			}
			Expect(ctx.Client.Create(ctx, vm2)).To(Succeed())

			vmGroup2 := &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vmGroup2Key.Namespace,
					Name:      vmGroup2Key.Name,
				},
			}
			Expect(ctx.Client.Create(ctx, vmGroup2)).To(Succeed())
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
			intgFakeVMProvider.Reset()
		})

		Context("Finalizer", func() {
			It("should add finalizer to all VirtualMachineGroup objects", func() {
				Eventually(func(g Gomega) {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					g.Expect(vmGroup1.GetFinalizers()).To(ContainElement(finalizer))

					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					g.Expect(vmGroup2.GetFinalizers()).To(ContainElement(finalizer))
					// Using a longer timeout to ensure all VM Groups above are reconciled.
				}, "5s", "100ms").Should(Succeed(), "waiting for VirtualMachineGroup finalizer")
			})
		})

		Context("Members", func() {
			BeforeEach(func() {
				vmGroup1 := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
				vmGroup1Copy := vmGroup1.DeepCopy()
				vmGroup1Copy.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{
								Kind: virtualMachineKind,
								Name: vm1Key.Name,
							},
							{
								Kind: virtualMachineGroupKind,
								Name: vmGroup2Key.Name,
							},
						},
					},
				}
				Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
			})

			When("members are not found", func() {
				BeforeEach(func() {
					vm1 := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					Expect(ctx.Client.Delete(ctx, vm1)).To(Succeed())

					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					Expect(ctx.Client.Delete(ctx, vmGroup2)).To(Succeed())
				})

				It("should mark the group not ready and the member condition as false", func() {
					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Conditions[0].Type).To(Equal(vmopv1.ReadyConditionType))
						g.Expect(vmGroup1.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Conditions[0].Reason).To(Equal("MembersNotReady"))

						g.Expect(vmGroup1.Status.Members).To(HaveLen(2))
						g.Expect(vmGroup1.Status.Members[0].Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Type).To(Equal(vmopv1.VirtualMachineGroupMemberConditionGroupLinked))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Reason).To(Equal("NotFound"))
						g.Expect(vmGroup1.Status.Members[1].Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Type).To(Equal(vmopv1.VirtualMachineGroupMemberConditionGroupLinked))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Reason).To(Equal("NotFound"))
					}, "5s", "100ms").Should(Succeed())
				})
			})

			When("members do not have the correct group name", func() {
				BeforeEach(func() {
					vm1 := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					vm1Copy := vm1.DeepCopy()
					vm1Copy.Spec.GroupName = "vmgroup-invalid"
					Expect(ctx.Client.Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())

					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					vmGroup2Copy := vmGroup2.DeepCopy()
					vmGroup2Copy.Spec.GroupName = "vmgroup-invalid"
					Expect(ctx.Client.Patch(ctx, vmGroup2Copy, client.MergeFrom(vmGroup2))).To(Succeed())
				})

				It("should mark the group not ready and the member condition as false", func() {
					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Conditions[0].Type).To(Equal(vmopv1.ReadyConditionType))
						g.Expect(vmGroup1.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Conditions[0].Reason).To(Equal("MembersNotReady"))

						g.Expect(vmGroup1.Status.Members).To(HaveLen(2))
						g.Expect(vmGroup1.Status.Members[0].Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Type).To(Equal(vmopv1.VirtualMachineGroupMemberConditionGroupLinked))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Reason).To(Equal("NotMember"))
						g.Expect(vmGroup1.Status.Members[1].Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Type).To(Equal(vmopv1.VirtualMachineGroupMemberConditionGroupLinked))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Reason).To(Equal("NotMember"))
					}, "5s", "100ms").Should(Succeed())
				})
			})

			When("members are found and have the correct group name", func() {
				BeforeEach(func() {
					vm1 := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					vm1Copy := vm1.DeepCopy()
					vm1Copy.Spec.GroupName = vmGroup1Key.Name
					Expect(ctx.Client.Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())

					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					vmGroup2Copy := vmGroup2.DeepCopy()
					vmGroup2Copy.Spec.GroupName = vmGroup1Key.Name
					Expect(ctx.Client.Patch(ctx, vmGroup2Copy, client.MergeFrom(vmGroup2))).To(Succeed())
				})

				It("should mark the group ready, member condition as true, and owner reference set to the group", func() {
					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Conditions[0].Type).To(Equal(vmopv1.ReadyConditionType))
						g.Expect(vmGroup1.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))

						g.Expect(vmGroup1.Status.Members).To(HaveLen(2))
						g.Expect(vmGroup1.Status.Members[0].Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Type).To(Equal(vmopv1.VirtualMachineGroupMemberConditionGroupLinked))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Status).To(Equal(metav1.ConditionTrue))
						g.Expect(vmGroup1.Status.Members[1].Conditions).To(HaveLen(2))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Type).To(Equal(vmopv1.ReadyConditionType))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Status).To(Equal(metav1.ConditionTrue))
						g.Expect(vmGroup1.Status.Members[1].Conditions[1].Type).To(Equal(vmopv1.VirtualMachineGroupMemberConditionGroupLinked))
						g.Expect(vmGroup1.Status.Members[1].Conditions[1].Status).To(Equal(metav1.ConditionTrue))

						vm1 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
						g.Expect(vm1.OwnerReferences).To(HaveLen(1))
						g.Expect(vm1.OwnerReferences[0].Kind).To(Equal(virtualMachineGroupKind))
						g.Expect(vm1.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))

						vmGroup2 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
						g.Expect(vmGroup2.OwnerReferences).To(HaveLen(1))
						g.Expect(vmGroup2.OwnerReferences[0].Kind).To(Equal(virtualMachineGroupKind))
						g.Expect(vmGroup2.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))
					}, "5s", "100ms").Should(Succeed())
				})
			})
		})

		Context("Placement", func() {
			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.PlaceVirtualMachineGroupFn = func(
					ctx context.Context,
					group *vmopv1.VirtualMachineGroup,
					groupPlacement []providers.VMGroupPlacement) error {

					for _, grpPlacement := range groupPlacement {
						for _, vm := range grpPlacement.VMMembers {
							found := false
							for i := range grpPlacement.VMGroup.Status.Members {
								ms := &grpPlacement.VMGroup.Status.Members[i]
								if ms.Name == vm.Name && ms.Kind == "VirtualMachine" {
									conditions.MarkTrue(ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
									found = true
									break
								}
							}

							if !found {
								ms := vmopv1.VirtualMachineGroupMemberStatus{
									Name: vm.Name,
									Kind: "VirtualMachine",
								}
								conditions.MarkTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
								grpPlacement.VMGroup.Status.Members = append(grpPlacement.VMGroup.Status.Members, ms)
							}
						}
					}

					return nil
				}
				intgFakeVMProvider.Unlock()

				By("setting up group-1 with members vm-1 and vmgroup-2")
				vmGroup1 := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
				vmGroup1Copy := vmGroup1.DeepCopy()
				vmGroup1Copy.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{
								Kind: virtualMachineKind,
								Name: vm1Key.Name,
							},
						},
					},
					{
						Members: []vmopv1.GroupMember{
							{
								Kind: virtualMachineGroupKind,
								Name: vmGroup2Key.Name,
							},
						},
					},
				}
				Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())

				By("setting up group-2 with members vm-2")
				vmGroup2 := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
				vmGroup2Copy := vmGroup2.DeepCopy()
				vmGroup2Copy.Spec.GroupName = vmGroup1Key.Name
				vmGroup2Copy.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{
								Kind: virtualMachineKind,
								Name: vm2Key.Name,
							},
						},
					},
				}
				Expect(ctx.Client.Patch(ctx, vmGroup2Copy, client.MergeFrom(vmGroup2))).To(Succeed())

				By("setting up group name for vm-1")
				vm1 := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
				vm1Copy := vm1.DeepCopy()
				vm1Copy.Spec.GroupName = vmGroup1Key.Name
				Expect(ctx.Client.Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())

				By("setting up group name for vm-2")
				vm2 := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
				vm2Copy := vm2.DeepCopy()
				vm2Copy.Spec.GroupName = vmGroup2Key.Name
				Expect(ctx.Client.Patch(ctx, vm2Copy, client.MergeFrom(vm2))).To(Succeed())
			})

			It("should update groups with placement ready condition", func() {
				Eventually(func(g Gomega) {
					By("all group members should have expected placement ready condition state")

					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					g.Expect(vmGroup2.Status.Members).To(HaveLen(1))
					vm2MS := vmGroup2.Status.Members[0]
					g.Expect(vm2MS.Name).To(Equal("vm-2"))
					g.Expect(vm2MS.Kind).To(Equal("VirtualMachine"))
					g.Expect(conditions.IsTrue(&vm2MS, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)).To(BeTrue())

					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					found := false
					for _, ms := range vmGroup1.Status.Members {
						if ms.Name == "vm-1" && ms.Kind == "VirtualMachine" {
							g.Expect(conditions.IsTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)).To(BeTrue())
							found = true
							break
						}
					}
					g.Expect(found).To(BeTrue(), "vm-1 in vmgroup-1 should have placement ready condition")

				}, "5s", "100ms").Should(Succeed())
			})
		})

		Context("PowerState", func() {
			BeforeEach(func() {
				By("setting up group-1 with members vm-1 and vmgroup-2")
				vmGroup1 := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
				vmGroup1Copy := vmGroup1.DeepCopy()
				vmGroup1Copy.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{
								Kind: virtualMachineKind,
								Name: vm1Key.Name,
							},
						},
					},
					{
						Members: []vmopv1.GroupMember{
							{
								Kind: virtualMachineGroupKind,
								Name: vmGroup2Key.Name,
							},
						},
					},
				}
				Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())

				By("setting up group-2 with members vm-2")
				vmGroup2 := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
				vmGroup2Copy := vmGroup2.DeepCopy()
				vmGroup2Copy.Spec.GroupName = vmGroup1Key.Name
				vmGroup2Copy.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{
								Kind: virtualMachineKind,
								Name: vm2Key.Name,
							},
						},
					},
				}
				Expect(ctx.Client.Patch(ctx, vmGroup2Copy, client.MergeFrom(vmGroup2))).To(Succeed())

				By("setting up group name for vm-1")
				vm1 := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
				vm1Copy := vm1.DeepCopy()
				vm1Copy.Spec.GroupName = vmGroup1Key.Name
				Expect(ctx.Client.Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())

				By("setting up group name for vm-2")
				vm2 := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
				vm2Copy := vm2.DeepCopy()
				vm2Copy.Spec.GroupName = vmGroup2Key.Name
				Expect(ctx.Client.Patch(ctx, vm2Copy, client.MergeFrom(vm2))).To(Succeed())
			})

			When("power off the group with try soft power off mode", func() {
				BeforeEach(func() {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					vmGroup1Copy := vmGroup1.DeepCopy()
					vmGroup1Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					vmGroup1Copy.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeTrySoft
					Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
				})

				It("should power off all members with try soft power off mode", func() {
					Eventually(func(g Gomega) {
						By("all group members should have expected power state and power off mode")
						vm1 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
						g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						g.Expect(vm1.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeTrySoft))

						vmGroup2 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
						g.Expect(vmGroup2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						g.Expect(vmGroup2.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeTrySoft))

						vm2 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
						g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						g.Expect(vm2.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeTrySoft))
					}, "5s", "100ms").Should(Succeed())
				})
			})

			When("power on the group with delay", func() {
				var (
					bootOrder1Delay  = 1 * time.Minute
					bootOrder2Delay  = 2 * time.Minute
					updatedPowerTime = time.Now()
				)

				BeforeEach(func() {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					vmGroup1Copy := vmGroup1.DeepCopy()
					vmGroup1Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					vmGroup1Copy.Spec.BootOrder[0].PowerOnDelay = &metav1.Duration{Duration: bootOrder1Delay}
					vmGroup1Copy.Spec.BootOrder[1].PowerOnDelay = &metav1.Duration{Duration: bootOrder2Delay}
					// Mimic mutating webhook to set last updated power state time annotation.
					// This is used to verify the power on delay is applied correctly.
					vmGroup1Copy.Annotations = map[string]string{
						constants.LastUpdatedPowerStateTimeAnnotation: updatedPowerTime.Format(time.RFC3339),
					}
					Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
				})

				It("should power on all members with the expected delay", func() {
					Eventually(func(g Gomega) {
						vm1 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
						g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
						g.Expect(vm1.Annotations).To(HaveKey(constants.ApplyPowerStateTimeAnnotation))
						vm1ApplyPowerStateTime, err := time.Parse(time.RFC3339, vm1.Annotations[constants.ApplyPowerStateTimeAnnotation])
						g.Expect(err).To(Not(HaveOccurred()))
						g.Expect(vm1ApplyPowerStateTime).To(BeTemporally("~", updatedPowerTime.Add(bootOrder1Delay), 5*time.Second))

						vmGroup2 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
						g.Expect(vmGroup2.Annotations).To(HaveKey(constants.ApplyPowerStateTimeAnnotation))
						vmGroup2ApplyPowerStateTime, err := time.Parse(time.RFC3339, vmGroup2.Annotations[constants.ApplyPowerStateTimeAnnotation])
						g.Expect(err).To(Not(HaveOccurred()))
						// Boot order delay is cumulative.
						bootOrder2Delay := bootOrder1Delay + bootOrder2Delay
						g.Expect(vmGroup2ApplyPowerStateTime).To(BeTemporally("~", updatedPowerTime.Add(bootOrder2Delay), 5*time.Second))

						vm2 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
						g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
						g.Expect(vm2.Annotations).To(HaveKey(constants.ApplyPowerStateTimeAnnotation))
						vm2ApplyPowerStateTime, err := time.Parse(time.RFC3339, vm2.Annotations[constants.ApplyPowerStateTimeAnnotation])
						g.Expect(err).To(Not(HaveOccurred()))
						g.Expect(vm2ApplyPowerStateTime).To(BeTemporally("~", updatedPowerTime.Add(bootOrder2Delay), 5*time.Second))
					}, "5s", "100ms").Should(Succeed())
				})
			})
		})

		Context("Deletion", func() {
			BeforeEach(func() {
				// Use Eventually to retry the delete operation in case of conflicts
				Eventually(func(g Gomega) {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					g.Expect(ctx.Client.Delete(ctx, vmGroup1)).To(Succeed())
				}).Should(Succeed(), "deleting vmGroup1 should succeed after retries")
			})

			// Please note it is not possible to validate garbage collection with
			// the fake client or with envtest, because neither of them implement
			// the Kubernetes garbage collector.
			It("should remove finalizer and delete the object", func() {
				Eventually(func(g Gomega) {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					err := ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})
