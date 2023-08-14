// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package metrics2

import (
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	clItemMetricsOnce sync.Once
	clItemMetrics     *ContentLibraryItemMetrics
)

type ContentLibraryItemMetrics struct {
	vmiResourceResolve *prometheus.GaugeVec
	vmiContentSync     *prometheus.GaugeVec
}

// NewContentLibraryItemMetrics initializes a singleton and registers all the defined metrics.
func NewContentLibraryItemMetrics() *ContentLibraryItemMetrics {
	clItemMetricsOnce.Do(func() {
		clItemMetrics = &ContentLibraryItemMetrics{
			vmiResourceResolve: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: "vmi",
				Name:      "resource_resolve",
				Help:      "VMImage CR resolve status reconciled by the content library item controller",
			}, []string{
				vmiNameLabel,
				vmiNamespaceLabel,
			}),
			vmiContentSync: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: "vmi",
				Name:      "content_sync",
				Help:      "VMImage content sync status from fetching the library item in vSphere",
			}, []string{
				vmiNameLabel,
				vmiNamespaceLabel,
			}),
		}

		metrics.Registry.MustRegister(
			clItemMetrics.vmiResourceResolve,
			clItemMetrics.vmiContentSync,
		)
	})

	return clItemMetrics
}

// RegisterVMIResourceResolve registers vmi resource resolve status metrics.
// If success is true, it sets the value to 1 else to 0.
func (m *ContentLibraryItemMetrics) RegisterVMIResourceResolve(logger logr.Logger, vmiName, ns string, success bool) {
	labels := getVMIMetricsLabels(vmiName, ns)
	m.vmiResourceResolve.With(labels).Set(func() float64 {
		if success {
			return 1
		}
		return 0
	}())

	logger.V(5).WithValues("labels", labels).Info("Set metrics for VMImage CR resolve status")
}

// RegisterVMIContentSync registers vmi content sync status metrics.
// If success is true, it sets the value to 1 else to 0.
func (m *ContentLibraryItemMetrics) RegisterVMIContentSync(logger logr.Logger, vmiName, ns string, success bool) {
	labels := getVMIMetricsLabels(vmiName, ns)
	m.vmiContentSync.With(labels).Set(func() float64 {
		if success {
			return 1
		}
		return 0
	}())

	logger.V(5).WithValues("labels", labels).Info("Set metrics for VMImage content sync status")
}

// DeleteMetrics deletes all the related ContentLibraryItem metrics from the given name and namespace.
func (m *ContentLibraryItemMetrics) DeleteMetrics(logger logr.Logger, vmiName, ns string) {
	labels := getVMIMetricsLabels(vmiName, ns)
	m.vmiResourceResolve.Delete(labels)
	m.vmiContentSync.Delete(labels)

	logger.V(5).WithValues("labels", labels).Info("Deleted all VMImage related Metrics")
}

func getVMIMetricsLabels(name, ns string) prometheus.Labels {
	return prometheus.Labels{
		vmiNameLabel:      name,
		vmiNamespaceLabel: ns,
	}
}
