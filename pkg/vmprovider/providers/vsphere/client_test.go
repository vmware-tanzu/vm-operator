// +build !integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"net/url"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

func testConfig(vcpnid string, vcport string, user string, pass string) *VSphereVmProviderConfig {
	providerConfig := &VSphereVmProviderConfig{
		VcPNID: vcpnid,
		VcPort: vcport,
		VcCreds: &VSphereVmProviderCredentials{
			Username: user,
			Password: pass,
		},
	}
	return providerConfig
}

var _ = Describe("NewClient", func() {

	Context("When called with valid config", func() {
		Specify("returns a valid client and no error", func() {
			client, err := NewClient(ctx, testConfig(server.URL.Hostname(), server.URL.Port(), "some-username", "some-password"))
			Expect(err).To(Not(HaveOccurred()))
			Expect(client).To(Not(BeNil()))
		})
	})

	Context("When called with invalid host and port", func() {
		Specify("soap.ParseURL should fail", func() {
			failConfig := testConfig("test%test", "", "test-user", "test-pass")
			client, err := NewClient(ctx, failConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("failed to parse"))
			Expect(client).To(BeNil())
		})
	})

	Context("When called with invalid VC PNID", func() {
		Specify("returns failed to parse error", func() {
			failConfig := testConfig("test-pnid", "test-port", "test-user", "test-pass")
			client, err := NewClient(ctx, failConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid"))
			Expect(err.Error()).To(ContainSubstring("port"))
			Expect(err.Error()).To(ContainSubstring("test-port"))
			Expect(client).To(BeNil())
		})
	})

	DescribeTable("Should fail if given wrong username and/or wrong password",
		func(expectedUsername, expectedPassword, username, password string) {
			server.URL.User = url.UserPassword(expectedUsername, expectedPassword)
			model.Service.Listen = server.URL
			config := testConfig(server.URL.Hostname(), server.URL.Port(), username, password)
			client, err := NewClient(ctx, config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("login failed for url"))
			Expect(client).To(BeNil())
		},
		Entry("with wrong username and password", "correct-username", "correct-password", "username", "password"),
		Entry("with wrong username and correct password", "correct-username", "correct-password", "username", "correct-password"),
		Entry("with correct username and wrong password", "correct-username", "correct-password", "correct-username", "password"),
	)
})
