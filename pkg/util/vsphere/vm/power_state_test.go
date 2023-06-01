// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm_test

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
	vmutilInternal "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm/internal"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func powerStateTests() {
	Context("ParsePowerOpMode", func() {
		It("should return empty string for unknown power op", func() {
			Expect(vmutil.ParsePowerOpMode("")).To(Equal(vmutil.PowerOpBehavior(0)))
			Expect(vmutil.ParsePowerOpMode("fake")).To(Equal(vmutil.PowerOpBehavior(0)))
		})
		It("should parse variants of hard", func() {
			Expect(vmutil.ParsePowerOpMode("hard")).To(Equal(vmutil.PowerOpBehaviorHard))
			Expect(vmutil.ParsePowerOpMode("Hard")).To(Equal(vmutil.PowerOpBehaviorHard))
			Expect(vmutil.ParsePowerOpMode("HARD")).To(Equal(vmutil.PowerOpBehaviorHard))
		})
		It("should parse variants of soft", func() {
			Expect(vmutil.ParsePowerOpMode("soft")).To(Equal(vmutil.PowerOpBehaviorSoft))
			Expect(vmutil.ParsePowerOpMode("Soft")).To(Equal(vmutil.PowerOpBehaviorSoft))
			Expect(vmutil.ParsePowerOpMode("SOFT")).To(Equal(vmutil.PowerOpBehaviorSoft))
		})
		It("should parse variants of trySoft", func() {
			Expect(vmutil.ParsePowerOpMode("trySoft")).To(Equal(vmutil.PowerOpBehaviorTrySoft))
			Expect(vmutil.ParsePowerOpMode("TrySoft")).To(Equal(vmutil.PowerOpBehaviorTrySoft))
			Expect(vmutil.ParsePowerOpMode("Trysoft")).To(Equal(vmutil.PowerOpBehaviorTrySoft))
			Expect(vmutil.ParsePowerOpMode("trysoft")).To(Equal(vmutil.PowerOpBehaviorTrySoft))
			Expect(vmutil.ParsePowerOpMode("TRYSOFT")).To(Equal(vmutil.PowerOpBehaviorTrySoft))
		})
	})

	Context("ParsePowerState", func() {
		It("should return empty string for unknown power state", func() {
			Expect(vmutil.ParsePowerState("")).To(Equal(types.VirtualMachinePowerState("")))
			Expect(vmutil.ParsePowerState("fake")).To(Equal(types.VirtualMachinePowerState("")))
		})
		It("should parse variants of poweredOn", func() {
			Expect(vmutil.ParsePowerState("poweredOn")).To(Equal(types.VirtualMachinePowerStatePoweredOn))
			Expect(vmutil.ParsePowerState("PoweredOn")).To(Equal(types.VirtualMachinePowerStatePoweredOn))
			Expect(vmutil.ParsePowerState("Poweredon")).To(Equal(types.VirtualMachinePowerStatePoweredOn))
			Expect(vmutil.ParsePowerState("poweredon")).To(Equal(types.VirtualMachinePowerStatePoweredOn))
			Expect(vmutil.ParsePowerState("POWEREDON")).To(Equal(types.VirtualMachinePowerStatePoweredOn))
		})
		It("should parse variants of poweredOff", func() {
			Expect(vmutil.ParsePowerState("poweredOff")).To(Equal(types.VirtualMachinePowerStatePoweredOff))
			Expect(vmutil.ParsePowerState("PoweredOff")).To(Equal(types.VirtualMachinePowerStatePoweredOff))
			Expect(vmutil.ParsePowerState("Poweredoff")).To(Equal(types.VirtualMachinePowerStatePoweredOff))
			Expect(vmutil.ParsePowerState("poweredoff")).To(Equal(types.VirtualMachinePowerStatePoweredOff))
			Expect(vmutil.ParsePowerState("POWEREDOFF")).To(Equal(types.VirtualMachinePowerStatePoweredOff))
		})
		It("should parse variants of suspended", func() {
			Expect(vmutil.ParsePowerState("suspended")).To(Equal(types.VirtualMachinePowerStateSuspended))
			Expect(vmutil.ParsePowerState("Suspended")).To(Equal(types.VirtualMachinePowerStateSuspended))
			Expect(vmutil.ParsePowerState("SUSPENDED")).To(Equal(types.VirtualMachinePowerStateSuspended))
		})
	})

	Context("SetPowerState", func() {
		var (
			ctx                               *builder.TestContextForVCSim
			mgdObj                            mo.VirtualMachine
			obj                               *object.VirtualMachine
			expectedErr                       error
			expectedResult                    vmutil.SetPowerStateResult
			fetchCurrentPowerState            bool
			powerOpBehavior                   vmutil.PowerOpBehavior
			initialPowerState                 types.VirtualMachinePowerState
			desiredPowerState                 types.VirtualMachinePowerState
			skipPostSetPowerStateVerification bool
			mutateContextFn                   func(context.Context) context.Context
			delayTask                         = func(
				ctx context.Context,
				taskName string,
				taskDelay, softTimeout time.Duration) context.Context {

				simulator.TaskDelay.MethodDelay[taskName] = int(taskDelay.Milliseconds())
				return context.WithValue(ctx, vmutilInternal.SoftTimeoutKey, softTimeout)
			}
		)

		BeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
			mgdObj = mo.VirtualMachine{
				ManagedEntity: mo.ManagedEntity{
					ExtensibleManagedObject: mo.ExtensibleManagedObject{
						Self: types.ManagedObjectReference{
							Type:  "VirtualMachine",
							Value: "vm-44",
						},
					},
				},
			}
			simulator.TaskDelay.MethodDelay = map[string]int{}
			obj = object.NewVirtualMachine(ctx.VCClient.Client, mgdObj.Self)
			expectedResult = 0                                          // default
			expectedErr = nil                                           // default
			fetchCurrentPowerState = true                               // default
			initialPowerState = types.VirtualMachinePowerStatePoweredOn // default
			mutateContextFn = func(ctx context.Context) context.Context {
				return ctx
			}
		})

		JustBeforeEach(func() {
			var (
				err                error
				result             vmutil.SetPowerStateResult
				observedPowerState types.VirtualMachinePowerState
			)

			if obj != nil && initialPowerState != "" {
				observedPowerState, err = obj.PowerState(ctx)
				Expect(err).ToNot(HaveOccurred())
				if observedPowerState != initialPowerState {
					switch initialPowerState {
					case types.VirtualMachinePowerStatePoweredOff:
						_, err = obj.PowerOff(ctx)
					case types.VirtualMachinePowerStatePoweredOn:
						_, err = obj.PowerOn(ctx)
					case types.VirtualMachinePowerStateSuspended:
						_, err = obj.Suspend(ctx)
					}
					Expect(err).ToNot(HaveOccurred())
					Expect(obj.WaitForPowerState(ctx, initialPowerState)).To(Succeed())
				}
			}

			// The actual test
			result, err = vmutil.SetAndWaitOnPowerState(
				mutateContextFn(logr.NewContext(ctx, suite.GetLogger())),
				ctx.VCClient.Client,
				mgdObj,
				fetchCurrentPowerState,
				desiredPowerState,
				powerOpBehavior)

			e, a := expectedErr, err
			switch {
			case e != nil && a != nil:
				Expect(a.Error()).To(Equal(e.Error()))
			case e == nil && a != nil:
				Fail("unexpected error occurred: " + a.Error())
			case e != nil && a == nil:
				Fail("expected error did not occur: " + e.Error())
			case e == nil && a == nil:
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(expectedResult))
				if !skipPostSetPowerStateVerification {
					observedPowerState, err = obj.PowerState(ctx)
					Expect(err).ToNot(HaveOccurred())
					Expect(observedPowerState).To(Equal(desiredPowerState))
				}
			}
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
		})

		When("The VM does not exist", func() {
			BeforeEach(func() {
				desiredPowerState = types.VirtualMachinePowerStatePoweredOff
				powerOpBehavior = vmutil.PowerOpBehaviorHard
				mgdObj.Self.Value = "does-not-exist"
				initialPowerState = ""
				expectedErr = errors.New("failed to retrieve power state ServerFaultCode: The object has already been deleted or has not been completely created")
			})
			Context("and the current power state is cached in the managed object", func() {
				BeforeEach(func() {
					mgdObj.Summary.Runtime.PowerState = types.VirtualMachinePowerStatePoweredOn
				})
				Context("and fetchCurrentPowerState is true", func() {
					It("a power op should return an error", func() {})
				})
				Context("and fetchCurrentPowerState is false", func() {
					BeforeEach(func() {
						fetchCurrentPowerState = false
						expectedErr = errors.New("failed to invoke hard power op for poweredOff ServerFaultCode: managed object not found: VirtualMachine:does-not-exist")
					})
					It("a power op should return an error", func() {})
				})
			})
			Context("and the current power state is not cached in the managed object", func() {
				BeforeEach(func() {
					mgdObj.Summary.Runtime.PowerState = ""
				})
				Context("and fetchCurrentPowerState is true", func() {
					It("a power op should return an error", func() {})
				})
				Context("and fetchCurrentPowerState is false", func() {
					BeforeEach(func() {
						fetchCurrentPowerState = false
					})
					It("a power op should return an error", func() {})
				})
			})
		})

		When("Powering on a VM", func() {
			BeforeEach(func() {
				desiredPowerState = types.VirtualMachinePowerStatePoweredOn
				expectedResult = vmutil.SetPowerStateResultChanged
			})
			Context("that is powered off", func() {
				BeforeEach(func() {
					initialPowerState = types.VirtualMachinePowerStatePoweredOff
				})
				Context("and the correct power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = initialPowerState
					})
					Context("and fetchCurrentPowerState is false", func() {
						BeforeEach(func() {
							fetchCurrentPowerState = false
						})
						It("should power on the VM", func() {})
					})
					Context("and fetchCurrentPowerState is true", func() {
						It("should power on the VM", func() {})
					})
				})
				Context("and the incorrect power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = desiredPowerState
					})
					Context("and fetchCurrentPowerState is false", func() {
						BeforeEach(func() {
							fetchCurrentPowerState = false
							expectedResult = vmutil.SetPowerStateResultNone
							skipPostSetPowerStateVerification = true
						})
						It("should not power on the VM because it thinks it already is", func() {})
					})
					Context("and fetchCurrentPowerState is true", func() {
						It("should power on the VM", func() {})
					})
				})
				Context("and an empty power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = ""
					})
					It("should power on the VM", func() {})
				})
			})
			Context("that is suspended", func() {
				BeforeEach(func() {
					initialPowerState = types.VirtualMachinePowerStateSuspended
				})
				Context("and the correct power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = initialPowerState
					})
					Context("and fetchCurrentPowerState is false", func() {
						BeforeEach(func() {
							fetchCurrentPowerState = false
						})
						It("should resume the VM", func() {})
					})
					Context("and fetchCurrentPowerState is true", func() {
						It("should resume the VM", func() {})
					})
				})
				Context("and the incorrect power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = desiredPowerState
					})
					Context("and fetchCurrentPowerState is false", func() {
						BeforeEach(func() {
							fetchCurrentPowerState = false
							expectedResult = vmutil.SetPowerStateResultNone
							skipPostSetPowerStateVerification = true
						})
						It("should not resume the VM because it thinks it already is", func() {})
					})
					Context("and fetchCurrentPowerState is true", func() {
						It("should resume on the VM", func() {})
					})
				})
				Context("and an empty power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = ""
					})
					It("should resume the VM", func() {})
				})
			})
		})

		When("Powering off a VM", func() {
			BeforeEach(func() {
				desiredPowerState = types.VirtualMachinePowerStatePoweredOff
			})
			Context("that is powered on", func() {
				BeforeEach(func() {
					initialPowerState = types.VirtualMachinePowerStatePoweredOn
				})
				Context("using hard off", func() {
					BeforeEach(func() {
						powerOpBehavior = vmutil.PowerOpBehaviorHard
						expectedResult = vmutil.SetPowerStateResultChangedHard
					})
					Context("and the correct power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = initialPowerState
						})
						Context("and fetchCurrentPowerState is false", func() {
							BeforeEach(func() {
								fetchCurrentPowerState = false
							})
							It("should power off the VM", func() {})
						})
						Context("and fetchCurrentPowerState is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and the incorrect power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = desiredPowerState
						})
						Context("and fetchCurrentPowerState is false", func() {
							BeforeEach(func() {
								fetchCurrentPowerState = false
								expectedResult = vmutil.SetPowerStateResultNone
								skipPostSetPowerStateVerification = true
							})
							It("should not power off the VM because it thinks it already is", func() {})
						})
						Context("and fetchCurrentPowerState is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and an empty power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = ""
						})
						It("should power off the VM", func() {})
					})
				})
				Context("using soft off", func() {
					BeforeEach(func() {
						powerOpBehavior = vmutil.PowerOpBehaviorSoft
						expectedResult = vmutil.SetPowerStateResultChangedSoft
					})
					Context("and the correct power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = initialPowerState
						})
						Context("and fetchCurrentPowerState is false", func() {
							BeforeEach(func() {
								fetchCurrentPowerState = false
							})
							It("should power off the VM", func() {})
						})
						Context("and fetchCurrentPowerState is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and the incorrect power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = desiredPowerState
						})
						Context("and fetchCurrentPowerState is false", func() {
							BeforeEach(func() {
								fetchCurrentPowerState = false
								expectedResult = vmutil.SetPowerStateResultNone
								skipPostSetPowerStateVerification = true
							})
							It("should not power off the VM because it thinks it already is", func() {})
						})
						Context("and fetchCurrentPowerState is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and an empty power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = ""
						})
						It("should power off the VM", func() {})
					})
				})
				Context("using try soft off", func() {
					BeforeEach(func() {
						powerOpBehavior = vmutil.PowerOpBehaviorTrySoft
						expectedResult = vmutil.SetPowerStateResultChangedSoft
					})
					Context("and the correct power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = initialPowerState
						})
						Context("and fetchCurrentPowerState is false", func() {
							BeforeEach(func() {
								fetchCurrentPowerState = false
							})
							It("should power off the VM", func() {})
						})
						Context("and fetchCurrentPowerState is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and the incorrect power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = desiredPowerState
						})
						Context("and fetchCurrentPowerState is false", func() {
							BeforeEach(func() {
								fetchCurrentPowerState = false
								expectedResult = vmutil.SetPowerStateResultNone
								skipPostSetPowerStateVerification = true
							})
							It("should not power off the VM because it thinks it already is", func() {})
						})
						Context("and fetchCurrentPowerState is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and an empty power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = ""
						})
						It("should power off the VM", func() {})
					})
					Context("and waiting for the power state times out", func() {
						BeforeEach(func() {
							Skip("TODO(akutz) vC Sim ShutdownGuest delay not working as expected")
							expectedResult = vmutil.SetPowerStateResultChangedHard
							mutateContextFn = func(ctx context.Context) context.Context {
								return delayTask(ctx, "ShutdownGuest", 1*time.Second, 500*time.Millisecond)
							}
						})
						It("should power off the VM using a hard op", func() {})
					})
				})
			})
			Context("that is suspended", func() {
				BeforeEach(func() {
					initialPowerState = types.VirtualMachinePowerStateSuspended
				})
				Context("using hard off", func() {
					BeforeEach(func() {
						powerOpBehavior = vmutil.PowerOpBehaviorHard
						expectedResult = vmutil.SetPowerStateResultChangedHard
					})
					Context("and the correct power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = initialPowerState
						})
						Context("and fetchCurrentPowerState is false", func() {
							BeforeEach(func() {
								fetchCurrentPowerState = false
							})
							It("should power off the VM", func() {})
						})
						Context("and fetchCurrentPowerState is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and the incorrect power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = desiredPowerState
						})
						Context("and fetchCurrentPowerState is false", func() {
							BeforeEach(func() {
								fetchCurrentPowerState = false
								expectedResult = vmutil.SetPowerStateResultNone
							})
							It("should not power off the VM because it thinks it already is", func() {})
						})
						Context("and fetchCurrentPowerState is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and an empty power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = ""
						})
						It("should power off the VM", func() {})
					})
				})
				Context("using soft off", func() {
					BeforeEach(func() {
						powerOpBehavior = vmutil.PowerOpBehaviorSoft
						expectedErr = errors.New("soft set power state to poweredOff failed ServerFaultCode: InvalidPowerState")
					})
					Context("and the correct power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = initialPowerState
						})
						Context("and fetchCurrentPowerState is false", func() {
							BeforeEach(func() {
								fetchCurrentPowerState = false
							})
							It("should fail because a suspended VM cannot be powered off with ShutdownGuest", func() {})
						})
						Context("and fetchCurrentPowerState is true", func() {
							It("should fail because a suspended VM cannot be powered off with ShutdownGuest", func() {})
						})
					})
					Context("and the incorrect power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = desiredPowerState
						})
						Context("and fetchCurrentPowerState is false", func() {
							BeforeEach(func() {
								fetchCurrentPowerState = false
								expectedErr = nil
								expectedResult = vmutil.SetPowerStateResultNone
								skipPostSetPowerStateVerification = true
							})
							It("should not power off the VM because it thinks it already is", func() {})
						})
						Context("and fetchCurrentPowerState is true", func() {
							It("should fail because a suspended VM cannot be powered off with ShutdownGuest", func() {})
						})
					})
					Context("and an empty power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = ""
						})
						It("should fail because a suspended VM cannot be powered off with ShutdownGuest", func() {})
					})
				})
			})
		})

		When("Suspending a VM", func() {
			BeforeEach(func() {
				desiredPowerState = types.VirtualMachinePowerStateSuspended
			})
			Context("using hard standby", func() {
				BeforeEach(func() {
					powerOpBehavior = vmutil.PowerOpBehaviorHard
					expectedResult = vmutil.SetPowerStateResultChangedHard
				})
				Context("and the correct power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = initialPowerState
					})
					Context("and fetchCurrentPowerState is false", func() {
						BeforeEach(func() {
							fetchCurrentPowerState = false
						})
						It("should suspend the VM", func() {})
					})
					Context("and fetchCurrentPowerState is true", func() {
						It("should suspend the VM", func() {})
					})
				})
				Context("and the incorrect power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = desiredPowerState
					})
					Context("and fetchCurrentPowerState is false", func() {
						BeforeEach(func() {
							fetchCurrentPowerState = false
							expectedResult = vmutil.SetPowerStateResultNone
							skipPostSetPowerStateVerification = true
						})
						It("should not suspend the VM because it thinks it already is", func() {})
					})
					Context("and fetchCurrentPowerState is true", func() {
						It("should power off the VM", func() {})
					})
				})
				Context("and an empty power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = ""
					})
					It("should suspend the VM", func() {})
				})
			})
			Context("using soft standby", func() {
				BeforeEach(func() {
					powerOpBehavior = vmutil.PowerOpBehaviorSoft
					expectedResult = vmutil.SetPowerStateResultChangedSoft
				})
				Context("and the correct power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = initialPowerState
					})
					Context("and fetchCurrentPowerState is false", func() {
						BeforeEach(func() {
							fetchCurrentPowerState = false
						})
						It("should suspend the VM", func() {})
					})
					Context("and fetchCurrentPowerState is true", func() {
						It("should suspend the VM", func() {})
					})
				})
				Context("and the incorrect power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = desiredPowerState
					})
					Context("and fetchCurrentPowerState is false", func() {
						BeforeEach(func() {
							fetchCurrentPowerState = false
							expectedResult = vmutil.SetPowerStateResultNone
							skipPostSetPowerStateVerification = true
						})
						It("should not suspend the VM because it thinks it already is", func() {})
					})
					Context("and fetchCurrentPowerState is true", func() {
						It("should suspend the VM", func() {})
					})
				})
				Context("and an empty power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = ""
					})
					It("should suspend the VM", func() {})
				})
			})
			Context("using try soft standby", func() {
				BeforeEach(func() {
					powerOpBehavior = vmutil.PowerOpBehaviorTrySoft
					expectedResult = vmutil.SetPowerStateResultChangedSoft
				})
				Context("and the correct power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = initialPowerState
					})
					Context("and fetchCurrentPowerState is false", func() {
						BeforeEach(func() {
							fetchCurrentPowerState = false
						})
						It("should suspend the VM", func() {})
					})
					Context("and fetchCurrentPowerState is true", func() {
						It("should suspend the VM", func() {})
					})
				})
				Context("and the incorrect power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = desiredPowerState
					})
					Context("and fetchCurrentPowerState is false", func() {
						BeforeEach(func() {
							fetchCurrentPowerState = false
							expectedResult = vmutil.SetPowerStateResultNone
							skipPostSetPowerStateVerification = true
						})
						It("should not suspend the VM because it thinks it already is", func() {})
					})
					Context("and fetchCurrentPowerState is true", func() {
						It("should suspend the VM", func() {})
					})
				})
				Context("and an empty power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = ""
					})
					It("should suspend the VM", func() {})
				})
				Context("and waiting for the power state times out", func() {
					BeforeEach(func() {
						Skip("TODO(akutz) vC Sim StandbyGuest delay not working as expected")
						expectedResult = vmutil.SetPowerStateResultChangedHard
						mutateContextFn = func(ctx context.Context) context.Context {
							return delayTask(ctx, "StandbyGuest", 1*time.Second, 500*time.Millisecond)
						}
					})
					It("should suspend the VM using a hard op", func() {})
				})
			})
		})
	})
}
