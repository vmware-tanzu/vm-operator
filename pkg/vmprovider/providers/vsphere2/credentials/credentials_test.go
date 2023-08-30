// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package credentials_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	. "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/credentials"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func newSecret(name string, ns string, user string, pass string) (*corev1.Secret, *VSphereVMProviderCredentials) {
	creds := &VSphereVMProviderCredentials{
		Username: user,
		Password: pass,
	}
	secret := ProviderCredentialsToSecret(ns, creds, name)
	return secret, creds
}

var _ = Describe("GetProviderCredentials", func() {

	Context("when a good secret exists", func() {
		Specify("returns good credentials with no error", func() {
			secretIn, credsIn := newSecret("some-name", "some-namespace", "some-user", "some-pass")
			client := builder.NewFakeClient(secretIn)
			credsOut, err := GetProviderCredentials(client, secretIn.Namespace, secretIn.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(credsOut).To(Equal(credsIn))
		})
	})

	Context("when a bad secret exists", func() {

		Context("with empty username", func() {
			Specify("returns no credentials with error", func() {
				secretIn, _ := newSecret("some-name", "some-namespace", "", "some-pass")
				client := builder.NewFakeClient(secretIn)
				credsOut, err := GetProviderCredentials(client, secretIn.Namespace, secretIn.Name)
				Expect(err).To(HaveOccurred())
				Expect(credsOut).To(BeNil())
			})
		})

		Context("with empty password", func() {
			Specify("returns no credentials with error", func() {
				secretIn, _ := newSecret("some-name", "some-namespace", "some-user", "")
				client := builder.NewFakeClient(secretIn)
				credsOut, err := GetProviderCredentials(client, secretIn.Namespace, secretIn.Name)
				Expect(err).To(HaveOccurred())
				Expect(credsOut).To(BeNil())
			})
		})
	})

	Context("when no secret exists", func() {
		Specify("returns no credentials with error", func() {
			client := builder.NewFakeClient()
			credsOut, err := GetProviderCredentials(client, "none-namespace", "none-name")
			Expect(err).To(HaveOccurred())
			Expect(credsOut).To(BeNil())
		})
	})
})
