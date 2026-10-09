// Copyright (c) 2022-2024 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/external/image-registry-operator/api/v1alpha1"
)

// ListContentLibraries returns v1alpha1.ContentLibraryList from the associated Kubernetes Cluster.
func ListContentLibraries(ctx context.Context, client ctrlclient.Client, namespace string, options []ctrlclient.ListOption) (*v1alpha1.ContentLibraryList, error) {
	contentLibrariesList := &v1alpha1.ContentLibraryList{}

	options = append(options, ctrlclient.InNamespace(namespace))

	err := client.List(ctx, contentLibrariesList, options...)
	if err != nil {
		return nil, err
	}

	return contentLibrariesList, nil
}

// GetContentLibraryByUUID returns a v1alpha1.ContentLibrary with a Spec.UUID matching the passed id.
func GetContentLibraryByUUID(ctx context.Context, client ctrlclient.Client, namespace, id string) (*v1alpha1.ContentLibrary, error) {
	cls, err := ListContentLibraries(ctx, client, namespace, []ctrlclient.ListOption{})
	if err != nil {
		return nil, err
	}

	for i := range cls.Items {
		if string(cls.Items[i].Spec.UUID) == id {
			return &cls.Items[i], nil
		}
	}

	return nil, &ErrContentLibraryNotFound{id: id, namespace: namespace}
}

/*
 * Errors
 */

// ErrContentLibraryNotFound is an error indicating a ContentLibrary was not
// found in a particular namespace.
type ErrContentLibraryNotFound struct {
	id        string
	namespace string
}

func (e ErrContentLibraryNotFound) Error() string {
	return fmt.Sprintf("ContentLibrary with UUID '%s' not found in namespace '%s'", e.id, e.namespace)
}
