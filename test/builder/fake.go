// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"

	"github.com/google/uuid"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	imgregv1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha2"
	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	appv1a1 "github.com/vmware-tanzu/vm-operator/external/appplatform/api/v1alpha1"
	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopapi "github.com/vmware-tanzu/vm-operator/api"
	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

func NewFakeClient(objs ...client.Object) client.Client {
	return NewFakeClientWithInterceptors(interceptor.Funcs{}, objs...)
}

func NewFakeClientWithInterceptors(
	funcs interceptor.Funcs,
	objs ...client.Object,
) client.Client {
	scheme := NewScheme()

	// Objects handed to WithObjects go straight into the tracker without
	// passing through the Create interceptor below, so assign their UIDs here.
	for _, obj := range objs {
		ensureUID(obj)
	}

	funcs.Create = assignUIDOnCreate(funcs.Create)

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(funcs).
		WithObjects(objs...).
		WithStatusSubresource(KnownObjectTypes()...).
		WithIndex(
			&vmopv1.VirtualMachineSnapshot{},
			kubeutil.VMSnapshotVMNameFieldIndex,
			kubeutil.VMSnapshotVMNameIndexerFunc).
		Build()
}

// assignUIDOnCreate wraps a Create interceptor so that every object created
// through the fake client comes back with a UID, the way an object created
// against a real API server does. The fake client assigns a resourceVersion but
// no UID, which leaves code that derives a value from an object's UID — the
// VM's directory on a datastore, a cloud-init instance ID, an owner reference —
// looking at an empty string in tests but a real ID in production.
//
// A UID the caller set explicitly is left alone, since a spec that pins one is
// usually asserting on it.
func assignUIDOnCreate(next createFunc) createFunc {
	return func(
		ctx context.Context,
		c client.WithWatch,
		obj client.Object,
		opts ...client.CreateOption) error {

		ensureUID(obj)

		if next != nil {
			return next(ctx, c, obj, opts...)
		}
		return c.Create(ctx, obj, opts...)
	}
}

type createFunc func(
	ctx context.Context,
	c client.WithWatch,
	obj client.Object,
	opts ...client.CreateOption) error

// ensureUID assigns obj a unique UID if it does not already have one.
func ensureUID(obj client.Object) {
	if obj != nil && obj.GetUID() == "" {
		obj.SetUID(types.UID(uuid.NewString()))
	}
}

// KnownObjectTypes has the known VM operator types that will be
// status enabled when initializing the fake client. Add any types
// here if the fake client needs to patch the Status sub-resource of that
// resource type.
func KnownObjectTypes() []client.Object {
	return []client.Object{
		&vmopv1.VirtualMachine{},
		&vmopv1.VirtualMachineGroup{},
		&vmopv1.VirtualMachineService{},
		&vmopv1.VirtualMachineClass{},
		&vmopv1.VirtualMachineClassInstance{},
		&vmopv1.VirtualMachinePublishRequest{},
		&vmopv1.ClusterVirtualMachineImage{},
		&vmopv1.VirtualMachineImage{},
		&vmopv1.VirtualMachineImageCache{},
		&vmopv1.VirtualMachineWebConsoleRequest{},
		&vmopv1.VirtualMachineSnapshot{},
		&vmopv1a1.WebConsoleRequest{},
		&cnsv1alpha1.CnsNodeVmAttachment{},
		&cnsv1alpha1.CnsNodeVMBatchAttachment{},
		&cnsv1alpha1.CnsRegisterVolume{},
		&spqv1.StoragePolicyQuota{},
		&spqv1.StoragePolicyUsage{},
		&spqv1.StorageQuota{},
		&ncpv1alpha1.VirtualNetworkInterface{},
		&netopv1alpha1.NetworkInterface{},
		&vpcv1alpha1.Subnet{},
		&vpcv1alpha1.SubnetSet{},
		&vpcv1alpha1.SubnetPort{},
		&byokv1.EncryptionClass{},
		&capv1.Capabilities{},
		&appv1a1.SupervisorProperties{},
		&vspherepolv1.ComputePolicy{},
		&vspherepolv1.PolicyEvaluation{},
		&vspherepolv1.TagPolicy{},
		&infrav1.StoragePolicy{},
		&vimv1.ConfigTarget{},
		&vimv1.VirtualMachineConfigOptions{},
		&vimv1.VirtualMachineConfigPolicy{},
		&vimv1.VirtualMachineGuestOptions{},
	}
}

func NewFakeRecorder() (record.Recorder, chan string) {
	fakeEventRecorder := events.NewFakeRecorder(1024)
	recorder := record.New(fakeEventRecorder)
	return recorder, fakeEventRecorder.Events
}

func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = vmopapi.AddToScheme(scheme)
	_ = capv1.AddToScheme(scheme)
	_ = byokv1.AddToScheme(scheme)
	_ = appv1a1.AddToScheme(scheme)
	_ = ncpv1alpha1.AddToScheme(scheme)
	_ = cnsv1alpha1.AddToScheme(scheme)
	_ = spqv1.AddToScheme(scheme)
	_ = netopv1alpha1.AddToScheme(scheme)
	_ = topologyv1.AddToScheme(scheme)
	_ = imgregv1a1.AddToScheme(scheme)
	_ = imgregv1.AddToScheme(scheme)
	_ = vpcv1alpha1.AddToScheme(scheme)
	_ = vspherepolv1.AddToScheme(scheme)
	_ = infrav1.AddToScheme(scheme)
	_ = vimv1.AddToScheme(scheme)
	return scheme
}
