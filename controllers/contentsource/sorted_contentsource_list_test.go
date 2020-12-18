// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentsource_test

import (
	"sort"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/contentsource"
)

func testSortedContentSources() {
	Context("with ContentSources with CreationTimestamp in random orders", func() {
		list := contentsource.SortableContentSources([]v1alpha1.ContentSource{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "dummy-name-1",
					CreationTimestamp: metav1.NewTime(time.Unix(13, 0)),
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "dummy-name-2",
					CreationTimestamp: metav1.NewTime(time.Unix(10, 0)),
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "dummy-name-3",
					CreationTimestamp: metav1.NewTime(time.Unix(11, 0)),
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "dummy-name-4",
					CreationTimestamp: metav1.NewTime(time.Unix(12, 0)),
				},
			},
		})

		It("should sort by latest to oldest CreationTimestamp", func() {
			sort.Sort(list)

			for i := 1; i < len(list)-1; i++ {
				Expect(list[i-1].CreationTimestamp.Time.After(list[i].CreationTimestamp.Time))
			}
		})
	})
}
