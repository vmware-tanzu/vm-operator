// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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

// ErrInvalidPowerOpResult is returned if the channel returned by
// SetPowerState and/or Restart receives a value that is a type other than
// PowerOpResult or error.
type ErrInvalidPowerOpResult struct {
	PowerOpResult any
}

// Error enables this type to be returned as a Golang error object.
func (e ErrInvalidPowerOpResult) Error() string {
	return fmt.Sprintf("invalid result: %q", e.PowerOpResult)
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

// PowerOpResult represents one of the possible results when calling
// SetPowerState or SetAndWaitOnPowerState.
type PowerOpResult uint8

const (
	// PowerOpResultNone indicates the power state was not changed
	// because the VM's power state matched the desired power state.
	PowerOpResultNone PowerOpResult = iota

	// PowerOpResultChanged indicates the power state was changed.
	// This is set if the VM was powered on.
	PowerOpResultChanged

	// PowerOpResultChangedHard indicates the power state was changed
	// with a hard operation. This is set if the VM was powered off,
	// suspended, or restarted.
	PowerOpResultChangedHard

	// PowerOpResultChangedSoft indicates the power state was changed
	// with a soft operation. This is set if the VM was powered off,
	// suspended, or restarted.
	PowerOpResultChangedSoft
)

// AnyChange returns true if the result indicates a change, whether that is a
// power on operation or a hard or soft power off/suspend operation.
func (r PowerOpResult) AnyChange() bool {
	return r >= PowerOpResultChanged
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

// SetPowerState updates a VM's power state if the power state in the
// provided, managed object is missing or does not match the desired power
// state.
// If fetchProperties is true, then even if already known, the required
// properties are fetched from vSphere.
// Either a PowerOpResult or error object will be sent on the returned
// channel prior to its closure.
func SetPowerState(
	ctx context.Context,
	client *vim25.Client,
	mo mo.VirtualMachine,
	fetchProperties bool,
	desiredPowerState types.VirtualMachinePowerState,
	powerOpBehavior PowerOpBehavior) <-chan any {

	chanResult := make(chan any, 1)

	go func() {
		result, err := setPowerState(
			ctx,
			client,
			mo,
			fetchProperties,
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
	fetchProperties bool,
	desiredPowerState types.VirtualMachinePowerState,
	powerOpBehavior PowerOpBehavior) (PowerOpResult, error) {

	return setPowerState(
		ctx,
		client,
		mo,
		fetchProperties,
		desiredPowerState,
		powerOpBehavior)
}

func setPowerState(
	ctx context.Context,
	client *vim25.Client,
	mo mo.VirtualMachine,
	fetchProperties bool,
	desiredPowerState types.VirtualMachinePowerState,
	powerOpBehavior PowerOpBehavior) (PowerOpResult, error) {

	var (
		currentPowerState types.VirtualMachinePowerState
		log               = logr.FromContextOrDiscard(ctx)
		obj               = object.NewVirtualMachine(client, mo.Self)
	)

	// Fetch the VM's current power state if it is not already known or if
	// explicitly requested.
	if mo.Summary.Runtime.PowerState == "" || fetchProperties {
		if err := obj.Properties(
			ctx,
			mo.Self,
			[]string{object.PropRuntimePowerState},
			&mo); err != nil {
			return 0, fmt.Errorf("failed to retrieve properties %w", err)
		}
	}
	currentPowerState = mo.Summary.Runtime.PowerState

	log = log.WithValues(
		"currentPowerState", currentPowerState,
		"desiredPowerState", desiredPowerState,
		"desiredPowerOpBehavior", powerOpBehavior.String())

	if currentPowerState == desiredPowerState {
		log.Info("Power state already set")
		return PowerOpResultNone, nil
	}

	var (
		powerOpHardFn       func(context.Context) (*object.Task, error)
		powerOpSoftFn       func(context.Context) error
		waitForPowerStateFn = obj.WaitForPowerState
	)

	switch desiredPowerState {
	case types.VirtualMachinePowerStatePoweredOn:
		powerOpHardFn = obj.PowerOn
		log = log.WithValues("powerOpHardFn", "PowerOn")
	case types.VirtualMachinePowerStatePoweredOff:
		switch powerOpBehavior {
		case PowerOpBehaviorHard:
			powerOpHardFn = obj.PowerOff
			log = log.WithValues("powerOpHardFn", "PowerOff")
		case PowerOpBehaviorSoft:
			powerOpSoftFn = obj.ShutdownGuest
			log = log.WithValues("powerOpSoftFn", "ShutdownGuest")
		case PowerOpBehaviorTrySoft:
			powerOpHardFn = obj.PowerOff
			powerOpSoftFn = obj.ShutdownGuest
			log = log.WithValues(
				"powerOpHardFn", "PowerOff",
				"powerOpSoftFn", "ShutdownGuest")
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
		log.Info("Hard power op")
		return doAndWaitOnHardPowerOp(ctx, desiredPowerState, powerOpHardFn)
	case powerOpHardFn == nil && powerOpSoftFn != nil: // soft
		log.Info("Soft power op")
		return doAndWaitOnSoftPowerOp(ctx, desiredPowerState, powerOpSoftFn, waitForPowerStateFn)
	case powerOpHardFn != nil && powerOpSoftFn != nil: // trySoft + hard
		log.Info("Try soft power op")
		result, err := doAndWaitOnSoftPowerOp(
			ctx,
			desiredPowerState,
			powerOpSoftFn,
			waitForPowerStateFn)
		if err != nil {
			log.Error(err, "Soft power op failed, trying hard power op")
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
	powerOpFn func(context.Context) (*object.Task, error)) (PowerOpResult, error) {

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
				log.Info(
					"Power state already set",
					"desiredPowerState",
					desiredPowerState)
				return PowerOpResultNone, nil
			}
		}
		if ti != nil {
			log.Error(err, "Change power state task failed", "taskInfo", ti)
		}
		return 0, fmt.Errorf("hard set power state to %s failed %w", desiredPowerState, err)
	}

	switch desiredPowerState {
	case types.VirtualMachinePowerStatePoweredOn:
		return PowerOpResultChanged, nil
	case types.VirtualMachinePowerStatePoweredOff, types.VirtualMachinePowerStateSuspended:
		return PowerOpResultChangedHard, nil
	}

	return 0, ErrInvalidPowerState{PowerState: desiredPowerState}
}

func doAndWaitOnSoftPowerOp(
	ctx context.Context,
	desiredPowerState types.VirtualMachinePowerState,
	powerOpFn func(context.Context) error,
	waitForPowerStateFn func(context.Context, types.VirtualMachinePowerState) error) (PowerOpResult, error) {

	if err := powerOpFn(ctx); err != nil {
		return 0, fmt.Errorf("soft set power state to %s failed %w", desiredPowerState, err)
	}

	// Attempt to get a timeout from the context.
	timeout, ok := ctx.Value(internal.SoftTimeoutKey).(time.Duration)
	if !ok {
		timeout = DefaultTrySoftTimeout
	}

	log := logr.FromContextOrDiscard(ctx)
	log.Info(
		"Creating context with timeout waiting for power state after soft power op",
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
	return PowerOpResultChangedSoft, nil
}

// Restart restarts a VM if the provided lastRestart timestamp occurs after
// the last time the VM was restarted.
// If fetchProperties is true, then even if already known, the required
// properties are fetched from vSphere.
// Either a PowerOpResult or error object will be sent on the returned
// channel prior to its closure.
func Restart(
	ctx context.Context,
	client *vim25.Client,
	mo mo.VirtualMachine,
	fetchProperties bool,
	desiredLastRestartTime time.Time,
	powerOpBehavior PowerOpBehavior) <-chan any {

	chanResult := make(chan any, 1)

	go func() {
		result, err := restart(
			ctx,
			client,
			mo,
			fetchProperties,
			desiredLastRestartTime,
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

// RestartAndWait is the same as Restart but blocks until an error occurs or the
// operation completes successfully.
func RestartAndWait(
	ctx context.Context,
	client *vim25.Client,
	mo mo.VirtualMachine,
	fetchProperties bool,
	desiredLastRestartTime time.Time,
	powerOpBehavior PowerOpBehavior) (PowerOpResult, error) {

	return restart(
		ctx,
		client,
		mo,
		fetchProperties,
		desiredLastRestartTime,
		powerOpBehavior)
}

const (
	propConfigExtraConfig = "config.extraConfig"

	// ExtraConfigKeyLastRestartTime is the name of the key in a VM's
	// ExtraConfig array that contains the epoch of the last time the VM was
	// restarted.
	ExtraConfigKeyLastRestartTime = "vmservice.lastRestartTime"
)

func restart(
	ctx context.Context,
	client *vim25.Client,
	mo mo.VirtualMachine,
	fetchProperties bool,
	desiredLastRestartTime time.Time,
	powerOpBehavior PowerOpBehavior) (PowerOpResult, error) {

	var (
		err                          error
		currentPowerState            types.VirtualMachinePowerState
		currentLastRestartTime       *time.Time
		currentLastRestartTimeString string
		propsToFetch                 []string
		result                       PowerOpResult
		log                          = logr.FromContextOrDiscard(ctx)
		obj                          = object.NewVirtualMachine(client, mo.Self)
		desiredLastRestartTimeString = desiredLastRestartTime.Format(time.RFC3339Nano)
	)

	// Fetch the VM's current power state and extra config if they are not
	// already known or explicitly requested.
	if fetchProperties || mo.Summary.Runtime.PowerState == "" {
		propsToFetch = append(propsToFetch, object.PropRuntimePowerState)
	}
	if fetchProperties || mo.Config == nil || len(mo.Config.ExtraConfig) == 0 {
		propsToFetch = append(propsToFetch, propConfigExtraConfig)
	}
	if len(propsToFetch) > 0 {
		if err := obj.Properties(
			ctx,
			mo.Self,
			propsToFetch,
			&mo); err != nil {
			return 0, fmt.Errorf("failed to retrieve properties %w", err)
		}
	}
	if mo.Config == nil {
		return 0, errors.New("nil vm.config")
	}
	currentPowerState = mo.Summary.Runtime.PowerState

	log = log.WithValues(
		"currentPowerState", currentPowerState,
		"desiredLastRestartTime", desiredLastRestartTimeString,
		"desiredPowerOpBehavior", powerOpBehavior.String())

	if currentPowerState != types.VirtualMachinePowerStatePoweredOn {
		return 0, ErrInvalidPowerState{PowerState: currentPowerState}
	}

	// Find the currentLastRestartTime as recorded in the extra config.
	currentLastRestartTime, err = GetLastRestartTimeFromExtraConfig(ctx, mo.Config.ExtraConfig)
	if err != nil {
		return 0, err
	}
	if currentLastRestartTime != nil {
		currentLastRestartTimeString = currentLastRestartTime.Format(time.RFC3339Nano)

		log = log.WithValues("currentLastRestartTime", currentLastRestartTimeString)

		// Do not restart unless desiredLastRestartTime is newer than
		// currentLastRestartTime.
		if desiredLastRestartTime.UnixNano() <= currentLastRestartTime.UnixNano() {
			log.Info("Will not restart VM as desiredLastRestartTime <= currentLastRestartTime")
			return PowerOpResultNone, nil
		}
	}

	// Do not restart if desiredLastRestartTime is in the future.
	if desiredLastRestartTime.After(time.Now().UTC()) {
		log.Info("Will not restart VM as desiredLastRestartTime is in the future")
		return PowerOpResultNone, nil
	}

	switch powerOpBehavior {
	case PowerOpBehaviorHard:
		log.Info("Hard restart")
		result, err = doAndWaitOnHardRestart(ctx, obj.Reset)
	case PowerOpBehaviorSoft:
		log.Info("Soft restart")
		result, err = doAndWaitOnSoftRestart(ctx, obj.RebootGuest)
	case PowerOpBehaviorTrySoft:
		log.Info("Try soft restart")
		if result, err = doAndWaitOnSoftRestart(ctx, obj.RebootGuest); err != nil {
			log.Error(err, "soft restart failed, trying hard restart")
			result, err = doAndWaitOnHardRestart(ctx, obj.Reset)
		}
	default:
		return 0, ErrInvalidPowerOpBehavior{PowerOpBehavior: powerOpBehavior}
	}

	if err != nil {
		return 0, err
	}

	if result.AnyChange() {
		// Recording the new lastRestartTime after the restart *can* result in
		// extra restarts. However, it is better to restart more than once than
		// not at all, as would be possible if the lastRestartTime was recorded
		// before the restart, and the restart operation failed.
		if err := setLastRestartTimeInExtraConfigAndWait(
			ctx, obj, desiredLastRestartTime); err != nil {
			return result, err
		}
	}

	return result, nil
}

func doAndWaitOnHardRestart(
	ctx context.Context,
	powerOpFn func(context.Context) (*object.Task, error)) (PowerOpResult, error) {

	log := logr.FromContextOrDiscard(ctx)

	t, err := powerOpFn(ctx)
	if err != nil {
		return 0, fmt.Errorf(
			"failed to hard restart vm %w", err)
	}
	if ti, err := t.WaitForResult(ctx); err != nil {
		if ti != nil {
			log.Error(err, "Failed to hard restart VM", "taskInfo", ti)
		}
		return 0, fmt.Errorf("hard restart failed %w", err)
	}

	return PowerOpResultChangedHard, nil
}

func doAndWaitOnSoftRestart(
	ctx context.Context,
	powerOpFn func(context.Context) error) (PowerOpResult, error) {

	log := logr.FromContextOrDiscard(ctx)

	// Attempt to get a timeout from the context.
	timeout, ok := ctx.Value(internal.SoftTimeoutKey).(time.Duration)
	if !ok {
		timeout = DefaultTrySoftTimeout
	}

	log.Info(
		"Creating context with timeout for soft restart",
		"timeout", timeout.String())

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := powerOpFn(ctx); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return 0, errors.New("timed out while soft restarting vm")
		}
		return 0, fmt.Errorf("failed to soft restart vm %w", err)
	}
	return PowerOpResultChangedSoft, nil
}

// GetLastRestartTimeFromExtraConfig inspects the provided ExtraConfig for the
// key "vmservice.lastRestartTime" and returns its value. If not found, nil
// is returned. If the key exists but the value is not an epoch, then an error
// is returned.
func GetLastRestartTimeFromExtraConfig(
	ctx context.Context,
	extraConfig []types.BaseOptionValue) (*time.Time, error) {

	log := logr.FromContextOrDiscard(ctx)

	for i := range extraConfig {
		bov := extraConfig[i]
		if bov == nil {
			continue
		}
		ov := bov.GetOptionValue()
		if ov == nil {
			continue
		}
		if ov.Key != ExtraConfigKeyLastRestartTime {
			continue
		}
		var epoch int64
		switch tval := ov.Value.(type) {
		case int:
			epoch = int64(tval)
		case int64:
			epoch = tval
		case float64:
			epoch = int64(tval)
		case string:
			i64, err := strconv.ParseInt(tval, 10, 64)
			if err != nil {
				return nil, fmt.Errorf(
					"%s is %q and cannot be parsed as an int64",
					ExtraConfigKeyLastRestartTime,
					tval)
			}
			epoch = i64
		}
		if epoch == 0 {
			return nil, nil
		}
		t := time.Unix(0, epoch).UTC()

		log.Info(
			"Got recorded lastRestartTime from extra config",
			"extraConfigKey", ExtraConfigKeyLastRestartTime,
			"extraConfigValue", epoch)

		return &t, nil
	}

	return nil, nil
}

func setLastRestartTimeInExtraConfigAndWait(
	ctx context.Context,
	obj *object.VirtualMachine,
	lastRestartTime time.Time) error {

	var (
		log                  = logr.FromContextOrDiscard(ctx)
		lastRestartTimeEpoch = lastRestartTime.UnixNano()
	)

	log.Info("Recording lastRestartTime to vm",
		"extraConfigKey", ExtraConfigKeyLastRestartTime,
		"extraConfigValue", lastRestartTimeEpoch)

	t, err := obj.Reconfigure(ctx, types.VirtualMachineConfigSpec{
		ExtraConfig: []types.BaseOptionValue{
			&types.OptionValue{
				Key:   ExtraConfigKeyLastRestartTime,
				Value: strconv.FormatInt(lastRestartTimeEpoch, 10),
			},
		}})
	if err != nil {
		return fmt.Errorf(
			"failed to invoke reconfigure that records extra config key=%s value=%v to vm %w",
			ExtraConfigKeyLastRestartTime,
			lastRestartTimeEpoch,
			err)
	}

	if err := t.Wait(ctx); err != nil {
		return fmt.Errorf(
			"failed to record extra config key=%s value=%v to vm %w",
			ExtraConfigKeyLastRestartTime,
			lastRestartTimeEpoch,
			err)
	}

	return nil
}
