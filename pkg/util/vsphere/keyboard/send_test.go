// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package keyboard_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/keyboard"
)

type fakeScanCodeSender struct {
	calls []vimtypes.UsbScanCodeSpec
	err   error
}

func (f *fakeScanCodeSender) PutUsbScanCodes(
	_ context.Context, spec vimtypes.UsbScanCodeSpec) (int32, error) {

	if f.err != nil {
		return 0, f.err
	}
	f.calls = append(f.calls, spec)
	return int32(len(spec.KeyEvents)), nil
}

func sendTests() {
	Describe("SendCommands", func() {
		var sender *fakeScanCodeSender

		BeforeEach(func() {
			sender = &fakeScanCodeSender{}
		})

		It("batches consecutive literal characters into one call", func() {
			tokens, err := keyboard.ParseCommands([]string{"ab"}, nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(keyboard.SendCommands(context.Background(), sender, tokens)).To(Succeed())
			Expect(sender.calls).To(HaveLen(1))
			Expect(sender.calls[0].KeyEvents).To(HaveLen(2))
		})

		It("encodes an uppercase literal with the shift modifier", func() {
			tokens, err := keyboard.ParseCommands([]string{"A"}, nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(keyboard.SendCommands(context.Background(), sender, tokens)).To(Succeed())
			Expect(sender.calls).To(HaveLen(1))
			ev := sender.calls[0].KeyEvents[0]
			Expect(ev.UsbHidCode).To(Equal(int32(0x04)<<16 | 7))
			Expect(ev.Modifiers).ToNot(BeNil())
			Expect(ev.Modifiers.LeftShift).To(HaveValue(BeTrue()))
		})

		It("encodes a lowercase literal without a modifier", func() {
			tokens, err := keyboard.ParseCommands([]string{"a"}, nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(keyboard.SendCommands(context.Background(), sender, tokens)).To(Succeed())
			ev := sender.calls[0].KeyEvents[0]
			Expect(ev.UsbHidCode).To(Equal(int32(0x04)<<16 | 7))
			Expect(ev.Modifiers).To(BeNil())
		})

		It("holds a modifier across subsequent keystrokes until released", func() {
			tokens, err := keyboard.ParseCommands([]string{"<leftShiftOn>1<leftShiftOff>2"}, nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(keyboard.SendCommands(context.Background(), sender, tokens)).To(Succeed())
			Expect(sender.calls).To(HaveLen(1))
			Expect(sender.calls[0].KeyEvents).To(HaveLen(2))

			one := sender.calls[0].KeyEvents[0]
			Expect(one.Modifiers).ToNot(BeNil())
			Expect(one.Modifiers.LeftShift).To(HaveValue(BeTrue()))

			two := sender.calls[0].KeyEvents[1]
			Expect(two.Modifiers).To(BeNil())
		})

		It("flushes the pending batch and sleeps on a wait token", func() {
			tokens, err := keyboard.ParseCommands([]string{"a<wait5ms>b"}, nil)
			Expect(err).ToNot(HaveOccurred())

			start := time.Now()
			Expect(keyboard.SendCommands(context.Background(), sender, tokens)).To(Succeed())
			Expect(time.Since(start)).To(BeNumerically(">=", 5*time.Millisecond))

			Expect(sender.calls).To(HaveLen(2))
			Expect(sender.calls[0].KeyEvents).To(HaveLen(1))
			Expect(sender.calls[1].KeyEvents).To(HaveLen(1))
		})

		It("returns an error when PutUsbScanCodes fails", func() {
			sender.err = errors.New("boom")
			tokens, err := keyboard.ParseCommands([]string{"a"}, nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(keyboard.SendCommands(context.Background(), sender, tokens)).To(
				MatchError(ContainSubstring("boom")))
		})

		It("returns the context error when cancelled during a wait", func() {
			tokens, err := keyboard.ParseCommands([]string{"<wait10s>"}, nil)
			Expect(err).ToNot(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()

			Expect(keyboard.SendCommands(ctx, sender, tokens)).To(MatchError(context.Canceled))
		})

		It("returns an error for an unsupported literal character", func() {
			// Verify indirectly: a token stream built with a character
			// outside the supported set surfaces a clear error rather than
			// silently dropping the keystroke.
			tokens := []keyboard.Token{{Kind: keyboard.TokenLiteral, Literal: 'é'}}
			Expect(keyboard.SendCommands(context.Background(), sender, tokens)).To(HaveOccurred())
		})
	})
}
