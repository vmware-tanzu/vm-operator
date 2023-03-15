// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

var (
	contentSourceMetricsOnce sync.Once
	contentSourceMetrics     *ContentSourceMetrics
)

type ContentSourceMetrics struct {
	vmImage *prometheus.GaugeVec
}

// NewContentSourceMetrics initializes a singleton and registers all the defined metrics.
func NewContentSourceMetrics() *ContentSourceMetrics {
	contentSourceMetricsOnce.Do(func() {
		contentSourceMetrics = &ContentSourceMetrics{
			vmImage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: "contentsource",
				Name:      "vmimage",
				Help:      "VMImage CR status from ContentSource items in K8s cluster",
			}, []string{
				imageNameLabel,
				imageIDLabel,
				providerNameLabel,
				providerKindLabel,
			}),
		}

		metrics.Registry.MustRegister(
			contentSourceMetrics.vmImage,
		)
	})

	return contentSourceMetrics
}

// RegisterVMImageCreateOrUpdate registers the metrics for the given VMImage.
// If success is true, it sets the value to 1 else to 0.
func (csm *ContentSourceMetrics) RegisterVMImageCreateOrUpdate(logger logr.Logger, vmImage vmopv1.VirtualMachineImage, success bool) {
	logger.V(5).Info("Adding metrics for a VMImage CR create or update operation")
	vmImageLabels := getVMImageLabels(vmImage)
	csm.vmImage.With(vmImageLabels).Set(func() float64 {
		if success {
			return 1
		}
		return 0
	}())
}

// RegisterVMImageDelete registers the metrics for the given VMImage delete operation.
// If success is true, it deletes the metric; otherwise, it sets the value to -1.
func (csm *ContentSourceMetrics) RegisterVMImageDelete(logger logr.Logger, vmImage vmopv1.VirtualMachineImage, success bool) {
	logger.V(5).Info("Adding metrics for a VMImage CR delete operation")
	labels := getVMImageLabels(vmImage)
	if success {
		csm.vmImage.Delete(labels)
	} else {
		// Setting the value to -1 to distinguish from the create/update failure operation.
		csm.vmImage.With(labels).Set(-1)
	}
}

// DeleteMetrics deletes the related metrics from the given ContentProviderReference.
func (csm *ContentSourceMetrics) DeleteMetrics(logger logr.Logger, providerRef vmopv1.ContentProviderReference) {
	logger.V(5).Info("Deleting all VMImage metrics from the given provider name and kind")
	labels := prometheus.Labels{
		providerNameLabel: providerRef.Name,
		providerKindLabel: providerRef.Kind,
	}
	csm.vmImage.DeletePartialMatch(labels)
}

// getVMImageLabels is a helper function to return all the required labels for the given VMImage.
func getVMImageLabels(vmImage vmopv1.VirtualMachineImage) prometheus.Labels {
	return prometheus.Labels{
		// Using .Status.ImageName as .Name could be modified in the duplicate VM image name case.
		imageNameLabel:    vmImage.Status.ImageName,
		imageIDLabel:      vmImage.Spec.ImageID,
		providerNameLabel: vmImage.Spec.ProviderRef.Name,
		providerKindLabel: vmImage.Spec.ProviderRef.Kind,
	}
}
