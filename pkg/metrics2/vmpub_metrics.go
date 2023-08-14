// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package metrics2

import (
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

type PublishResult int

const (
	PublishFailed     PublishResult = -1
	PublishInProgress PublishResult = 0
	PublishSucceeded  PublishResult = 1
)

var (
	vmPubMetricsOnce sync.Once
	vmPubMetrics     *VMPublishMetrics
)

type VMPublishMetrics struct {
	vmPubRequest *prometheus.GaugeVec
}

// NewVMPublishMetrics initializes a singleton and registers all the defined metrics.
func NewVMPublishMetrics() *VMPublishMetrics {
	vmPubMetricsOnce.Do(func() {
		vmPubMetrics = &VMPublishMetrics{
			vmPubRequest: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: "vm",
				Name:      "publish_request",
				Help:      "VirtualMachine publish request result",
			}, []string{
				"name",
				"namespace",
			}),
		}

		metrics.Registry.MustRegister(
			vmPubMetrics.vmPubRequest,
		)
	})

	return vmPubMetrics
}

// RegisterVMPublishRequest registers VM publish request metrics with the given value.
func (m *VMPublishMetrics) RegisterVMPublishRequest(logger logr.Logger, reqName, ns string, val PublishResult) {
	labels := getVMPubRequestLabels(reqName, ns)
	m.vmPubRequest.With(labels).Set(float64(val))

	logger.V(5).WithValues("labels", labels, "result", val).Info("Set metrics for VM publish request")
}

// DeleteMetrics deletes all the related VM publish request metrics from the given name and namespace.
func (m *VMPublishMetrics) DeleteMetrics(logger logr.Logger, reqName, ns string) {
	labels := getVMPubRequestLabels(reqName, ns)
	deleted := m.vmPubRequest.Delete(labels)

	logger.V(5).WithValues("labels", labels, "deleted", deleted).Info("Delete VM publish request metrics")
}

func getVMPubRequestLabels(name, ns string) prometheus.Labels {
	return prometheus.Labels{
		"name":      name,
		"namespace": ns,
	}
}
