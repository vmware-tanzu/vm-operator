// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package keyboard

import (
	"context"
	"fmt"
	"time"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

// ScanCodeSender sends USB HID scan codes to a VM's virtual USB keyboard.
// It is satisfied by *github.com/vmware/govmomi/object.VirtualMachine.
type ScanCodeSender interface {
	PutUsbScanCodes(ctx context.Context, spec vimtypes.UsbScanCodeSpec) (int32, error)
}

// SendCommands sends the parsed boot command tokens to vm via its virtual
// USB keyboard. Consecutive non-wait tokens are batched into a single
// PutUsbScanCodes call. TokenWait tokens flush any pending batch and then
// sleep for their duration, honoring ctx cancellation.
func SendCommands(ctx context.Context, vm ScanCodeSender, tokens []Token) error {
	log := pkglog.FromContextOrDefault(ctx)

	var (
		batch         []vimtypes.UsbScanCodeSpecKeyEvent
		heldModifiers *vimtypes.UsbScanCodeSpecModifierType
	)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}

		n, err := vm.PutUsbScanCodes(ctx, vimtypes.UsbScanCodeSpec{KeyEvents: batch})
		if err != nil {
			log.Error(err, "Failed to send USB scan codes")
			return fmt.Errorf("failed to send USB scan codes: %w", err)
		}
		log.V(5).Info("Sent USB scan codes", "count", n)

		batch = nil
		return nil
	}

	for i, t := range tokens {
		switch t.Kind {
		case TokenLiteral:
			ev, err := literalKeyEvent(t.Literal, heldModifiers)
			if err != nil {
				return fmt.Errorf("boot command token %d: %w", i, err)
			}
			batch = append(batch, ev)

		case TokenSpecialKey:
			ev, err := specialKeyEvent(t.Special, heldModifiers)
			if err != nil {
				return fmt.Errorf("boot command token %d: %w", i, err)
			}
			batch = append(batch, ev)

		case TokenModifierOn, TokenModifierOff:
			if heldModifiers == nil {
				heldModifiers = &vimtypes.UsbScanCodeSpecModifierType{}
			}
			if err := applyModifier(heldModifiers, t.Modifier, t.Kind == TokenModifierOn); err != nil {
				return fmt.Errorf("boot command token %d: %w", i, err)
			}

		case TokenWait:
			if err := flush(); err != nil {
				return err
			}
			if err := sleep(ctx, t.Wait); err != nil {
				return err
			}

		default:
			return fmt.Errorf("boot command token %d: unknown token kind %d", i, t.Kind)
		}
	}

	return flush()
}

// sleep waits for d, returning early with ctx's error if ctx is done first.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
