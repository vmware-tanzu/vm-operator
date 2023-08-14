// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package metrics2

const (
	// If this changes, the metrics collection configs (e.g. telegraf) will need to be updated as well.
	metricsNamespace = "vmservice"

	// VM related metrics labels.
	vmNameLabel          = "vm_name"
	vmNamespaceLabel     = "vm_namespace"
	conditionTypeLabel   = "condition_type"
	conditionReasonLabel = "condition_reason"
	specLabel            = "spec"
	statusLabel          = "status"

	// VMImage related metrics labels (from image registry service).
	vmiNameLabel      = "vmi_name"
	vmiNamespaceLabel = "vmi_namespace"
)
