// +build !integration

// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package test

import (
	"crypto/tls"

	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/simulator"
)

func SetupModelAndServerWithSettings(tlsConfig *tls.Config) (*simulator.Model, *simulator.Server) {
	newModel := simulator.VPX()

	// By Default, the Model being used by vcsim has two ResourcePools
	// (one for the cluster and host each). Setting Model.Host=0 ensures
	// we only have one ResourcePool, making it easier to pick the
	// ResourcePool without having to look up using a hardcoded path.
	newModel.Host = 0

	err := newModel.Create()
	Expect(err).ToNot(HaveOccurred())

	newModel.Service.RegisterEndpoints = true

	newModel.Service.TLS = tlsConfig
	newServer := newModel.Service.NewServer()

	return newModel, newServer
}
