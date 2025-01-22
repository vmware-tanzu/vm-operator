// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mem_test

import (
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/vmware-tanzu/vm-operator/pkg/mem"
)

var _ = Describe("Start", func() {
	var (
		s      sinkr
		logger logr.Logger
	)
	BeforeEach(func() {
		s = sinkr{}
		logger = logr.New(&s)
	})
	JustBeforeEach(func() {
		mem.Start(logger, time.Millisecond*500, metrics.Registry.MustRegister)
	})
	It("Should log the memory at least twice", func() {
		Eventually(func(g Gomega) {
			g.Expect(atomic.LoadInt32(&s.infoCalls)).To(BeNumerically(">=", int32(2)))
			mf, err := metrics.Registry.Gather()
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(mf).To(HaveLen(26))
		}, time.Second*20).Should(Succeed())
	})
})

type sinkr struct {
	infoCalls int32
}

func (s sinkr) Init(info logr.RuntimeInfo) {
}

func (s sinkr) Enabled(level int) bool {
	return true
}

func (s *sinkr) Info(level int, msg string, keysAndValues ...any) {
	atomic.AddInt32(&s.infoCalls, 1)
}

func (s sinkr) Error(err error, msg string, keysAndValues ...any) {
}

func (s sinkr) WithValues(keysAndValues ...any) logr.LogSink {
	return &s
}

func (s sinkr) WithName(name string) logr.LogSink {
	return &s
}
