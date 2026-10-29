// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"context"
	"crypto/tls"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	_ "github.com/vmware/govmomi/pbm/simulator" // load PBM simulator
	"github.com/vmware/govmomi/simulator"
	_ "github.com/vmware/govmomi/vapi/simulator" // load VAPI simulator

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/credentials"
)

var _ = Describe("NewClient", Label(testlabels.VCSim), func() {

	var (
		ctx        context.Context
		model      *simulator.Model
		server     *simulator.Server
		vcConfig   *config.VSphereVMProviderConfig
		inlineFlag bool
	)

	BeforeEach(func() {
		ctx = logr.NewContext(context.Background(), GinkgoLogr)
		inlineFlag = false
	})

	JustBeforeEach(func() {
		model = simulator.VPX()
		Expect(model.Create()).To(Succeed())
		DeferCleanup(model.Remove)

		// NewVimClient defaults the scheme to https, so the simulator must
		// serve TLS. NewClient also builds REST and PBM clients, so their
		// endpoints have to be registered.
		model.Service.TLS = &tls.Config{}
		model.Service.RegisterEndpoints = true
		server = model.Service.NewServer()
		DeferCleanup(server.Close)

		password, _ := simulator.DefaultLogin.Password()
		vcConfig = &config.VSphereVMProviderConfig{
			VcPNID: server.URL.Hostname(),
			VcPort: server.URL.Port(),
			VcCreds: credentials.VSphereVMProviderCredentials{
				Username: simulator.DefaultLogin.Username(),
				Password: password,
			},
			Datacenter:            model.Map().Any("Datacenter").Reference().Value,
			InsecureSkipTLSVerify: true,
		}

		ctx = pkgcfg.WithContext(ctx, pkgcfg.Default())
		ctx = pkgcfg.UpdateContext(ctx, func(cfg *pkgcfg.Config) {
			cfg.Features.VCSessionInlineRelogin = inlineFlag
		})
	})

	// This is the only production site that reads the feature state. Without
	// these two specs, deleting the line or reading the wrong field leaves
	// every other test passing while the feature never activates.
	When("VCSessionInlineRelogin is true", func() {
		BeforeEach(func() {
			inlineFlag = true
		})
		It("enables inline re-login on the underlying client", func() {
			c, err := client.NewClient(ctx, vcConfig)
			Expect(err).ToNot(HaveOccurred())
			defer c.Logout(ctx)

			Expect(c.Client.Config().InlineReloginEnabled).To(BeTrue())
		})
	})

	When("VCSessionInlineRelogin is false", func() {
		BeforeEach(func() {
			inlineFlag = false
		})
		It("leaves the underlying client on the legacy keepalive", func() {
			c, err := client.NewClient(ctx, vcConfig)
			Expect(err).ToNot(HaveOccurred())
			defer c.Logout(ctx)

			Expect(c.Client.Config().InlineReloginEnabled).To(BeFalse())
		})
	})
})
