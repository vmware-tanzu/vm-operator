// +build !integration

/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere_test

import (
	"context"
	"crypto/tls"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/simulator"
)

var model *simulator.Model
var server *simulator.Server
var ctx context.Context

var _ = BeforeSuite(func() {
	model = simulator.VPX()

	// By Default, the model being used by vcsim has two ResourcePools
	// (one for the cluster and host each). Setting model.Host=0 ensure
	// we only have one ResourcePool, making it easier to pick the
	// ResourcePool without having to look up using a hardcoded path.
	model.Host = 0

	err := model.Create()
	Expect(err).To(BeNil())

	model.Service.TLS = new(tls.Config)
	server = model.Service.NewServer()

	ctx = context.Background()
})

var _ = AfterSuite(func() {
	server.Close()
	model.Remove()
})

func TestVSphereProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vSphere Provider Suite")
}
