// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"

	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

const (
	kb = 1 * 1000
	mb = kb * 1000
	gb = mb * 1000

	kib = 1 * 1024
	mib = kib * 1024
	gib = mib * 1024
)

var _ = DescribeTable("BytesToResource",
	func(b int, expQ resource.Quantity, expS string) {
		act := kubeutil.BytesToResource(int64(b))
		Expect(act.Cmp(expQ)).To(BeZero(), fmt.Sprintf("exp=%s, act=%s", expQ.String(), act.String()))
		Expect(act.String()).To(Equal(expS))
	},
	Entry("0K", 0, resource.MustParse("0k"), "0"),
	Entry("0Ki", 0, resource.MustParse("0Ki"), "0"),
	Entry("0M", 0, resource.MustParse("0M"), "0"),
	Entry("0Mi", 0, resource.MustParse("0Mi"), "0"),
	Entry("0G", 0, resource.MustParse("0G"), "0"),
	Entry("0Gi", 0, resource.MustParse("0Gi"), "0"),
	Entry("1Ki", kib*1, resource.MustParse("1Ki"), "1Ki"),
	Entry("10Ki", kib*10, resource.MustParse("10Ki"), "10Ki"),
	Entry("500MiB", mib*500, resource.MustParse("500Mi"), "500Mi"),
	Entry("512Mi", mib*512, resource.MustParse("0.5Gi"), "512Mi"),
	Entry("1GiB", gib*1, resource.MustParse("1Gi"), "1Gi"),
	Entry("10GiB", gib*10, resource.MustParse("10Gi"), "10Gi"),
	Entry("1k", kb*1, resource.MustParse("1k"), "1k"),
	Entry("10k", kb*10, resource.MustParse("10k"), "10k"),
	Entry("1M", mb*1, resource.MustParse("1M"), "1M"),
	Entry("10M", mb*10, resource.MustParse("10M"), "10M"),
	Entry("512M", mb*512, resource.MustParse("512M"), "512M"),
	Entry("1000M", mb*1000, resource.MustParse("1G"), "1G"),
	Entry("1G", gb*1, resource.MustParse("1G"), "1G"),
	Entry("1024M", mb*1024, resource.MustParse("1024M"), "1024M"),
	Entry("2000M", mb*2000, resource.MustParse("2G"), "2G"),
	Entry("2G", gb*2, resource.MustParse("2G"), "2G"),
	Entry("2048M", mb*2048, resource.MustParse("2048M"), "2048M"),
)
