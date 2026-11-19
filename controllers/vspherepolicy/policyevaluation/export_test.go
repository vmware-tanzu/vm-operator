// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package policyevaluation

// Export the unexported watch mapper functions for unit testing.
var (
	VirtualMachineToPolicyEvaluationMapperFn = virtualMachineToPolicyEvaluationMapperFn
	PolicyToPolicyEvaluationMapperFn         = policyToPolicyEvaluationMapperFn
	TagPolicyToPolicyEvaluationMapperFn      = tagPolicyToPolicyEvaluationMapperFn
)
