// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm_test

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
	vmutilInternal "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm/internal"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

//nolint:gocyclo
func powerStateTests() {
	var (
		mutateContextFn func(context.Context) context.Context
		delayTask       = func(
			ctx context.Context,
			taskName string,
			taskDelay, softTimeout time.Duration) context.Context {

			simulator.TaskDelay.MethodDelay[taskName] = int(taskDelay.Milliseconds())
			simulator.TaskDelay.MethodDelay["LockHandoff"] = 0 // don't lock vm during the delay
			return context.WithValue(ctx, vmutilInternal.SoftTimeoutKey, softTimeout)
		}
	)

	BeforeEach(func() {
		mutateContextFn = func(ctx context.Context) context.Context {
			return ctx
		}
	})

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
			Expect(vmutil.ParsePowerState("")).To(Equal(vimtypes.VirtualMachinePowerState("")))
			Expect(vmutil.ParsePowerState("fake")).To(Equal(vimtypes.VirtualMachinePowerState("")))
		})
		It("should parse variants of poweredOn", func() {
			Expect(vmutil.ParsePowerState("poweredOn")).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
			Expect(vmutil.ParsePowerState("PoweredOn")).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
			Expect(vmutil.ParsePowerState("Poweredon")).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
			Expect(vmutil.ParsePowerState("poweredon")).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
			Expect(vmutil.ParsePowerState("POWEREDON")).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
		})
		It("should parse variants of poweredOff", func() {
			Expect(vmutil.ParsePowerState("poweredOff")).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
			Expect(vmutil.ParsePowerState("PoweredOff")).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
			Expect(vmutil.ParsePowerState("Poweredoff")).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
			Expect(vmutil.ParsePowerState("poweredoff")).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
			Expect(vmutil.ParsePowerState("POWEREDOFF")).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
		})
		It("should parse variants of suspended", func() {
			Expect(vmutil.ParsePowerState("suspended")).To(Equal(vimtypes.VirtualMachinePowerStateSuspended))
			Expect(vmutil.ParsePowerState("Suspended")).To(Equal(vimtypes.VirtualMachinePowerStateSuspended))
			Expect(vmutil.ParsePowerState("SUSPENDED")).To(Equal(vimtypes.VirtualMachinePowerStateSuspended))
		})
	})

	Context("SetPowerState", func() {
		var (
			ctx                               *builder.TestContextForVCSim
			mgdObj                            mo.VirtualMachine
			obj                               *object.VirtualMachine
			expectedErr                       error
			expectedResult                    vmutil.PowerOpResult
			fetchProperties                   bool
			powerOpBehavior                   vmutil.PowerOpBehavior
			initialPowerState                 vimtypes.VirtualMachinePowerState
			desiredPowerState                 vimtypes.VirtualMachinePowerState
			skipPostSetPowerStateVerification bool
		)

		BeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
			vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(vmList)).To(BeNumerically(">", 0))
			mgdObj = vmutil.ManagedObjectFromMoRef(vmList[0].Reference())
			simulator.TaskDelay.MethodDelay = map[string]int{}
			obj = object.NewVirtualMachine(ctx.VCClient.Client, mgdObj.Self)
			expectedResult = 0                                             // default
			expectedErr = nil                                              // default
			fetchProperties = true                                         // default
			initialPowerState = vimtypes.VirtualMachinePowerStatePoweredOn // default
		})

		JustBeforeEach(func() {
			var (
				err                error
				result             vmutil.PowerOpResult
				observedPowerState vimtypes.VirtualMachinePowerState
			)

			if obj != nil && initialPowerState != "" {
				observedPowerState, err = obj.PowerState(ctx)
				Expect(err).ToNot(HaveOccurred())
				if observedPowerState != initialPowerState {
					switch initialPowerState {
					case vimtypes.VirtualMachinePowerStatePoweredOff:
						_, err = obj.PowerOff(ctx)
					case vimtypes.VirtualMachinePowerStatePoweredOn:
						_, err = obj.PowerOn(ctx)
					case vimtypes.VirtualMachinePowerStateSuspended:
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
				fetchProperties,
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
				desiredPowerState = vimtypes.VirtualMachinePowerStatePoweredOff
				powerOpBehavior = vmutil.PowerOpBehaviorHard
				mgdObj.Self.Value = doesNotExist
				initialPowerState = ""
				expectedErr = errors.New("failed to retrieve properties ServerFaultCode: The object has already been deleted or has not been completely created")
			})
			Context("and the current power state is cached in the managed object", func() {
				BeforeEach(func() {
					mgdObj.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
				})
				Context("and fetchProperties is true", func() {
					It("a power op should return an error", func() {})
				})
				Context("and fetchProperties is false", func() {
					BeforeEach(func() {
						fetchProperties = false
						expectedErr = errors.New("failed to invoke hard power op for poweredOff ServerFaultCode: managed object not found: VirtualMachine:does-not-exist")
					})
					It("a power op should return an error", func() {})
				})
			})
			Context("and the current power state is not cached in the managed object", func() {
				BeforeEach(func() {
					mgdObj.Summary.Runtime.PowerState = ""
				})
				Context("and fetchProperties is true", func() {
					It("a power op should return an error", func() {})
				})
				Context("and fetchProperties is false", func() {
					BeforeEach(func() {
						fetchProperties = false
					})
					It("a power op should return an error", func() {})
				})
			})
		})

		When("Powering on a VM", func() {
			BeforeEach(func() {
				desiredPowerState = vimtypes.VirtualMachinePowerStatePoweredOn
				expectedResult = vmutil.PowerOpResultChanged
			})
			Context("that is powered off", func() {
				BeforeEach(func() {
					initialPowerState = vimtypes.VirtualMachinePowerStatePoweredOff
				})
				Context("and the correct power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = initialPowerState
					})
					Context("and fetchProperties is false", func() {
						BeforeEach(func() {
							fetchProperties = false
						})
						It("should power on the VM", func() {})
					})
					Context("and fetchProperties is true", func() {
						It("should power on the VM", func() {})
					})
				})
				Context("and the incorrect power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = desiredPowerState
					})
					Context("and fetchProperties is false", func() {
						BeforeEach(func() {
							fetchProperties = false
							expectedResult = vmutil.PowerOpResultNone
							skipPostSetPowerStateVerification = true
						})
						It("should not power on the VM because it thinks it already is", func() {})
					})
					Context("and fetchProperties is true", func() {
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
					initialPowerState = vimtypes.VirtualMachinePowerStateSuspended
				})
				Context("and the correct power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = initialPowerState
					})
					Context("and fetchProperties is false", func() {
						BeforeEach(func() {
							fetchProperties = false
						})
						It("should resume the VM", func() {})
					})
					Context("and fetchProperties is true", func() {
						It("should resume the VM", func() {})
					})
				})
				Context("and the incorrect power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = desiredPowerState
					})
					Context("and fetchProperties is false", func() {
						BeforeEach(func() {
							fetchProperties = false
							expectedResult = vmutil.PowerOpResultNone
							skipPostSetPowerStateVerification = true
						})
						It("should not resume the VM because it thinks it already is", func() {})
					})
					Context("and fetchProperties is true", func() {
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
				desiredPowerState = vimtypes.VirtualMachinePowerStatePoweredOff
			})
			Context("that is powered on", func() {
				BeforeEach(func() {
					initialPowerState = vimtypes.VirtualMachinePowerStatePoweredOn
				})
				Context("using hard off", func() {
					BeforeEach(func() {
						powerOpBehavior = vmutil.PowerOpBehaviorHard
						expectedResult = vmutil.PowerOpResultChangedHard
					})
					Context("and the correct power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = initialPowerState
						})
						Context("and fetchProperties is false", func() {
							BeforeEach(func() {
								fetchProperties = false
							})
							It("should power off the VM", func() {})
						})
						Context("and fetchProperties is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and the incorrect power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = desiredPowerState
						})
						Context("and fetchProperties is false", func() {
							BeforeEach(func() {
								fetchProperties = false
								expectedResult = vmutil.PowerOpResultNone
								skipPostSetPowerStateVerification = true
							})
							It("should not power off the VM because it thinks it already is", func() {})
						})
						Context("and fetchProperties is true", func() {
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
						expectedResult = vmutil.PowerOpResultChangedSoft
					})
					Context("and the correct power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = initialPowerState
						})
						Context("and fetchProperties is false", func() {
							BeforeEach(func() {
								fetchProperties = false
							})
							It("should power off the VM", func() {})
						})
						Context("and fetchProperties is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and the incorrect power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = desiredPowerState
						})
						Context("and fetchProperties is false", func() {
							BeforeEach(func() {
								fetchProperties = false
								expectedResult = vmutil.PowerOpResultNone
								skipPostSetPowerStateVerification = true
							})
							It("should not power off the VM because it thinks it already is", func() {})
						})
						Context("and fetchProperties is true", func() {
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
						expectedResult = vmutil.PowerOpResultChangedSoft
					})
					Context("and the correct power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = initialPowerState
						})
						Context("and fetchProperties is false", func() {
							BeforeEach(func() {
								fetchProperties = false
							})
							It("should power off the VM", func() {})
						})
						Context("and fetchProperties is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and the incorrect power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = desiredPowerState
						})
						Context("and fetchProperties is false", func() {
							BeforeEach(func() {
								fetchProperties = false
								expectedResult = vmutil.PowerOpResultNone
								skipPostSetPowerStateVerification = true
							})
							It("should not power off the VM because it thinks it already is", func() {})
						})
						Context("and fetchProperties is true", func() {
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
							expectedResult = vmutil.PowerOpResultChangedHard
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
					initialPowerState = vimtypes.VirtualMachinePowerStateSuspended
				})
				Context("using hard off", func() {
					BeforeEach(func() {
						powerOpBehavior = vmutil.PowerOpBehaviorHard
						expectedResult = vmutil.PowerOpResultChangedHard
					})
					Context("and the correct power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = initialPowerState
						})
						Context("and fetchProperties is false", func() {
							BeforeEach(func() {
								fetchProperties = false
							})
							It("should power off the VM", func() {})
						})
						Context("and fetchProperties is true", func() {
							It("should power off the VM", func() {})
						})
					})
					Context("and the incorrect power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = desiredPowerState
						})
						Context("and fetchProperties is false", func() {
							BeforeEach(func() {
								fetchProperties = false
								expectedResult = vmutil.PowerOpResultNone
							})
							It("should not power off the VM because it thinks it already is", func() {})
						})
						Context("and fetchProperties is true", func() {
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
						Context("and fetchProperties is false", func() {
							BeforeEach(func() {
								fetchProperties = false
							})
							It("should fail because a suspended VM cannot be powered off with ShutdownGuest", func() {})
						})
						Context("and fetchProperties is true", func() {
							It("should fail because a suspended VM cannot be powered off with ShutdownGuest", func() {})
						})
					})
					Context("and the incorrect power state is cached in the managed object", func() {
						BeforeEach(func() {
							mgdObj.Summary.Runtime.PowerState = desiredPowerState
						})
						Context("and fetchProperties is false", func() {
							BeforeEach(func() {
								fetchProperties = false
								expectedErr = nil
								expectedResult = vmutil.PowerOpResultNone
								skipPostSetPowerStateVerification = true
							})
							It("should not power off the VM because it thinks it already is", func() {})
						})
						Context("and fetchProperties is true", func() {
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
				desiredPowerState = vimtypes.VirtualMachinePowerStateSuspended
			})
			Context("using hard standby", func() {
				BeforeEach(func() {
					powerOpBehavior = vmutil.PowerOpBehaviorHard
					expectedResult = vmutil.PowerOpResultChangedHard
				})
				Context("and the correct power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = initialPowerState
					})
					Context("and fetchProperties is false", func() {
						BeforeEach(func() {
							fetchProperties = false
						})
						It("should suspend the VM", func() {})
					})
					Context("and fetchProperties is true", func() {
						It("should suspend the VM", func() {})
					})
				})
				Context("and the incorrect power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = desiredPowerState
					})
					Context("and fetchProperties is false", func() {
						BeforeEach(func() {
							fetchProperties = false
							expectedResult = vmutil.PowerOpResultNone
							skipPostSetPowerStateVerification = true
						})
						It("should not suspend the VM because it thinks it already is", func() {})
					})
					Context("and fetchProperties is true", func() {
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
					expectedResult = vmutil.PowerOpResultChangedSoft
				})
				Context("and the correct power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = initialPowerState
					})
					Context("and fetchProperties is false", func() {
						BeforeEach(func() {
							fetchProperties = false
						})
						It("should suspend the VM", func() {})
					})
					Context("and fetchProperties is true", func() {
						It("should suspend the VM", func() {})
					})
				})
				Context("and the incorrect power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = desiredPowerState
					})
					Context("and fetchProperties is false", func() {
						BeforeEach(func() {
							fetchProperties = false
							expectedResult = vmutil.PowerOpResultNone
							skipPostSetPowerStateVerification = true
						})
						It("should not suspend the VM because it thinks it already is", func() {})
					})
					Context("and fetchProperties is true", func() {
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
					expectedResult = vmutil.PowerOpResultChangedSoft
				})
				Context("and the correct power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = initialPowerState
					})
					Context("and fetchProperties is false", func() {
						BeforeEach(func() {
							fetchProperties = false
						})
						It("should suspend the VM", func() {})
					})
					Context("and fetchProperties is true", func() {
						It("should suspend the VM", func() {})
					})
				})
				Context("and the incorrect power state is cached in the managed object", func() {
					BeforeEach(func() {
						mgdObj.Summary.Runtime.PowerState = desiredPowerState
					})
					Context("and fetchProperties is false", func() {
						BeforeEach(func() {
							fetchProperties = false
							expectedResult = vmutil.PowerOpResultNone
							skipPostSetPowerStateVerification = true
						})
						It("should not suspend the VM because it thinks it already is", func() {})
					})
					Context("and fetchProperties is true", func() {
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
						expectedResult = vmutil.PowerOpResultChangedHard
						mutateContextFn = func(ctx context.Context) context.Context {
							return delayTask(ctx, "StandbyGuest", 1*time.Second, 500*time.Millisecond)
						}
					})
					It("should suspend the VM using a hard op", func() {})
				})
			})
		})
	})

	Context("Restart", func() {
		var (
			ctx                    *builder.TestContextForVCSim
			mgdObj                 mo.VirtualMachine
			obj                    *object.VirtualMachine
			expectedErr            error
			expectedResult         vmutil.PowerOpResult
			expectedPowerState     vimtypes.VirtualMachinePowerState
			fetchProperties        bool
			desiredLastRestartTime time.Time
			powerOpBehavior        vmutil.PowerOpBehavior
			initialPowerState      vimtypes.VirtualMachinePowerState
			initialLastRestartTime *time.Time
			mutateContextFn        func(context.Context) context.Context
		)

		getLastRestartTime := func(
			ctx context.Context,
			obj *object.VirtualMachine) *time.Time {

			var vm mo.VirtualMachine
			Expect(obj.Properties(
				ctx,
				obj.Reference(),
				[]string{"config.extraConfig"},
				&vm)).To(Succeed())
			t, err := vmutil.GetLastRestartTimeFromExtraConfig(ctx, vm.Config.ExtraConfig)
			Expect(err).ToNot(HaveOccurred())
			return t
		}

		setAndAssertLastRestartTime := func(
			ctx context.Context,
			obj *object.VirtualMachine,
			lastRestartTime time.Time) {

			task, err := obj.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
				ExtraConfig: []vimtypes.BaseOptionValue{
					&vimtypes.OptionValue{
						Key:   vmutil.ExtraConfigKeyLastRestartTime,
						Value: strconv.FormatInt(lastRestartTime.UnixNano(), 10),
					},
				},
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())
			observedLastRestartTime := getLastRestartTime(ctx, obj)
			Expect(observedLastRestartTime.UnixNano()).To(Equal(lastRestartTime.UnixNano()))
		}

		BeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
			vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(vmList)).To(BeNumerically(">", 0))
			mgdObj = vmutil.ManagedObjectFromMoRef(vmList[0].Reference())
			simulator.TaskDelay.MethodDelay = map[string]int{}
			obj = object.NewVirtualMachine(ctx.VCClient.Client, mgdObj.Self)
			expectedPowerState = vimtypes.VirtualMachinePowerStatePoweredOn // default
			expectedResult = 0                                              // default
			expectedErr = nil                                               // default
			fetchProperties = true                                          // default
			initialPowerState = vimtypes.VirtualMachinePowerStatePoweredOn  // default
			initialLastRestartTime = nil                                    // default
			mutateContextFn = func(ctx context.Context) context.Context {
				return ctx
			}
		})

		JustBeforeEach(func() {
			var (
				err                error
				result             vmutil.PowerOpResult
				observedPowerState vimtypes.VirtualMachinePowerState
			)

			if obj != nil && initialLastRestartTime != nil {
				// Assign an initial lastRestartTime to the VM's ExtraConfig
				// and assert that it was set correctly.
				setAndAssertLastRestartTime(ctx, obj, *initialLastRestartTime)
			}

			if obj != nil && initialPowerState != "" {
				observedPowerState, err = obj.PowerState(ctx)
				Expect(err).ToNot(HaveOccurred())
				if observedPowerState != initialPowerState {
					switch initialPowerState {
					case vimtypes.VirtualMachinePowerStatePoweredOff:
						_, err = obj.PowerOff(ctx)
					case vimtypes.VirtualMachinePowerStatePoweredOn:
						_, err = obj.PowerOn(ctx)
					case vimtypes.VirtualMachinePowerStateSuspended:
						_, err = obj.Suspend(ctx)
					}
					Expect(err).ToNot(HaveOccurred())
					Expect(obj.WaitForPowerState(ctx, initialPowerState)).To(Succeed())
				}
			}

			// The actual test
			result, err = vmutil.RestartAndWait(
				mutateContextFn(logr.NewContext(ctx, suite.GetLogger())),
				ctx.VCClient.Client,
				mgdObj,
				fetchProperties,
				desiredLastRestartTime,
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
				observedPowerState, err = obj.PowerState(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(observedPowerState).To(Equal(expectedPowerState))

				if result != vmutil.PowerOpResultNone && initialLastRestartTime != nil {
					observedLastRestartTime := getLastRestartTime(ctx, obj)
					Expect(observedLastRestartTime).ToNot(BeNil())
					suite.GetLogger().Info("comparing last restart times",
						"initialLastRestartTime", initialLastRestartTime.Format(time.RFC3339Nano),
						"initialLastRestartTimeEpoch", initialLastRestartTime.UnixNano(),
						"observedLastRestartTime", observedLastRestartTime.Format(time.RFC3339Nano),
						"observedLastRestartTimeEpoch", observedLastRestartTime.UnixNano(),
						"desiredLastRestartTime", desiredLastRestartTime.Format(time.RFC3339Nano),
						"desiredLastRestartTimeEpoch", desiredLastRestartTime.UnixNano())
					Expect(observedLastRestartTime.After(*initialLastRestartTime)).To(BeTrue())
					Expect(observedLastRestartTime.Equal(desiredLastRestartTime)).To(BeTrue())
				}
			}
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
		})

		When("The VM does not exist", func() {
			BeforeEach(func() {
				powerOpBehavior = vmutil.PowerOpBehaviorHard
				mgdObj.Self.Value = doesNotExist
				initialPowerState = ""
				expectedErr = errors.New("failed to retrieve properties ServerFaultCode: The object has already been deleted or has not been completely created")
			})
			Context("and the current power state is cached in the managed object", func() {
				BeforeEach(func() {
					mgdObj.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
				})
				Context("and fetchProperties is true", func() {
					It("a power op should return an error", func() {})
				})
				Context("and fetchProperties is false", func() {
					BeforeEach(func() {
						fetchProperties = false
					})
					It("a power op should return an error", func() {})
				})
			})
			Context("and the current power state is not cached in the managed object", func() {
				BeforeEach(func() {
					mgdObj.Summary.Runtime.PowerState = ""
				})
				Context("and fetchProperties is true", func() {
					It("a power op should return an error", func() {})
				})
				Context("and fetchProperties is false", func() {
					BeforeEach(func() {
						fetchProperties = false
					})
					It("a power op should return an error", func() {})
				})
			})
			Context("and the lastRestartTime is cached in the managed object", func() {
				BeforeEach(func() {
					mgdObj.Config = &vimtypes.VirtualMachineConfigInfo{
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   vmutil.ExtraConfigKeyLastRestartTime,
								Value: strconv.FormatInt(time.Now().UTC().UnixNano(), 10),
							},
						},
					}
				})
				Context("and fetchProperties is true", func() {
					It("a power op should return an error", func() {})
				})
				Context("and fetchProperties is false", func() {
					BeforeEach(func() {
						fetchProperties = false
					})
					It("a power op should return an error", func() {})
				})
			})
			Context("and the lastRestartTime is not cached in the managed object", func() {
				BeforeEach(func() {
					mgdObj.Config = nil
				})
				Context("and fetchProperties is true", func() {
					It("a power op should return an error", func() {})
				})
				Context("and fetchProperties is false", func() {
					BeforeEach(func() {
						fetchProperties = false
					})
					It("a power op should return an error", func() {})
				})
			})
			Context("and the current power state and lastRestartTime are both cached in the managed object", func() {
				BeforeEach(func() {
					mgdObj.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
					mgdObj.Config = &vimtypes.VirtualMachineConfigInfo{
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   vmutil.ExtraConfigKeyLastRestartTime,
								Value: strconv.FormatInt(time.Now().UTC().UnixNano(), 10),
							},
						},
					}
				})
				Context("and fetchProperties is true", func() {
					It("a power op should return an error", func() {})
				})
				Context("and fetchProperties is false", func() {
					BeforeEach(func() {
						fetchProperties = false
						expectedErr = nil
						expectedResult = vmutil.PowerOpResultNone
					})
					It("a power op should be a no-op since the current power state cannot be determined", func() {})
				})
			})
			Context("and neither the current power state nor lastRestartTime are not cached in the managed object", func() {
				BeforeEach(func() {
					mgdObj.Summary.Runtime.PowerState = ""
					mgdObj.Config = nil
				})
				Context("and fetchProperties is true", func() {
					It("a power op should return an error", func() {})
				})
				Context("and fetchProperties is false", func() {
					BeforeEach(func() {
						fetchProperties = false
					})
					It("a power op should return an error", func() {})
				})
			})
		})

		When("Restarting a VM", func() {
			var now time.Time

			BeforeEach(func() {
				now = time.Now().UTC().Add(-1 * time.Hour)
				now := now // capture this
				initialLastRestartTime = &now
				desiredLastRestartTime = now.Add(1 * time.Minute)
			})
			Context("using hard restart", func() {
				BeforeEach(func() {
					powerOpBehavior = vmutil.PowerOpBehaviorHard
					expectedResult = vmutil.PowerOpResultChangedHard
				})

				It("should restart the VM", func() {})

				When("desired last restart time is older than last restart", func() {
					BeforeEach(func() {
						desiredLastRestartTime = now.Add(-1 * time.Minute)
						expectedResult = vmutil.PowerOpResultNone
					})
					It("should not restart the VM", func() {})
				})
				When("desired last restart time is in the future", func() {
					BeforeEach(func() {
						desiredLastRestartTime = now.Add(10 * time.Hour)
						expectedResult = vmutil.PowerOpResultNone
					})
					It("should not restart the VM", func() {})
				})
			})
			Context("using soft restart", func() {
				BeforeEach(func() {
					powerOpBehavior = vmutil.PowerOpBehaviorSoft
				})
				Context("with tools not running", func() {
					BeforeEach(func() {
						expectedErr = errors.New("failed to soft restart vm ServerFaultCode: ToolsUnavailable")
					})
					It("should not restart the VM", func() {})
				})
				Context("with tools running", func() {
					BeforeEach(func() {
						expectedResult = vmutil.PowerOpResultChangedSoft
						task, err := obj.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   "SET.guest.toolsRunningStatus",
									Value: vimtypes.VirtualMachineToolsRunningStatusGuestToolsRunning,
								},
							},
						})
						Expect(err).ShouldNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())
					})
					It("should restart the VM", func() {})
				})
			})
			Context("using try soft restart", func() {
				BeforeEach(func() {
					powerOpBehavior = vmutil.PowerOpBehaviorTrySoft
				})
				Context("with tools not running", func() {
					BeforeEach(func() {
						expectedResult = vmutil.PowerOpResultChangedHard
					})
					It("should not restart the VM", func() {})
				})
				Context("with tools running", func() {
					BeforeEach(func() {
						expectedResult = vmutil.PowerOpResultChangedSoft
						task, err := obj.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   "SET.guest.toolsRunningStatus",
									Value: vimtypes.VirtualMachineToolsRunningStatusGuestToolsRunning,
								},
							},
						})
						Expect(err).ShouldNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())
					})
					It("should restart the VM", func() {})
				})
			})
		})
	})
}
