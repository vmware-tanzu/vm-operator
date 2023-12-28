// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"crypto/rsa"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("Webconsole Ticket", func() {

	Context("EncryptWebMKS", func() {
		var (
			privateKey   *rsa.PrivateKey
			publicKeyPem string
		)

		BeforeEach(func() {
			privateKey, publicKeyPem = builder.WebConsoleRequestKeyPair()
		})

		It("Encrypts a string correctly", func() {
			plaintext := "HelloWorld2"
			ciphertext, err := virtualmachine.EncryptWebMKS(publicKeyPem, plaintext)
			Expect(err).ShouldNot(HaveOccurred())
			decrypted, err := virtualmachine.DecryptWebMKS(privateKey, ciphertext)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(decrypted).To(Equal(plaintext))
		})

		It("Error on invalid public key", func() {
			plaintext := "HelloWorld3"
			_, err := virtualmachine.EncryptWebMKS("invalid-pub-key", plaintext)
			Expect(err).Should(HaveOccurred())
		})
	})
})
