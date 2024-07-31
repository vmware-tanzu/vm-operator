// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

const (
	AnnotationServiceExternalTrafficPolicyKey = "virtualmachineservice.vmoperator.vmware.com/service.externalTrafficPolicy"
	// AnnotationServiceHealthCheckNodePortKey is the key of the annotation that is used to set HTTP health check on the TKG Service's healthCheckNodePort when the Service is LoadBalancer type and externalTrafficPolicy is Local.
	AnnotationServiceHealthCheckNodePortKey = "virtualmachineservice.vmoperator.vmware.com/service.healthCheckNodePort"
	// AnnotationServiceEndpointHealthCheckEnabledKey is the key of the annotation that is used to enable health check on the VMService endpoint port. This is different from AnnotationServiceHealthCheckNodePortKey.
	AnnotationServiceEndpointHealthCheckEnabledKey = "virtualmachineservice.vmoperator.vmware.com/service.endpointHealthCheckEnabled"
)
