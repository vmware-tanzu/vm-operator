// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package crd

import "embed"

// Bases is an embedded filesystem containing the VM Operator base CRDs.
//
//go:embed bases/*.yaml
var Bases embed.FS

// External is an embedded filesystem containing the external CRDs provided by
// VM Operator.
//
//go:embed external-crds/encryption.vmware.com_*.yaml
//go:embed external-crds/vsphere.policy.vmware.com_*.yaml
var External embed.FS
