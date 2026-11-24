// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"

	"github.com/vmware/govmomi/vapi/tagging"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// ResolveTagIDs implements VirtualMachineProviderInterface.
func (vs *vSphereVMProvider) ResolveTagIDs(
	ctx context.Context,
	category string,
	names []string) (string, map[string]string, error) {

	if category == "" {
		return "", nil, errors.New(
			"category name is required to resolve tag IDs")
	}

	ctx = pkgctx.WithVCOpID(ctx, nil, "resolveTagIDs")

	ids := map[string]string{}
	if len(names) == 0 {
		return "", ids, nil
	}

	c, err := vs.getVcClient(ctx)
	if err != nil {
		return "", nil, fmt.Errorf(
			"failed to get vSphere client for ResolveTagIDs: %w", err)
	}
	mgr := tagging.NewManager(c.RestClient())

	var (
		categoryID string
		fromCache  bool
	)
	if v, ok := vs.tagCategoryIDs.Load(category); ok {
		categoryID, fromCache = v.(string), true
	}

	if categoryID == "" {
		if categoryID, err = lookupTagCategoryID(ctx, mgr, category); err != nil {
			return "", nil, err
		}
		if categoryID == "" {
			return "", ids, nil
		}
		vs.tagCategoryIDs.Store(category, categoryID)
	}

	ids, err = lookupTagIDs(ctx, mgr, categoryID, names)
	if err != nil {
		return "", nil, err
	}

	if len(ids) == 0 && fromCache {
		// The cached category may have been deleted and recreated, so drop it
		// and look the category up again.
		vs.tagCategoryIDs.Delete(category)
		return vs.ResolveTagIDs(ctx, category, names)
	}

	return categoryID, ids, nil
}

// lookupTagCategoryID returns the ID of the named category, or an empty string
// if there is no such category.
func lookupTagCategoryID(
	ctx context.Context,
	mgr *tagging.Manager,
	category string) (string, error) {

	items, err := mgr.ListCategories(ctx, category)
	if err != nil {
		return "", fmt.Errorf(
			"failed to list tag category %q: %w", category, err)
	}
	for _, item := range items {
		if item.Info.Name == category {
			return item.CategoryID, nil
		}
	}
	return "", nil
}

// lookupTagIDs returns the IDs of the named tags in the category, keyed by
// name.
func lookupTagIDs(
	ctx context.Context,
	mgr *tagging.Manager,
	categoryID string,
	names []string) (map[string]string, error) {

	items, err := mgr.ListTags(ctx, names...)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags %v: %w", names, err)
	}

	ids := map[string]string{}
	for _, item := range items {
		if item.Info.Category == categoryID {
			ids[item.Info.Name] = item.Tag
		}
	}
	return ids, nil
}
