// +build !integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"

	. "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

func newSecret(name string, ns string, user string, pass string) (*v1.Secret, *VSphereVmProviderCredentials) {
	creds := &VSphereVmProviderCredentials{
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
			clientSet := fake.NewSimpleClientset(secretIn)
			credsOut, err := GetProviderCredentials(clientSet, secretIn.ObjectMeta.Namespace, secretIn.ObjectMeta.Name)
			Expect(credsOut).To(Equal(credsIn))
			Expect(err).To(BeNil())
		})
	})

	Context("when a bad secret exists", func() {

		Context("with empty username", func() {
			Specify("returns no credentials with error", func() {
				secretIn, _ := newSecret("some-name", "some-namespace", "", "some-pass")
				clientSet := fake.NewSimpleClientset(secretIn)
				credsOut, err := GetProviderCredentials(clientSet, secretIn.ObjectMeta.Namespace, secretIn.ObjectMeta.Name)
				Expect(credsOut).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})

		Context("with empty password", func() {
			Specify("returns no credentials with error", func() {
				secretIn, _ := newSecret("some-name", "some-namespace", "some-user", "")
				clientSet := fake.NewSimpleClientset(secretIn)
				credsOut, err := GetProviderCredentials(clientSet, secretIn.ObjectMeta.Namespace, secretIn.ObjectMeta.Name)
				Expect(credsOut).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Context("when no secret exists", func() {
		Specify("returns no credentials with error", func() {
			clientSet := fake.NewSimpleClientset()
			credsOut, err := GetProviderCredentials(clientSet, "none-namespace", "none-name")
			Expect(credsOut).To(BeNil())
			Expect(err).NotTo(BeNil())
		})
	})

})
