// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/controllers/util/encoding"
)

// ApplyYAML calls ApplyYAMLWithNamespace with an empty namespace.
func ApplyYAML(ctx context.Context, c client.Client, data []byte) error {
	return ApplyYAMLWithNamespace(ctx, c, data, "")
}

// ApplyYAMLWithNamespace applies the provided YAML as unstructured data with
// the given client.
// The data may be a single YAML document or multidoc YAML
// This function is idempotent.
// When a non-empty namespace is provided then all objects are assigned the
// the namespace prior to being created.
func ApplyYAMLWithNamespace(ctx context.Context, c client.Client, data []byte, namespace string) error {
	return ForEachObjectInYAML(ctx, c, data, namespace, func(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
		// Create the object on the API server.
		if err := c.Create(ctx, obj); err != nil {
			// The create call is idempotent, so if the object already exists
			// then do not consider it to be an error.
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrapf(
					err,
					"failed to create object %s %s/%s",
					obj.GroupVersionKind(),
					obj.GetNamespace(),
					obj.GetName())
			}
		}
		return nil
	})
}

// DeleteYAML calls DeleteYAMLWithNamespace with an empty namespace.
func DeleteYAML(ctx context.Context, c client.Client, data []byte) error {
	return DeleteYAMLWithNamespace(ctx, c, data, "")
}

// DeleteYAMLWithNamespace deletes the provided YAML as unstructured data with
// the given client.
// The data may be a single YAML document or multidoc YAML.
// When a non-empty namespace is provided then all objects are assigned the
// the namespace prior to any other actions being performed with or to the
// object.
func DeleteYAMLWithNamespace(ctx context.Context, c client.Client, data []byte, namespace string) error {
	return ForEachObjectInYAML(ctx, c, data, namespace, func(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
		// Delete the object on the API server.
		if err := c.Delete(ctx, obj); err != nil {
			// The delete call is idempotent, so if the object does not
			// exist, then do not consider it to be an error.
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(
					err,
					"failed to delete object %s %s/%s",
					obj.GroupVersionKind(),
					obj.GetNamespace(),
					obj.GetName())
			}
		}
		return nil
	})
}

// ExistsYAML calls ExistsYAMLWithNamespace with an empty namespace.
func ExistsYAML(ctx context.Context, c client.Client, data []byte) error {
	return ExistsYAMLWithNamespace(ctx, c, data, "")
}

// ExistsYAMLWithNamespace verifies each object in the provided YAML exists on
// the API server.
// The data may be a single YAML document or multidoc YAML.
// When a non-empty namespace is provided then all objects are assigned the
// the namespace prior to any other actions being performed with or to the
// object.
// A nil error is returned if all objects exist.
func ExistsYAMLWithNamespace(ctx context.Context, c client.Client, data []byte, namespace string) error {
	return ForEachObjectInYAML(ctx, c, data, namespace, func(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
		key, err := client.ObjectKeyFromObject(obj)
		if err != nil {
			return errors.Wrapf(
				err,
				"failed to get object key for %s %s/%s",
				obj.GroupVersionKind(),
				obj.GetNamespace(),
				obj.GetName())
		}
		if err := c.Get(ctx, key, obj); err != nil {
			return errors.Wrapf(
				err,
				"failed to find %s %s/%s",
				obj.GroupVersionKind(),
				obj.GetNamespace(),
				obj.GetName())
		}
		return nil
	})
}

// DoesNotExistYAML calls DoesNotExistYAMLWithNamespace with an empty namespace.
func DoesNotExistYAML(ctx context.Context, c client.Client, data []byte) (bool, error) {
	return DoesNotExistYAMLWithNamespace(ctx, c, data, "")
}

// DoesNotExistYAMLWithNamespace verifies each object in the provided YAML no
// longer exists on the API server.
// The data may be a single YAML document or multidoc YAML.
// When a non-empty namespace is provided then all objects are assigned the
// the namespace prior to any other actions being performed with or to the
// object.
// A boolean true is returned if none of the objects exist.
// An error is returned if the Get call returns an error other than
// 404 NotFound.
func DoesNotExistYAMLWithNamespace(ctx context.Context, c client.Client, data []byte, namespace string) (bool, error) {
	found := true
	err := ForEachObjectInYAML(ctx, c, data, namespace, func(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
		key, err := client.ObjectKeyFromObject(obj)
		if err != nil {
			return errors.Wrapf(
				err,
				"failed to get object key for %s %s/%s",
				obj.GroupVersionKind(),
				obj.GetNamespace(),
				obj.GetName())
		}
		if err := c.Get(ctx, key, obj); err != nil {
			found = false
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(
					err,
					"failed to find %s %s/%s",
					obj.GroupVersionKind(),
					obj.GetNamespace(),
					obj.GetName())
			}
			return nil
		}
		return nil
	})
	return found, err
}

// ForEachObjectInYAMLActionFunc is a function that is executed against each
// object found in a YAML document.
// When a non-empty namespace is provided then the object is assigned the
// namespace prior to any other actions being performed with or to the object.
type ForEachObjectInYAMLActionFunc func(context.Context, client.Client, *unstructured.Unstructured) error

// ForEachObjectInYAML excutes actionFn for each object in the provided YAML.
// If an error is returned then no further objects are processed.
// The data may be a single YAML document or multidoc YAML.
// When a non-empty namespace is provided then all objects are assigned the
// the namespace prior to any other actions being performed with or to the
// object.
func ForEachObjectInYAML(
	ctx context.Context,
	c client.Client,
	data []byte,
	namespace string,
	actionFn ForEachObjectInYAMLActionFunc) error {

	chanObj, chanErr := encoding.DecodeYAML(data)
	for {
		select {
		case obj := <-chanObj:
			if obj == nil {
				return nil
			}
			if namespace != "" {
				obj.SetNamespace(namespace)
			}
			if err := actionFn(ctx, c, obj); err != nil {
				return err
			}
		case err := <-chanErr:
			if err == nil {
				return nil
			}
			return errors.Wrap(err, "received error while decoding yaml to delete from server")
		}
	}
}
