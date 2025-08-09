// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"
	"slices"
	"strings"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultIImagePublishContentLibraryLabelKey = "imageregistry.vmware.com/default"
)

// RetrieveDefaultImagePublishContentLibrary returns first lexicographical ordered content library by name
// with the "imageregistry.vmware.com/default" label. A NotFound error is returned if no default content library
// was found.
func RetrieveDefaultImagePublishContentLibrary(ctx context.Context, c ctrlclient.Client, namespace string) (
	*imgregv1a1.ContentLibrary, error) {
	clList := &imgregv1a1.ContentLibraryList{}
	if err := c.List(ctx,
		clList,
		ctrlclient.InNamespace(namespace),
		ctrlclient.MatchingLabels{DefaultIImagePublishContentLibraryLabelKey: ""},
	); err != nil {
		return nil, err
	}

	if len(clList.Items) == 0 {
		return nil, apierrors.NewNotFound(schema.GroupResource{
			Group:    imgregv1a1.GroupVersion.Group,
			Resource: "ContentLibrary",
		}, "")
	}

	return &slices.SortedFunc(slices.Values(clList.Items), func(a, b imgregv1a1.ContentLibrary) int {
		return strings.Compare(a.Name, b.Name)
	})[0], nil
}

func ConvertFieldErrorsToStrings(fieldErrs field.ErrorList) []string {
	var validationErrs []string
	for _, fieldErr := range fieldErrs {
		if fieldErr != nil {
			validationErrs = append(validationErrs, fieldErr.Error())
		}
	}
	return validationErrs
}
