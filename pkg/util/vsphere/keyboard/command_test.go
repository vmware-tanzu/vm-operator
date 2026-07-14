// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package keyboard_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/keyboard"
)

func commandTests() {
	Describe("ParseCommands", func() {
		It("tokenizes literal characters one at a time", func() {
			tokens, err := keyboard.ParseCommands([]string{"ab"}, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(tokens).To(Equal([]keyboard.Token{
				{Kind: keyboard.TokenLiteral, Literal: 'a'},
				{Kind: keyboard.TokenLiteral, Literal: 'b'},
			}))
		})

		It("tokenizes a special key", func() {
			tokens, err := keyboard.ParseCommands([]string{"<esc>"}, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(tokens).To(Equal([]keyboard.Token{
				{Kind: keyboard.TokenSpecialKey, Special: "esc"},
			}))
		})

		It("is case-insensitive for special keys", func() {
			tokens, err := keyboard.ParseCommands([]string{"<ESC>"}, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(tokens).To(Equal([]keyboard.Token{
				{Kind: keyboard.TokenSpecialKey, Special: "esc"},
			}))
		})

		It("tokenizes <wait> as 1 second", func() {
			tokens, err := keyboard.ParseCommands([]string{"<wait>"}, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(tokens).To(Equal([]keyboard.Token{
				{Kind: keyboard.TokenWait, Wait: time.Second},
			}))
		})

		It("tokenizes <wait5> as 5 seconds", func() {
			tokens, err := keyboard.ParseCommands([]string{"<wait5>"}, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(tokens).To(Equal([]keyboard.Token{
				{Kind: keyboard.TokenWait, Wait: 5 * time.Second},
			}))
		})

		It("tokenizes <wait3m30s> as a Go duration", func() {
			tokens, err := keyboard.ParseCommands([]string{"<wait3m30s>"}, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(tokens).To(Equal([]keyboard.Token{
				{Kind: keyboard.TokenWait, Wait: 3*time.Minute + 30*time.Second},
			}))
		})

		It("returns an error for an invalid wait duration", func() {
			_, err := keyboard.ParseCommands([]string{"<waitbogus>"}, nil)
			Expect(err).To(HaveOccurred())
		})

		It("tokenizes modifier hold/release", func() {
			tokens, err := keyboard.ParseCommands([]string{"<leftShiftOn>1<leftShiftOff>"}, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(tokens).To(Equal([]keyboard.Token{
				{Kind: keyboard.TokenModifierOn, Modifier: "leftshift"},
				{Kind: keyboard.TokenLiteral, Literal: '1'},
				{Kind: keyboard.TokenModifierOff, Modifier: "leftshift"},
			}))
		})

		It("returns an error for an unknown token", func() {
			_, err := keyboard.ParseCommands([]string{"<bogus>"}, nil)
			Expect(err).To(HaveOccurred())
		})

		It("renders each command before tokenizing", func() {
			render := func(s string) string {
				Expect(s).To(Equal("{{.Foo}}"))
				return "<esc>"
			}
			tokens, err := keyboard.ParseCommands([]string{"{{.Foo}}"}, render)
			Expect(err).ToNot(HaveOccurred())
			Expect(tokens).To(Equal([]keyboard.Token{
				{Kind: keyboard.TokenSpecialKey, Special: "esc"},
			}))
		})

		It("flattens tokens across multiple commands in order", func() {
			tokens, err := keyboard.ParseCommands([]string{"a", "<enter>", "b"}, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(tokens).To(Equal([]keyboard.Token{
				{Kind: keyboard.TokenLiteral, Literal: 'a'},
				{Kind: keyboard.TokenSpecialKey, Special: "enter"},
				{Kind: keyboard.TokenLiteral, Literal: 'b'},
			}))
		})
	})

	Describe("TotalWait", func() {
		It("sums every wait token's duration", func() {
			tokens, err := keyboard.ParseCommands([]string{"<wait3s>a<wait5>"}, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(keyboard.TotalWait(tokens)).To(Equal(8 * time.Second))
		})

		It("returns zero when there are no wait tokens", func() {
			tokens, err := keyboard.ParseCommands([]string{"abc"}, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(keyboard.TotalWait(tokens)).To(Equal(time.Duration(0)))
		})
	})
}
