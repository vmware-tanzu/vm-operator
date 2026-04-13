// Copyright (c) 2019-2024 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package common

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storageV1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	imageregistryv1alpha1 "github.com/vmware-tanzu/vm-operator/external/image-registry-operator/api/v1alpha1"
	imageregistryv1alpha2 "github.com/vmware-tanzu/vm-operator/external/image-registry-operator/api/v1alpha2"
	mopv1alpha2 "github.com/vmware-tanzu/vm-operator/external/mobility-operator/api/v1alpha2"
	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/vm-operator/external/nsx-operator/api/vpc/v1alpha1"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha1"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	cnsunregistervolumev1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/cnsunregistervolume/v1alpha1"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
)

// InitScheme adds scheme to each API typed resources.
func InitScheme() *runtime.Scheme {
	sc := runtime.NewScheme()
	addSchemes(sc)

	return sc
}

func addSchemes(sc *runtime.Scheme) {
	err := corev1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add core v1 to scheme: %v", err)
	}

	err = appsv1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add apps v1 to scheme: %v", err)
	}

	err = storageV1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add storage v1 to scheme: %v", err)
	}

	err = cnsv1alpha1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable to add cns v1alpha1 to scheme: %v", err)
	}

	err = vmopv1a1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add v1alpha1 VMOP APIs to scheme: %v", err)
	}

	err = vmopv1a2.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add v1alpha2 VMOP APIs to scheme: %v", err)
	}

	err = vmopv1a3.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add v1alpha3 VMOP APIs to scheme: %v", err)
	}

	err = vmopv1a5.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add v1alpha5 VMOP APIs to scheme: %v", err)
	}

	err = mopv1alpha2.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add v1alpha2 Mobility Operator APIs to scheme: %v", err)
	}

	err = imageregistryv1alpha1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable to add Image Registry APIs to scheme: %v", err)
	}

	err = imageregistryv1alpha2.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable to add Image Registry APIs to scheme: %v", err)
	}

	err = topologyv1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add tanzu topology APIs to scheme: %v", err)
	}

	err = ncpv1alpha1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add ncp v1alpha1 to scheme: %v", err)
	}

	err = netopv1alpha1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add net-operator v1alpha1 to scheme: %v", err)
	}

	err = vpcv1alpha1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable to add NSX Operator APIs to scheme: %v", err)
	}

	err = spqv1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable to add SPQ Operator APIs to scheme: %v", err)
	}

	err = cnsunregistervolumev1alpha1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable to add CnsUnregisterVolume APIs to scheme: %v", err)
	}

	err = apiextensionsv1.AddToScheme(sc)
	if err != nil {
		e2eframework.Failf("unable add apiextensions v1 to scheme: %v", err)
	}
}
