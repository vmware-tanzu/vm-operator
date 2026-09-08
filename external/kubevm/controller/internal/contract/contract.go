// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package contract reads the fixed status paths every kube-vm.io provider
// object is expected to surface, per the generic API's duck-typed status
// contract. It never imports a provider's Go types — every read goes
// through unstructured.Unstructured — so the core stays provider-agnostic.
package contract

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// InfrastructureReadyConditionType and UpToDateConditionType are the
// condition types the core reads off a provider object's status.conditions,
// mirroring them onto the generic object. Named InfrastructureReady, not
// Ready, because a provider may already reserve the literal "Ready" type for
// its own, differently-scoped meaning (VM Operator does: its guest
// readiness-probe result).
const (
	InfrastructureReadyConditionType = "InfrastructureReady"
	UpToDateConditionType            = "UpToDate"
)

// Address is a single entry of the provider object's status.addresses.
type Address struct {
	Interface string
	Type      string
	Address   string
}

// Status is the set of fixed contract paths read off a provider object.
type Status struct {
	Addresses        []Address
	PowerState       string
	ProviderID       string
	ProviderMetadata map[string]string
	Ready            *corev1.ConditionStatus
	ReadyReason      string
	ReadyMessage     string
	UpToDate         *corev1.ConditionStatus
	UpToDateReason   string
	UpToDateMessage  string
}

// ReadStatus reads every fixed contract path off the given provider object.
// A path that is absent leaves the corresponding field at its zero value;
// this is not an error, since a provider object is not required to have
// converged yet.
func ReadStatus(obj *unstructured.Unstructured) (Status, error) {
	var status Status

	addresses, found, err := unstructured.NestedSlice(obj.Object, "status", "addresses")
	if err != nil {
		return status, err
	}
	if found {
		for _, a := range addresses {
			am, ok := a.(map[string]any)
			if !ok {
				continue
			}
			status.Addresses = append(status.Addresses, Address{
				Interface: stringField(am, "interface"),
				Type:      stringField(am, "type"),
				Address:   stringField(am, "address"),
			})
		}
	}

	if v, found, err := unstructured.NestedString(obj.Object, "status", "powerState"); err != nil {
		return status, err
	} else if found {
		status.PowerState = v
	}

	if v, found, err := unstructured.NestedString(obj.Object, "status", "providerID"); err != nil {
		return status, err
	} else if found {
		status.ProviderID = v
	}

	if v, found, err := unstructured.NestedStringMap(obj.Object, "status", "providerMetadata"); err != nil {
		return status, err
	} else if found {
		status.ProviderMetadata = v
	}

	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return status, err
	}
	if found {
		for _, c := range conditions {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}

			switch stringField(cm, "type") {
			case InfrastructureReadyConditionType:
				s := corev1.ConditionStatus(stringField(cm, "status"))
				status.Ready = &s
				status.ReadyReason = stringField(cm, "reason")
				status.ReadyMessage = stringField(cm, "message")
			case UpToDateConditionType:
				s := corev1.ConditionStatus(stringField(cm, "status"))
				status.UpToDate = &s
				status.UpToDateReason = stringField(cm, "reason")
				status.UpToDateMessage = stringField(cm, "message")
			}
		}
	}

	return status, nil
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
