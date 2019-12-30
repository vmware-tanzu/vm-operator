// +build !integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io/ioutil"
	"math/big"
	"net"
	"net/url"
	"time"

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
		// Let the tests run without TLS validation by default.
		InsecureSkipTLSVerify: true,
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

// Most of the other VMoperator tests run without TLS verification. Start up a separate simulator with a fresh TLS key/cert
//  and ensure the client can connect to it.
var _ = Describe("Tests for client TLS", func() {
	Context("when the client recognizes the certificate presented by the VC", func() {
		It("successfully connects to the VC", func() {
			tlsServer.URL.User = url.UserPassword("some-username", "some-password")
			config := testConfig(tlsServer.URL.Hostname(), tlsServer.URL.Port(), "some-username", "some-password")
			config.InsecureSkipTLSVerify = false
			config.CAFilePath = tlsServerCertPath
			_, err := NewClient(context.Background(), config)

			Expect(err).NotTo(HaveOccurred())
		})
	})
	Context("when the CA bundle referred to does not exist", func() {
		It("returns an error about loading the CA bundle", func() {
			tlsServer.URL.User = url.UserPassword("some-username", "some-password")
			config := testConfig(tlsServer.URL.Hostname(), tlsServer.URL.Port(), "some-username", "some-password")
			config.CAFilePath = "/a/nonexistent/ca-bundle.crt"
			config.InsecureSkipTLSVerify = false
			_, err := NewClient(context.Background(), config)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to set root CA"))

		})

	})
	Context("when the client does not recognize the certificate presented by the VC", func() {
		Context("when TLS verification is used", func() {
			It("returns an error about certificate validation", func() {
				tlsServer.URL.User = url.UserPassword("some-username", "some-password")
				config := testConfig(tlsServer.URL.Hostname(), tlsServer.URL.Port(), "some-username", "some-password")
				_, randomCANotForServer := generateSelfSignedCert()
				config.CAFilePath = randomCANotForServer
				config.InsecureSkipTLSVerify = false
				_, err := NewClient(context.Background(), config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("x509: certificate signed by unknown authority"))
			})

		})
		Context("when TLS verification is skipped", func() {
			It("Successfully connects", func() {
				tlsServer.URL.User = url.UserPassword("some-username", "some-password")
				config := testConfig(tlsServer.URL.Hostname(), tlsServer.URL.Port(), "some-username", "some-password")
				config.CAFilePath = "/a/nonexistent/ca-bundle.crt"
				_, err := NewClient(context.Background(), config)

				Expect(err).NotTo(HaveOccurred())
			})

		})
	})
})

func generatePrivateKey() *rsa.PrivateKey {
	reader := rand.Reader
	bitSize := 2048

	// Based on https://golang.org/src/crypto/tls/generate_cert.go
	privateKey, err := rsa.GenerateKey(reader, bitSize)
	if err != nil {
		panic("failed to generate private key")
	}
	return privateKey
}

func generateSelfSignedCert() (string, string) {
	priv := generatePrivateKey()
	now := time.Now()
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	Expect(err).NotTo(HaveOccurred())

	template := x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		SerialNumber:          serialNumber,
		NotBefore:             now,
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	Expect(err).NotTo(HaveOccurred())
	certOut, err := ioutil.TempFile("", "cert.pem")
	Expect(err).NotTo(HaveOccurred())
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	Expect(err).NotTo(HaveOccurred())
	err = certOut.Close()
	Expect(err).NotTo(HaveOccurred())

	keyOut, err := ioutil.TempFile("", "key.pem")
	Expect(err).NotTo(HaveOccurred())
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	Expect(err).NotTo(HaveOccurred())
	err = pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	Expect(err).NotTo(HaveOccurred())
	err = keyOut.Close()
	Expect(err).NotTo(HaveOccurred())

	return keyOut.Name(), certOut.Name()
}
