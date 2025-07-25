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
	Entry("0", gib*0, resource.MustParse("0Gi"), "0"),
	Entry("500MiB", mib*500, resource.MustParse("500Mi"), "500Mi"),
	Entry("512Mi", mib*512, resource.MustParse("0.5Gi"), "512Mi"),
	Entry("1GiB", gib*1, resource.MustParse("1Gi"), "1Gi"),
	Entry("10GiB", gib*10, resource.MustParse("10Gi"), "10Gi"),
)
