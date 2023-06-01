// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm/internal"
)

// DefaultTrySoftTimeout is the default amount of time a TrySoft operation
// waits for a desired power state to be realized before switching to a Hard
// operation.
const DefaultTrySoftTimeout = 5 * time.Minute

// PowerOpBehavior indicates the three behaviors for powering off or suspending
// a VM.
type PowerOpBehavior uint8

const (
	// PowerOpBehaviorHard causes a VM to be powered off with the PowerOff API
	// or to be suspended with the Suspend API.
	PowerOpBehaviorHard PowerOpBehavior = iota + 1

	// PowerOpBehaviorSoft causes a VM to be powered off with the ShutdownGuest
	// API or to be suspended with the StandbyGuest API.
	PowerOpBehaviorSoft

	// PowerOpBehaviorTrySoft causes an attempt to use the soft behavior, and
	// if the VM is not in the desired power state after 10 minutes, the next
	// attempt falls back to the hard behavior.
	PowerOpBehaviorTrySoft
)

// String returns the stringified name of the behavior.
func (b PowerOpBehavior) String() string {
	switch b {
	case PowerOpBehaviorHard:
		return "Hard"
	case PowerOpBehaviorSoft:
		return "Soft"
	case PowerOpBehaviorTrySoft:
		return "TrySoft"
	default:
		return ""
	}
}

// ErrInvalidSetPowerStateResult is returned if the channel returned by
// SetPowerState receives a value that is a type other than SetPowerStateResult
// or error.
type ErrInvalidSetPowerStateResult struct {
	SetPowerStateResult any
}

// Error enables this type to be returned as a Golang error object.
func (e ErrInvalidSetPowerStateResult) Error() string {
	return fmt.Sprintf("invalid result: %q", e.SetPowerStateResult)
}

// ErrInvalidPowerState is returned if a power state does not map to
// "poweredOff", "poweredOn", or "suspended".
type ErrInvalidPowerState struct {
	PowerState types.VirtualMachinePowerState
}

// Error enables this type to be returned as a Golang error object.
func (e ErrInvalidPowerState) Error() string {
	return fmt.Sprintf("invalid power state: %q", e.PowerState)
}

// ErrInvalidPowerOpBehavior is returned if a power op behavior is not one of
// the pre-defined constants that map to PowerOpBehavior in this package.
type ErrInvalidPowerOpBehavior struct {
	PowerOpBehavior PowerOpBehavior
}

// Error enables this type to be returned as a Golang error object.
func (e ErrInvalidPowerOpBehavior) Error() string {
	return fmt.Sprintf("invalid power op behavior: %q", e.PowerOpBehavior)
}

// SetPowerStateResult represents one of the possible results when calling
// SetPowerState or SetAndWaitOnPowerState.
type SetPowerStateResult uint8

const (
	// SetPowerStateResultNone indicates the power state was not changed
	// because the VM's power state matched the desired power state.
	SetPowerStateResultNone SetPowerStateResult = iota

	// SetPowerStateResultChanged indicates the power state was changed.
	// This is set if the VM was powered on, off, or suspended.
	SetPowerStateResultChanged

	// SetPowerStateResultChangedHard indicates the power state was changed
	// with a hard operation. This may only be set during a power off or
	// suspend operation.
	SetPowerStateResultChangedHard

	// SetPowerStateResultChangedHard indicates the power state was changed
	// with a soft operation. This may only be set during a power off or
	// suspend operation.
	SetPowerStateResultChangedSoft
)

// AnyChange returns true if the result indicates a change, whether that is a
// power on operation or a hard or soft power off/suspend operation.
func (r SetPowerStateResult) AnyChange() bool {
	return r >= SetPowerStateResultChanged
}

// SetPowerState updates a VM's power state if the power state in the
// provided, managed object is missing or does not match the desired power
// state.
// If fetchCurrentPowerState is true, then even if already known, the power
// state is retrieved from the server.
// Either a SetPowerStateResult or error object will be sent on the returned
// channel prior to its closure.
func SetPowerState(
	ctx context.Context,
	client *vim25.Client,
	mo mo.VirtualMachine,
	fetchCurrentPowerState bool,
	desiredPowerState types.VirtualMachinePowerState,
	powerOpBehavior PowerOpBehavior) <-chan any {

	chanResult := make(chan any, 1)

	go func() {
		result, err := setPowerState(
			ctx,
			client,
			mo,
			fetchCurrentPowerState,
			desiredPowerState,
			powerOpBehavior)
		if err != nil {
			chanResult <- err
		} else {
			chanResult <- result
		}
		close(chanResult)
	}()

	return chanResult
}

