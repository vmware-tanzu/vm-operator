// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentsource

import "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

// SortableContentSources implements sort.Interface to sort ContentSources by earliest CreationTimestamp to latest.
type SortableContentSources []v1alpha1.ContentSource

func (list SortableContentSources) Len() int {
	return len(list)
}

func (list SortableContentSources) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list SortableContentSources) Less(i, j int) bool {
	return list[i].CreationTimestamp.Time.After(list[j].CreationTimestamp.Time)
}
