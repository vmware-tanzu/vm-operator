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

	// Crypto describes a test related to encryption.
	Crypto = "crypto"

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

	// Service describes a test related to a service (non-Controller runnable).
	Service = "service"

	// Update describes a test related to update logic.
	Update = "update"

	// Validation describes a test related to a validation webhook.
	Validation = "validation"

	// V1Alpha1 describes a test related to the v1alpha1 APIs.
	V1Alpha1 = "v1alpha1"

	// V1Alpha3 describes a test related to the v1alpha3 APIS.
	V1Alpha3 = "v1alpha3"

	// VCSim describes a test that uses vC Sim.
	VCSim = "vcsim"

	// Webhook describes a test related to a webhook.
	Webhook = "webhook"
)