// SetAndWaitOnPowerState is the same as SetPowerState but blocks until an
// error occurs or the operation completes successfully.
func SetAndWaitOnPowerState(
	ctx context.Context,
	client *vim25.Client,
	mo mo.VirtualMachine,
	fetchCurrentPowerState bool,
	desiredPowerState types.VirtualMachinePowerState,
	powerOpBehavior PowerOpBehavior) (SetPowerStateResult, error) {

	chanResult := SetPowerState(
		ctx,
		client,
		mo,
		fetchCurrentPowerState,
		desiredPowerState,
		powerOpBehavior)

	result := <-chanResult

	switch tResult := result.(type) {
	case SetPowerStateResult:
		return tResult, nil
	case error:
		return 0, tResult
	}

	return 0, ErrInvalidSetPowerStateResult{SetPowerStateResult: result}
}

func setPowerState(
	ctx context.Context,
	client *vim25.Client,
	mo mo.VirtualMachine,
	fetchCurrentPowerState bool,
	desiredPowerState types.VirtualMachinePowerState,
	powerOpBehavior PowerOpBehavior) (SetPowerStateResult, error) {

	var (
		currentPowerState types.VirtualMachinePowerState
		log               = logr.FromContextOrDiscard(ctx)
		obj               = object.NewVirtualMachine(client, mo.Self)
	)

	// Fetch the VM's current power state if it is not already known or if
	// explicitly requested.
	if mo.Summary.Runtime.PowerState == "" || fetchCurrentPowerState {
		if err := obj.Properties(
			ctx,
			mo.Self,
			[]string{object.PropRuntimePowerState},
			&mo); err != nil {
			return 0, fmt.Errorf("failed to retrieve power state %w", err)
		}
	}
	currentPowerState = mo.Summary.Runtime.PowerState

	if currentPowerState == desiredPowerState {
		log.V(5).Info(
			"power state already set",
			"currentPowerState", currentPowerState,
			"desiredPowerState", desiredPowerState,
			"desiredPowerOpBehavior", PowerOpBehaviorHard.String())
		return SetPowerStateResultNone, nil
	}

	log.V(5).Info(
		"setting power state",
		"currentPowerState", currentPowerState,
		"desiredPowerState", desiredPowerState,
		"desiredPowerOpBehavior", powerOpBehavior.String())

	var (
		powerOpHardFn       func(context.Context) (*object.Task, error)
		powerOpSoftFn       func(context.Context) error
		waitForPowerStateFn = obj.WaitForPowerState
	)

	switch desiredPowerState {
	case types.VirtualMachinePowerStatePoweredOn:
		powerOpHardFn = obj.PowerOn
	case types.VirtualMachinePowerStatePoweredOff:
		switch powerOpBehavior {
		case PowerOpBehaviorHard:
			powerOpHardFn = obj.PowerOff
		case PowerOpBehaviorSoft:
			powerOpSoftFn = obj.ShutdownGuest
		case PowerOpBehaviorTrySoft:
			powerOpSoftFn = obj.ShutdownGuest
			powerOpHardFn = obj.PowerOff
		default:
			return 0, ErrInvalidPowerOpBehavior{PowerOpBehavior: powerOpBehavior}
		}
	case types.VirtualMachinePowerStateSuspended:
		switch powerOpBehavior {
		case PowerOpBehaviorHard:
			powerOpHardFn = obj.Suspend
		case PowerOpBehaviorSoft:
			powerOpSoftFn = obj.StandbyGuest
		case PowerOpBehaviorTrySoft:
			powerOpHardFn = obj.Suspend
			powerOpSoftFn = obj.StandbyGuest
		default:
			return 0, ErrInvalidPowerOpBehavior{PowerOpBehavior: powerOpBehavior}
		}
	default:
		return 0, ErrInvalidPowerState{PowerState: desiredPowerState}
	}

	switch {
	case powerOpHardFn != nil && powerOpSoftFn == nil: // hard
		log.V(5).Info("hard power op")
		return doAndWaitOnHardPowerOp(ctx, desiredPowerState, powerOpHardFn)
	case powerOpHardFn == nil && powerOpSoftFn != nil: // soft
		log.V(5).Info("soft power op")
		return doAndWaitOnSoftPowerOp(ctx, desiredPowerState, powerOpSoftFn, waitForPowerStateFn)
	case powerOpHardFn != nil && powerOpSoftFn != nil: // trySoft + hard
		log.V(5).Info("try soft power op")
		result, err := doAndWaitOnSoftPowerOp(
			ctx,
			desiredPowerState,
			powerOpSoftFn,
			waitForPowerStateFn)
		if err != nil {
			log.V(5).Error(
				err,
				"soft power op failed, trying hard power op",
				"currentPowerState", currentPowerState,
				"desiredPowerState", desiredPowerState,
				"desiredPowerOpBehavior", PowerOpBehaviorHard.String())
			result, err = doAndWaitOnHardPowerOp(
				ctx,
				desiredPowerState,
				powerOpHardFn)
		}
		return result, err
	default:
		return 0, errors.New("missing hard and soft power op functions")
	}
}

