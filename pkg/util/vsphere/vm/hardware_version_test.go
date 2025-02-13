// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vm_test

import (
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func hardwareVersionTests() {
	Describe("ReconcileMinHardwareVersion", func() {
		var (
			ctx          *builder.TestContextForVCSim
			mgdObj       mo.VirtualMachine
			moRef        vimtypes.ManagedObjectReference
			obj          *object.VirtualMachine
			propsToFetch = []string{vmutil.HardwareVersionProperty}
		)

		BeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
			vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
			Expect(err).ToNot(HaveOccurred())
			Expect(vmList).ToNot(BeEmpty())
			moRef = vmList[0].Reference()
			mgdObj = vmutil.ManagedObjectFromMoRef(moRef)
			mgdObj.Config = &vimtypes.VirtualMachineConfigInfo{}
			simulator.TaskDelay.MethodDelay = map[string]int{}
			obj = object.NewVirtualMachine(ctx.VCClient.Client, mgdObj.Self)

			// If the VM is not powered off, then power it off.
			powerState, err := obj.PowerState(ctx)
			Expect(err).ToNot(HaveOccurred())
			if powerState == vimtypes.VirtualMachinePowerStatePoweredOn {
				tsk, err := obj.PowerOff(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(tsk.WaitEx(ctx)).To(Succeed())
			}
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
		})

		type testArgs struct {
			cachedHardwareVersion  string
			cachedPowerState       vimtypes.VirtualMachinePowerState
			cachedSelfValue        string
			expectedErrFn          func(err error)
			expectedResult         vmutil.ReconcileMinHardwareVersionResult
			expectedVersion        string
			fetchProperties        bool
			initialHardwareVersion string
			initialPowerState      vimtypes.VirtualMachinePowerState
			minHardwareVersion     int32
			nilClient              bool
			nilCtx                 bool
		}

		assertInvalidPowerStateFaultPoweredOn := func(err error) {
			var psFault *vimtypes.InvalidPowerStateFault
			_, ok := fault.As(err, &psFault)
			ExpectWithOffset(1, ok).To(BeTrue())
			ExpectWithOffset(1, psFault.ExistingState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
			ExpectWithOffset(1, psFault.RequestedState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
		}

		assertInvalidPowerStateFaultSuspended := func(err error) {
			var psFault *vimtypes.InvalidPowerStateFault
			_, ok := fault.As(err, &psFault)
			ExpectWithOffset(1, ok).To(BeTrue())
			ExpectWithOffset(1, psFault.ExistingState).To(Equal(vimtypes.VirtualMachinePowerStateSuspended))
			ExpectWithOffset(1, psFault.RequestedState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
		}

		assertAlreadyUpgradedFault := func(err error) {
			ok := fault.Is(err, &vimtypes.AlreadyUpgradedFault{})
			Expect(ok).To(BeTrue())
		}

		assertFailedToRetrievePropsNotFound := func(err error) {
			ExpectWithOffset(1, err).To(HaveOccurred())
			ExpectWithOffset(1, err.Error()).To(Equal("failed to retrieve properties ServerFaultCode: The object has already been deleted or has not been completely created"))
		}

		assertFailedToUpgradeNotFound := func(err error) {
			ExpectWithOffset(1, err).To(HaveOccurred())
			ExpectWithOffset(1, err.Error()).To(Equal("failed to invoke upgrade vm: ServerFaultCode: managed object not found: VirtualMachine:does-not-exist"))
		}

		doTest := func(args testArgs) {
			// Configure the VM's initial hardware version.
			if args.initialHardwareVersion != "" {
				tsk, err := obj.Reconfigure(
					ctx,
					vimtypes.VirtualMachineConfigSpec{
						Version: args.initialHardwareVersion,
					})
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
				ExpectWithOffset(1, tsk.WaitEx(ctx)).To(Succeed())
			}

			// Configure the VM's initial power state.
			switch args.initialPowerState {

			case "", vimtypes.VirtualMachinePowerStatePoweredOff:
				// No-op

			case vimtypes.VirtualMachinePowerStatePoweredOn:
				tsk, err := obj.PowerOn(ctx)
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
				ExpectWithOffset(1, tsk.WaitEx(ctx)).To(Succeed())

			case vimtypes.VirtualMachinePowerStateSuspended:
				tsk, err := obj.PowerOn(ctx)
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
				ExpectWithOffset(1, tsk.WaitEx(ctx)).To(Succeed())
				tsk, err = obj.Suspend(ctx)
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
				ExpectWithOffset(1, tsk.WaitEx(ctx)).To(Succeed())

			default:
				Fail(fmt.Sprintf("invalid initial power state: %s", args.initialPowerState), 1)
			}

			// Update the cache.
			mgdObj.Config.Version = args.cachedHardwareVersion
			mgdObj.Runtime.PowerState = args.cachedPowerState
			if args.cachedSelfValue != "" {
				mgdObj.Self.Value = args.cachedSelfValue
			}

			argClient := ctx.VCClient.Client
			argCtx := logr.NewContext(ctx, suite.GetLogger())

			if args.nilClient {
				argClient = nil
			}
			if args.nilCtx {
				argCtx = nil
			}

			// Call the tested function.
			result, err := vmutil.ReconcileMinHardwareVersion(
				argCtx,
				argClient,
				mgdObj,
				args.fetchProperties,
				args.minHardwareVersion)

			if fn := args.expectedErrFn; fn != nil {
				fn(err)
			}
			ExpectWithOffset(1, result).To(Equal(args.expectedResult))

			if args.expectedVersion != "" {
				ExpectWithOffset(1, obj.Properties(ctx, moRef, propsToFetch, &mgdObj)).To(Succeed())
				ExpectWithOffset(1, mgdObj.Config.Version).To(Equal(args.expectedVersion))
			}
		}

		DescribeTable(
			"when function params are invalid",
			doTest,

			Entry(
				"should return error when context is nil",
				testArgs{
					expectedErrFn: func(err error) {
						ExpectWithOffset(1, err).To(HaveOccurred())
						ExpectWithOffset(1, err.Error()).To(Equal("invalid ctx: nil"))
					},
					nilCtx: true,
				},
			),
			Entry(
				"should return error when client is nil",
				testArgs{
					expectedErrFn: func(err error) {
						ExpectWithOffset(1, err).To(HaveOccurred())
						ExpectWithOffset(1, err.Error()).To(Equal("invalid client: nil"))
					},
					nilClient: true,
				},
			),

			Entry(
				"should return error when minHardwareVersion is less than MinValidHardwareVersion",
				testArgs{
					expectedErrFn: func(err error) {
						ExpectWithOffset(1, err).To(HaveOccurred())
						ExpectWithOffset(1, err.Error()).To(Equal(
							fmt.Sprintf("invalid minHardwareVersion: %d",
								int32(vimtypes.MinValidHardwareVersion)-1)))
					},
					minHardwareVersion: int32(vimtypes.MinValidHardwareVersion) - 1,
				},
			),
			Entry(
				"should return error when minHardwareVersion is greater than MaxValidHardwareVersion",
				testArgs{
					expectedErrFn: func(err error) {
						ExpectWithOffset(1, err).To(HaveOccurred())
						ExpectWithOffset(1, err.Error()).To(Equal(
							fmt.Sprintf("invalid minHardwareVersion: %d",
								int32(vimtypes.MaxValidHardwareVersion)+1)))
					},
					minHardwareVersion: int32(vimtypes.MaxValidHardwareVersion) + 1,
				},
			),
		)

		DescribeTable(
			"when vm does not exist",
			doTest,

			Entry(
				"should fail to upgrade vm with NotFound error from RetrieveProperties when fetchProperties is true",
				testArgs{
					cachedSelfValue:        doesNotExist,
					expectedErrFn:          assertFailedToRetrievePropsNotFound,
					expectedVersion:        vimtypes.VMX15.String(),
					fetchProperties:        true,
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should fail to upgrade vm with NotFound error from RetrieveProperties when fetchProperties is false, hardware version is not cached, and power state is not cached",
				testArgs{
					cachedSelfValue:        doesNotExist,
					expectedErrFn:          assertFailedToRetrievePropsNotFound,
					expectedVersion:        vimtypes.VMX15.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should fail to upgrade vm with NotFound error from RetrieveProperties when fetchProperties is false, hardware version is cached, and power state is not cached",
				testArgs{
					cachedHardwareVersion:  vimtypes.VMX15.String(),
					cachedSelfValue:        doesNotExist,
					expectedErrFn:          assertFailedToRetrievePropsNotFound,
					expectedVersion:        vimtypes.VMX15.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should fail to upgrade vm with NotFound error from UpgradeVm when fetchProperties is false, hardware version is cached, and power state is cached",
				testArgs{
					cachedHardwareVersion:  vimtypes.VMX15.String(),
					cachedPowerState:       vimtypes.VirtualMachinePowerStatePoweredOff,
					cachedSelfValue:        doesNotExist,
					expectedErrFn:          assertFailedToUpgradeNotFound,
					expectedVersion:        vimtypes.VMX15.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
		)

		DescribeTable(
			"when cached properties are missing",
			doTest,

			Entry(
				"should upgrade vm when fetchProperties is true",
				testArgs{
					expectedResult:         vmutil.ReconcileMinHardwareVersionResultUpgraded,
					expectedVersion:        vimtypes.VMX17.String(),
					fetchProperties:        true,
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should upgrade vm when fetchProperties is false, hardware version is not cached, and power state is not cached",
				testArgs{
					expectedResult:         vmutil.ReconcileMinHardwareVersionResultUpgraded,
					expectedVersion:        vimtypes.VMX17.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should upgrade vm when fetchProperties is false, hardware version is cached, and power state is not cached",
				testArgs{
					cachedHardwareVersion:  vimtypes.VMX15.String(),
					expectedResult:         vmutil.ReconcileMinHardwareVersionResultUpgraded,
					expectedVersion:        vimtypes.VMX17.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should upgrade vm when fetchProperties is false, hardware version is not cached, and power state is cached",
				testArgs{
					cachedPowerState:       vimtypes.VirtualMachinePowerStatePoweredOff,
					expectedResult:         vmutil.ReconcileMinHardwareVersionResultUpgraded,
					expectedVersion:        vimtypes.VMX17.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
		)

		DescribeTable(
			"when cached properties are incorrect",
			doTest,

			Entry(
				"should upgrade vm when fetchProperties is true, incorrect hardware version is cached, and incorrect power state is cached",
				testArgs{
					cachedHardwareVersion:  vimtypes.VMX17.String(),
					cachedPowerState:       vimtypes.VirtualMachinePowerStatePoweredOn,
					expectedResult:         vmutil.ReconcileMinHardwareVersionResultUpgraded,
					expectedVersion:        vimtypes.VMX17.String(),
					fetchProperties:        true,
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should skip upgrade vm when fetchProperties is false, correct hardware version is cached, and power state is cached as powered on",
				testArgs{
					cachedHardwareVersion:  vimtypes.VMX15.String(),
					cachedPowerState:       vimtypes.VirtualMachinePowerStatePoweredOn,
					expectedResult:         vmutil.ReconcileMinHardwareVersionResultNotPoweredOff,
					expectedVersion:        vimtypes.VMX15.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should skip upgrade vm when fetchProperties is false, correct hardware version is cached, and power state is cached as suspended",
				testArgs{
					cachedHardwareVersion:  vimtypes.VMX15.String(),
					cachedPowerState:       vimtypes.VirtualMachinePowerStateSuspended,
					expectedResult:         vmutil.ReconcileMinHardwareVersionResultNotPoweredOff,
					expectedVersion:        vimtypes.VMX15.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should skip upgrade vm when fetchProperties is false, hardware version is cached as if already upgraded, and correct power state is cached",
				testArgs{
					cachedHardwareVersion:  vimtypes.VMX17.String(),
					cachedPowerState:       vimtypes.VirtualMachinePowerStatePoweredOff,
					expectedResult:         vmutil.ReconcileMinHardwareVersionResultAlreadyUpgraded,
					expectedVersion:        vimtypes.VMX15.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should fail to upgrade vm with InvalidPowerStateFault when fetchProperties is false, correct hardware version is cached, and power state is cached as powered off when vm is powered on",
				testArgs{
					cachedHardwareVersion:  vimtypes.VMX15.String(),
					cachedPowerState:       vimtypes.VirtualMachinePowerStatePoweredOff,
					expectedErrFn:          assertInvalidPowerStateFaultPoweredOn,
					expectedVersion:        vimtypes.VMX15.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOn,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should fail to upgrade vm with InvalidPowerStateFault when fetchProperties is false, correct hardware version is cached, and power state is cached as powered off when vm is suspended",
				testArgs{
					cachedHardwareVersion:  vimtypes.VMX15.String(),
					cachedPowerState:       vimtypes.VirtualMachinePowerStatePoweredOff,
					expectedErrFn:          assertInvalidPowerStateFaultSuspended,
					expectedVersion:        vimtypes.VMX15.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStateSuspended,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should fail to upgrade vm with AlreadyUpgradedFault when fetchProperties is false, hardware version is cached as if not upgraded, and correct power state is cached",
				testArgs{
					cachedHardwareVersion:  vimtypes.VMX15.String(),
					cachedPowerState:       vimtypes.VirtualMachinePowerStatePoweredOff,
					expectedErrFn:          assertAlreadyUpgradedFault,
					expectedVersion:        vimtypes.VMX17.String(),
					initialHardwareVersion: vimtypes.VMX17.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
		)

		DescribeTable(
			"when upgrade is not required",
			doTest,

			Entry(
				"should skip upgrade vm when minHardwareVersion is 0",
				testArgs{
					expectedResult:     vmutil.ReconcileMinHardwareVersionResultMinHardwareVersionZero,
					minHardwareVersion: 0,
				},
			),
			Entry(
				"should skip upgrade vm when current version is same as minHardwareVersion",
				testArgs{
					expectedResult:         vmutil.ReconcileMinHardwareVersionResultAlreadyUpgraded,
					expectedVersion:        vimtypes.VMX17.String(),
					initialHardwareVersion: vimtypes.VMX17.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
			Entry(
				"should skip upgrade vm when current version is greater than minHardwareVersion",
				testArgs{
					expectedResult:         vmutil.ReconcileMinHardwareVersionResultAlreadyUpgraded,
					expectedVersion:        vimtypes.VMX20.String(),
					initialHardwareVersion: vimtypes.VMX20.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
		)

		DescribeTable(
			"when upgrade is required",
			doTest,

			Entry(
				"should upgrade vm when current version is less than minHardwareVersion",
				testArgs{
					expectedResult:         vmutil.ReconcileMinHardwareVersionResultUpgraded,
					expectedVersion:        vimtypes.VMX17.String(),
					initialHardwareVersion: vimtypes.VMX15.String(),
					initialPowerState:      vimtypes.VirtualMachinePowerStatePoweredOff,
					minHardwareVersion:     17,
				},
			),
		)

	})
}
