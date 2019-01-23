/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package sharedinformers

// SetupKubernetesTypes registers the config for watching Kubernetes types
func (si *SharedInformers) SetupKubernetesTypes() bool {
	// Set this to true to initial the ClientSet and InformerFactory for
	// Kubernetes APIs (e.g. Deployment)
	return true
}

// StartAdditionalInformers starts watching Deployments
func (si *SharedInformers) StartAdditionalInformers(shutdown <-chan struct{}) {
	// Start specific Kubernetes API informers here.  Note, it is only necessary
	// to start 1 informer for each Kind. (e.g. only 1 Deployment informer)

	// Listen to Service and Endpoint events
	go si.KubernetesFactory.Core().V1().Services().Informer().Run(shutdown)
	go si.KubernetesFactory.Core().V1().Endpoints().Informer().Run(shutdown)
}
