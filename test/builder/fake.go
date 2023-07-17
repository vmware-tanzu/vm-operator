// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clientgorecord "k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

func NewFakeClient(objs ...client.Object) client.Client {
	scheme := NewScheme()
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func NewFakeRecorder() (record.Recorder, chan string) {
	fakeEventRecorder := clientgorecord.NewFakeRecorder(1024)
	recorder := record.New(fakeEventRecorder)
	return recorder, fakeEventRecorder.Events
}

func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	_ = v1alpha2.AddToScheme(scheme)
	_ = ncpv1alpha1.AddToScheme(scheme)
	_ = cnsv1alpha1.AddToScheme(scheme)
	_ = netopv1alpha1.AddToScheme(scheme)
	_ = topologyv1.AddToScheme(scheme)
	_ = imgregv1a1.AddToScheme(scheme)
	return scheme
}
