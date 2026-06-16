// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package vim contains controllers for vim.vmware.com resources.
package vim

// +kubebuilder:rbac:groups=vim.vmware.com,resources=configtargets;virtualmachineconfigoptions;virtualmachineconfigpolicies;virtualmachineguestoptions,verbs=create;get;list;patch;update;watch
// +kubebuilder:rbac:groups=vim.vmware.com,resources=configtargets/status;virtualmachineconfigoptions/status;virtualmachineconfigpolicies/status;virtualmachineguestoptions/status,verbs=get;patch;update
