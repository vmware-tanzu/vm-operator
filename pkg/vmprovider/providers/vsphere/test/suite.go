// +build !integration

// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package test

import (
	"context"
	"crypto/tls"
	"os"

	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/simulator"
	_ "github.com/vmware/govmomi/vapi/simulator" // blank import the VAPI simulator bindings
)

func BeforeSuite() (ctx context.Context,
	model *simulator.Model,
	server *simulator.Server,
	tlsKeyPath, tlsCertPath string,
	tlsModel *simulator.Model,
	tlsServer *simulator.Server) {

	ctx = context.Background()

	// Set up a simulator for testing most client interactions (ignoring TLS)
	model, server = SetupModelAndServerWithSettings(&tls.Config{
		MinVersion: tls.VersionTLS12,
	})

	// Set up a second simulator for testing TLS.
	tlsKeyPath, tlsCertPath = GenerateSelfSignedCert()
	tlsCert, err := tls.LoadX509KeyPair(tlsCertPath, tlsKeyPath)
	Expect(err).NotTo(HaveOccurred())
	tlsModel, tlsServer = SetupModelAndServerWithSettings(&tls.Config{
		Certificates: []tls.Certificate{
			tlsCert,
		},
		PreferServerCipherSuites: true,
		MinVersion:               tls.VersionTLS12,
	})

	return
}

func AfterSuite(
	ctx context.Context,
	model *simulator.Model,
	server *simulator.Server,
	tlsKeyPath, tlsCertPath string,
	tlsModel *simulator.Model,
	tlsServer *simulator.Server) {

	server.Close()
	model.Remove()

	tlsServer.Close()
	tlsModel.Remove()

	_ = os.Remove(tlsKeyPath)
	_ = os.Remove(tlsCertPath)
}
