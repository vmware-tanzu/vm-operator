// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package flexvmclasses holds the E2E specs for Flex VM Classes (spec 015,
// .sdd/specs/015-vmclass-v2/). Each story adds its specs here and pairs every
// new behavior with a check that the existing (pre-Flex) behavior still
// works: legacy classes created through the VI admin API, older API
// versions, and objects that were created before an operator upgrade.
package flexvmclasses

import (
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// FeatureLabel selects every Flex VM Classes spec.
const FeatureLabel = "flex-vm-classes"

// SpecInput contains the dependencies shared by the Flex VM Classes specs.
type SpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	WCPNamespaceName string
}
