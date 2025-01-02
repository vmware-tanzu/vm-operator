// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"
	"fmt"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
)

const (
	// DefaultEncryptionClassLabelName is the name of the label that identifies
	// the default EncryptionClass in a given namespace.
	DefaultEncryptionClassLabelName = "encryption.vmware.com/default"

	// DefaultEncryptionClassLabelValue is the value of the label that
	// identifies the default EncryptionClass in a given namespace.
	DefaultEncryptionClassLabelValue = "true"
)

var (
	// ErrNoDefaultEncryptionClass is returned by the
	// GetDefaultEncryptionClassForNamespace method if there are no
	// EncryptionClasses in a given namespace marked as default.
	ErrNoDefaultEncryptionClass = fmt.Errorf(
		"no EncryptionClass resource has the label %q: %q",
		DefaultEncryptionClassLabelName, DefaultEncryptionClassLabelValue)

	// ErrMultipleDefaultEncryptionClasses is returned by the
	// GetDefaultEncryptionClassForNamespace method if more than one
	// EncryptionClass in a given namespace are marked as default.
	ErrMultipleDefaultEncryptionClasses = fmt.Errorf(
		"multiple EncryptionClass resources have the label %q: %q",
		DefaultEncryptionClassLabelName, DefaultEncryptionClassLabelValue)
)

// GetDefaultEncryptionClassForNamespace returns the default EncryptionClass for
// the provided namespace.
func GetDefaultEncryptionClassForNamespace(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	namespace string) (byokv1.EncryptionClass, error) {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if namespace == "" {
		panic("namespace is empty")
	}

	var list byokv1.EncryptionClassList
	if err := k8sClient.List(
		ctx,
		&list,
		ctrlclient.InNamespace(namespace),
		ctrlclient.MatchingLabels{
			DefaultEncryptionClassLabelName: DefaultEncryptionClassLabelValue,
		}); err != nil {

		return byokv1.EncryptionClass{}, err
	}
	if len(list.Items) == 0 {
		return byokv1.EncryptionClass{}, ErrNoDefaultEncryptionClass
	}
	if len(list.Items) > 1 {
		return byokv1.EncryptionClass{}, ErrMultipleDefaultEncryptionClasses
	}
	return list.Items[0], nil
}
