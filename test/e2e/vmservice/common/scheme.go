// Copyright (c) 2019-2024 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package common

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storageV1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
	imageregistryv1alpha1 "github.com/vmware-tanzu/vm-operator/external/image-registry-operator/api/v1alpha1"
	imageregistryv1alpha2 "github.com/vmware-tanzu/vm-operator/external/image-registry-operator/api/v1alpha2"
	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	mopv1alpha2 "github.com/vmware-tanzu/vm-operator/external/mobility-operator/api/v1alpha2"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha1"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	vspherepolv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1a6 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// InitScheme adds scheme to each API typed resources.
func InitScheme() *runtime.Scheme {
	GinkgoHelper()
	sc := runtime.NewScheme()
	addSchemes(sc)

	return sc
}

func addSchemes(sc *runtime.Scheme) {
	Expect(apiextensionsv1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add apiextensions v1 to scheme")
	Expect(appsv1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add apps v1 to scheme")
	Expect(corev1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add core v1 to scheme")
	Expect(storageV1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add storage v1 to scheme")

	Expect(byokv1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable to add byok v1alpha1 to scheme")
	Expect(capv1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable to add capabilities v1alpha1 to scheme")
	Expect(cnsv1alpha1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable to add cns v1alpha1 to scheme")
	Expect(imageregistryv1alpha1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable to add Image Registry APIs to scheme")
	Expect(imageregistryv1alpha2.AddToScheme(sc)).ToNot(HaveOccurred(), "unable to add Image Registry APIs to scheme")
	Expect(infrav1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable to add infra v1alpha1 APIs to scheme")
	Expect(mopv1alpha2.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add v1alpha2 Mobility Operator APIs to scheme")
	Expect(ncpv1alpha1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add ncp v1alpha1 to scheme")
	Expect(netopv1alpha1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add net-operator v1alpha1 to scheme")
	Expect(spqv1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable to add SPQ Operator APIs to scheme")
	Expect(topologyv1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add tanzu topology APIs to scheme")
	Expect(vimv1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add vim v1alpha1 APIs to scheme")
	Expect(vpcv1alpha1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable to add NSX Operator APIs to scheme")
	Expect(vspherepolv1alpha1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable to add vsphere-policy v1alpha1 APIs to scheme")

	Expect(vmopv1a1.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add v1alpha1 VMOP APIs to scheme")
	Expect(vmopv1a2.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add v1alpha2 VMOP APIs to scheme")
	Expect(vmopv1a3.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add v1alpha3 VMOP APIs to scheme")
	Expect(vmopv1a5.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add v1alpha5 VMOP APIs to scheme")
	Expect(vmopv1a6.AddToScheme(sc)).ToNot(HaveOccurred(), "unable add v1alpha6 VMOP APIs to scheme")
}
