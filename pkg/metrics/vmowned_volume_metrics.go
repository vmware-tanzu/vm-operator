// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	vmOwnedVolumeMetricsOnce sync.Once
	vmOwnedVolumeMetrics     *VMOwnedVolumeMetrics
)

// VMOwnedVolumeMetrics holds Prometheus counters and histograms for the
// VM-owned volume ownership-transfer attach/detach path.
type VMOwnedVolumeMetrics struct {
	attachTotal         *prometheus.CounterVec
	detachTotal         *prometheus.CounterVec
	reAdoptionTotal     *prometheus.CounterVec
	snapshotDiskRecord  *prometheus.CounterVec
	reconfigErrorsTotal *prometheus.CounterVec
	reconfigDuration    *prometheus.HistogramVec
}

// NewVMOwnedVolumeMetrics returns the process-wide singleton
// VMOwnedVolumeMetrics instance, creating and registering all Prometheus
// metrics on the first call.
func NewVMOwnedVolumeMetrics() *VMOwnedVolumeMetrics {
	vmOwnedVolumeMetricsOnce.Do(func() {
		vmOwnedVolumeMetrics = &VMOwnedVolumeMetrics{
			attachTotal: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: metricsNamespace,
					Name:      "vm_owned_volume_attach_total",
					Help: "Total number of successful VM-owned volume attach operations " +
						"(ReconfigVM disk add + CVI transition to VM_MANAGED).",
				},
				[]string{vmNameLabel, vmNamespaceLabel},
			),
			detachTotal: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: metricsNamespace,
					Name:      "vm_owned_volume_detach_total",
					Help: "Total number of successful VM-owned volume detach operations " +
						"(ReconfigVM disk remove + CVI transition to TRANSFERRING_TO_CSI).",
				},
				[]string{vmNameLabel, vmNamespaceLabel},
			),
			reAdoptionTotal: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: metricsNamespace,
					Name:      "vm_owned_volume_readoption_total",
					Help: "Total number of successful VM-owned volume re-adoption operations " +
						"(snapshot-retained volume re-adopted by a VM after snapshot revert).",
				},
				[]string{vmNameLabel, vmNamespaceLabel},
			),
			snapshotDiskRecord: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: metricsNamespace,
					Name:      "vm_owned_volume_snapshot_disk_record_total",
					Help: "Total number of snapshot disk recording operations " +
						"(VM-owned disks captured in VMSnap.status.disks at snapshot time).",
				},
				[]string{vmNameLabel, vmNamespaceLabel},
			),
			reconfigErrorsTotal: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: metricsNamespace,
					Name:      "vm_owned_volume_reconfig_errors_total",
					Help: "Total number of ReconfigVM errors encountered during VM-owned " +
						"volume attach or detach operations.",
				},
				[]string{vmNameLabel, vmNamespaceLabel, "reason"},
			),
			reconfigDuration: prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Namespace: metricsNamespace,
					Name:      "vm_owned_volume_reconfig_duration_seconds",
					Help: "Duration of ReconfigVM calls for VM-owned volume attach and " +
						"detach operations.",
					Buckets: prometheus.DefBuckets,
				},
				[]string{vmNameLabel, vmNamespaceLabel},
			),
		}

		metrics.Registry.MustRegister(
			vmOwnedVolumeMetrics.attachTotal,
			vmOwnedVolumeMetrics.detachTotal,
			vmOwnedVolumeMetrics.reAdoptionTotal,
			vmOwnedVolumeMetrics.snapshotDiskRecord,
			vmOwnedVolumeMetrics.reconfigErrorsTotal,
			vmOwnedVolumeMetrics.reconfigDuration,
		)
	})
	return vmOwnedVolumeMetrics
}

// RecordAttach increments the attach counter for the given VM.
func (m *VMOwnedVolumeMetrics) RecordAttach(vmName, namespace string) {
	if m == nil {
		return
	}
	m.attachTotal.WithLabelValues(vmName, namespace).Inc()
}

// RecordDetach increments the detach counter for the given VM.
func (m *VMOwnedVolumeMetrics) RecordDetach(vmName, namespace string) {
	if m == nil {
		return
	}
	m.detachTotal.WithLabelValues(vmName, namespace).Inc()
}

// RecordReAdoption increments the re-adoption counter for the given VM.
func (m *VMOwnedVolumeMetrics) RecordReAdoption(vmName, namespace string) {
	if m == nil {
		return
	}
	m.reAdoptionTotal.WithLabelValues(vmName, namespace).Inc()
}

// RecordSnapshotDiskRecord increments the snapshot disk recording counter.
func (m *VMOwnedVolumeMetrics) RecordSnapshotDiskRecord(vmName, namespace string) {
	if m == nil {
		return
	}
	m.snapshotDiskRecord.WithLabelValues(vmName, namespace).Inc()
}

// RecordReconfigError increments the ReconfigVM error counter for the given
// VM with the supplied reason label.
func (m *VMOwnedVolumeMetrics) RecordReconfigError(vmName, namespace, reason string) {
	if m == nil {
		return
	}
	m.reconfigErrorsTotal.WithLabelValues(vmName, namespace, reason).Inc()
}

// RecordReconfigDuration records the duration of a ReconfigVM operation.
func (m *VMOwnedVolumeMetrics) RecordReconfigDuration(vmName, namespace string, d time.Duration) {
	if m == nil {
		return
	}
	m.reconfigDuration.WithLabelValues(vmName, namespace).Observe(d.Seconds())
}
