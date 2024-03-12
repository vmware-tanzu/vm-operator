// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package testlabels

const (
	// API describes a test related to the APIs.
	API = "api"

	// Controller describes a test related to the controllers.
	Controller = "controller"

	// Create describes a test related to create logic.
	Create = "create"

	// Delete describes a test related to delete logic.
	Delete = "delete"

	// EnvTest describes a test that uses the envtest package.
	EnvTest = "envtest"

	// Fuzz describes a fuzz test.
	Fuzz = "fuzz"

	// Mutation describes a test related to a mutation webhook.
	Mutation = "mutation"

	// NSXT describes a test related to NSXT.
	NSXT = "nsxt"

	// Update describes a test related to update logic.
	Update = "update"

	// Validation describes a test related to a validation webhook.
	Validation = "validation"

	// V1Alpha1 describes a test related to the v1alpha1 APIs.
	V1Alpha1 = "v1alpha1"

	// V1Alpha2 describes a test related to the v1alpha2 APIS.
	V1Alpha2 = "v1alpha2"

	// VCSim describes a test that uses vC Sim.
	VCSim = "vcsim"

	// Webhook describes a test related to a webhook.
	Webhook = "webhook"
)