func doAndWaitOnHardPowerOp(
	ctx context.Context,
	desiredPowerState types.VirtualMachinePowerState,
	powerOpFn func(context.Context) (*object.Task, error)) (SetPowerStateResult, error) {

	log := logr.FromContextOrDiscard(ctx)

	t, err := powerOpFn(ctx)
	if err != nil {
		return 0, fmt.Errorf(
			"failed to invoke hard power op for %s %w", desiredPowerState, err)
	}
	if ti, err := t.WaitForResult(ctx); err != nil {
		if err, ok := err.(task.Error); ok {
			// Ignore error if desired power state already set.
			if ips, ok := err.Fault().(*types.InvalidPowerState); ok && ips.ExistingState == ips.RequestedState {
				log.V(5).Info(
					"power state already set",
					"desiredPowerState",
					desiredPowerState)
				return SetPowerStateResultNone, nil
			}
		}
		if ti != nil {
			log.V(5).Error(err, "Change power state task failed", "taskInfo", ti)
		}
		return 0, fmt.Errorf("hard set power state to %s failed %w", desiredPowerState, err)
	}

	switch desiredPowerState {
	case types.VirtualMachinePowerStatePoweredOn:
		return SetPowerStateResultChanged, nil
	case types.VirtualMachinePowerStatePoweredOff, types.VirtualMachinePowerStateSuspended:
		return SetPowerStateResultChangedHard, nil
	}

	return 0, ErrInvalidPowerState{PowerState: desiredPowerState}
}

func doAndWaitOnSoftPowerOp(
	ctx context.Context,
	desiredPowerState types.VirtualMachinePowerState,
	powerOpFn func(context.Context) error,
	waitForPowerStateFn func(context.Context, types.VirtualMachinePowerState) error) (SetPowerStateResult, error) {

	if err := powerOpFn(ctx); err != nil {
		return 0, fmt.Errorf("soft set power state to %s failed %w", desiredPowerState, err)
	}

	// Attempt to get a timeout from the context.
	timeout, ok := ctx.Value(internal.SoftTimeoutKey).(time.Duration)
	if !ok {
		timeout = DefaultTrySoftTimeout
	}

	log := logr.FromContextOrDiscard(ctx)
	log.V(5).Info(
		"creating context with timeout waiting for power state after soft power op",
		"desiredPowerState", desiredPowerState,
		"timeout", timeout.String())

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := waitForPowerStateFn(ctx, desiredPowerState); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return 0, fmt.Errorf("timed out waiting for power state %s", desiredPowerState)
		}
		return 0, fmt.Errorf("failed to wait for power state %s %w", desiredPowerState, err)
	}
	return SetPowerStateResultChangedSoft, nil
}

// ParsePowerOpMode parses a VM Op API PowerOpMode and returns the corresponding
// PowerOpBehavior.
func ParsePowerOpMode(m string) PowerOpBehavior {
	switch strings.ToLower(m) {
	case strings.ToLower(PowerOpBehaviorHard.String()):
		return PowerOpBehaviorHard
	case strings.ToLower(PowerOpBehaviorSoft.String()):
		return PowerOpBehaviorSoft
	case strings.ToLower(PowerOpBehaviorTrySoft.String()):
		return PowerOpBehaviorTrySoft
	default:
		return 0
	}
}

// ParsePowerState parses a VM Op API PowerState and returns the corresponding
// vim25 value.
func ParsePowerState(m string) types.VirtualMachinePowerState {
	switch strings.ToLower(m) {
	case strings.ToLower(string(types.VirtualMachinePowerStatePoweredOff)):
		return types.VirtualMachinePowerStatePoweredOff
	case strings.ToLower(string(types.VirtualMachinePowerStatePoweredOn)):
		return types.VirtualMachinePowerStatePoweredOn
	case strings.ToLower(string(types.VirtualMachinePowerStateSuspended)):
		return types.VirtualMachinePowerStateSuspended
	default:
		return ""
	}
}
