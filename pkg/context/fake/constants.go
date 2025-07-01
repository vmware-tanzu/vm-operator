// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package fake

const (
	// ControllerName is the name of the fake controller.
	ControllerName = "fake-controller"

	// ControllerManagerName is the name of the fake controller manager.
	ControllerManagerName = "fake-controller-manager"

	// ControllerManagerNamespace is the name of the namespace in which the
	// fake controller manager's resources are located.
	ControllerManagerNamespace = "fake-vmoperator-system"

	// LeaderElectionNamespace is the namespace used to control leader election
	// for the fake controller manager.
	LeaderElectionNamespace = ControllerManagerNamespace

	// LeaderElectionID is the name of the ID used to control leader election
	// for the fake controller manager.
	LeaderElectionID = ControllerManagerName + "-runtime"

	// Namespace is the fake namespace.
	Namespace = "default"

	// WebhookName is the name of the fake webhook.
	WebhookName = "fake-webhook"

	// ServiceAccountName is fake service account name.
	ServiceAccountName = "fake-service-account"
)
