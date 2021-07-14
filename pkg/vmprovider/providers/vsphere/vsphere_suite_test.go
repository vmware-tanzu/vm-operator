// +build !integration

// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/simulator"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/test"
)

var (
	model             *simulator.Model
	server            *simulator.Server
	ctx               context.Context
	tlsTestModel      *simulator.Model
	tlsServer         *simulator.Server
	tlsServerCertPath string
	tlsServerKeyPath  string
)

var _ = BeforeSuite(func() {
	ctx, model, server,
		tlsServerKeyPath, tlsServerCertPath,
		tlsTestModel, tlsServer = test.BeforeSuite()
})

var _ = AfterSuite(func() {
	test.AfterSuite(
		ctx,
		model, server,
		tlsServerKeyPath, tlsServerCertPath,
		tlsTestModel, tlsServer)
})

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vSphere Provider Suite")
}
