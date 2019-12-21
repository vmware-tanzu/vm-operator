// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/simulator"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

var _ = Describe("Test Session Manager", func() {

	Context("Compute CPU Min Frequency in the Cluster", func() {

		Specify("return nil when no session objects are available", func() {
			ctx := context.Background()
			sm := vsphere.NewSessionManager(nil, nil)
			err := sm.ComputeClusterCpuMinFrequency(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		Specify("Compute CPU Min Frequency in the Cluster", func() {
			m := simulator.VPX()
			err := m.Create()
			Expect(err).To(BeNil())

			m.Service.TLS = new(tls.Config)
			server := m.Service.NewServer()
			defer server.Close()
			host := server.URL.Hostname()
			port, err := strconv.Atoi(server.URL.Port())
			Expect(err).To(BeNil())

			dcName := "DC0"
			clusterName := "DC0_C0"
			config := newVSphereVmProviderConfig(host, port, dcName, clusterName)
			sm := vsphere.NewSessionManager(nil, nil)

			ctx := context.Background()
			_, err = govmomi.NewClient(ctx, server.URL, true)
			Expect(err).NotTo(HaveOccurred())

			_, err = sm.NewSession("ns", config)
			Expect(err).NotTo(HaveOccurred())
			err = sm.ComputeClusterCpuMinFrequency(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func newVSphereVmProviderConfig(vcAddress string, vcPort int, dc, cluster string) *vsphere.VSphereVmProviderConfig {
	return &vsphere.VSphereVmProviderConfig{
		VcPNID: vcAddress,
		VcPort: strconv.Itoa(vcPort),
		VcCreds: &vsphere.VSphereVmProviderCredentials{
			Username: "some-user",
			Password: "some-pass",
		},
		Datacenter:    fmt.Sprintf("/%v", dc),
		ResourcePool:  fmt.Sprintf("/%v/host/%v/Resources", dc, cluster),
		Folder:        fmt.Sprintf("/%v/vm", dc),
		Datastore:     fmt.Sprintf("/%v/datastore/LocalDS_0", dc),
		ContentSource: "",
	}
}
