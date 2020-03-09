// +build !integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere_test

import (
	"context"
	"crypto/tls"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/simulator"
)

var model *simulator.Model
var server *simulator.Server
var ctx context.Context

var tlsTestModel *simulator.Model
var tlsServer *simulator.Server
var tlsServerCertPath string
var tlsServerKeyPath string

var _ = BeforeSuite(func() {
	ctx = context.Background()

	// Set up a simulator for testing most client interactions (ignoring TLS)
	model, server = setupModelAndServerWithSettings(&tls.Config{})

	// Set up a second simulator for testing TLS.
	tlsServerKeyPath, tlsServerCertPath = generateSelfSignedCert()
	cert, err := tls.LoadX509KeyPair(tlsServerCertPath, tlsServerKeyPath)
	Expect(err).NotTo(HaveOccurred())
	tlsTestModel, tlsServer = setupModelAndServerWithSettings(&tls.Config{
		Certificates: []tls.Certificate{
			cert,
		},
		PreferServerCipherSuites: true,
	})
})

func setupModelAndServerWithSettings(tlsConfig *tls.Config) (*simulator.Model, *simulator.Server) {
	newModel := simulator.VPX()

	// By Default, the Model being used by vcsim has two ResourcePools
	// (one for the cluster and host each). Setting Model.Host=0 ensures
	// we only have one ResourcePool, making it easier to pick the
	// ResourcePool without having to look up using a hardcoded path.
	newModel.Host = 0

	err := newModel.Create()
	Expect(err).ToNot(HaveOccurred())

	newModel.Service.TLS = tlsConfig
	newServer := newModel.Service.NewServer()

	return newModel, newServer
}

var _ = AfterSuite(func() {
	server.Close()
	model.Remove()

	tlsServer.Close()
	tlsTestModel.Remove()

	os.Remove(tlsServerKeyPath)
	os.Remove(tlsServerCertPath)
})

func TestVSphereProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vSphere Provider Suite")
}
