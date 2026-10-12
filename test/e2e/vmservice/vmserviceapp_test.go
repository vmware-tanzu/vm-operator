// Copyright (c) 2023 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmservicee2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmserviceapp"
)

var _ = Describe("Testing VM Services App Workloads", FlakeAttempts(5), Label("jenkins"), Label("packer"), Label("vmserviceapp"), func() {
	ctx := context.TODO()
	specInputFunc := func() vmserviceapp.SpecInput {
		return vmserviceapp.SpecInput{
			ArtifactFolder:   artifactFolder,
			ClusterProxy:     svClusterProxy,
			Config:           config,
			SkipCleanup:      skipCleanup,
			WCPClient:        wcpClient,
			WCPNamespaceName: wcpNamespaceName,
		}
	}

	Context("JENKINS", func() {
		vmserviceapp.JenkinsSpec(ctx, specInputFunc)
	})

	Context("PACKER", func() {
		vmserviceapp.PackerSpec(ctx, specInputFunc)
	})
})
