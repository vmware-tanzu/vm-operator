// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storagepolicy_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	apirecord "k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/controllers/storage/storagepolicy"
	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("AddToManager", func() {
	It("should successfully add controller to manager", func() {
		ctx := builder.NewTestSuiteForControllerWithContext(
			cource.WithContext(
				pkgcfg.UpdateContext(
					pkgcfg.NewContextWithDefaultConfig(),
					func(config *pkgcfg.Config) {
						config.Features.VSpherePolicies = true
					},
				),
			),
			storagepolicy.AddToManager,
			manager.InitializeProvidersNoopFn)

		ctx.BeforeSuite()
		ctx.AfterSuite()
	})
})

var _ = Describe("Reconcile", func() {
	var (
		ctx        context.Context
		client     ctrlclient.Client
		reconciler *storagepolicy.Reconciler
		obj        *infrav1.StoragePolicy
		objReq     ctrl.Request
		objStatus  infrav1.StoragePolicyStatus
		namespace  string
		provider   *providerfake.VMProvider

		withObjs  []ctrlclient.Object
		withFuncs interceptor.Funcs
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()
		namespace = "test-namespace"

		provider = providerfake.NewVMProvider()

		withObjs = nil
		withFuncs = interceptor.Funcs{}

		obj = &infrav1.StoragePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy-eval",
				Namespace: namespace,
			},
			Spec: infrav1.StoragePolicySpec{
				ID: "",
			},
		}
		objReq = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: obj.Namespace,
				Name:      obj.Name,
			},
		}
	})

	JustBeforeEach(func() {

		provider.DoesProfileSupportEncryptionFn = func(
			ctx context.Context,
			profileID string) (bool, error) {

			return profileID == "my-encrypted-storage-policy", nil
		}

		provider.GetStoragePolicyStatusFn = func(
			ctx context.Context, profileID string) (infrav1.StoragePolicyStatus, error) {

			return objStatus, nil
		}

		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(infrav1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(
				&infrav1.StoragePolicy{},
			).
			WithObjects(withObjs...).
			WithInterceptorFuncs(withFuncs).
			Build()

		reconciler = storagepolicy.NewReconciler(
			ctx,
			client,
			log.Log.WithName("storagepolicy"),
			record.New(apirecord.NewFakeRecorder(100)),
			provider)
	})

	Context("when StoragePolicy does not exist", func() {
		It("should return without error", func() {
			result, err := reconciler.Reconcile(ctx, objReq)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("when StoragePolicy exists", func() {
		BeforeEach(func() {
			withObjs = append(withObjs, obj)

			objStatus = infrav1.StoragePolicyStatus{
				Datastores: []infrav1.Datastore{
					{
						ID: infrav1.ManagedObjectID{
							ObjectID: "datastore-123",
						},
						Type: infrav1.DatastoreTypeVMFS,
					},
				},
				DiskProvisioningMode: infrav1.DiskProvisioningModeThin,
				Encrypted:            false,
				StorageClasses: []string{
					"fake",
					"fake-latebinding",
				},
			}
		})

		When("there is no error getting its status", func() {
			It("should return the status", func() {
				result, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var obj2 infrav1.StoragePolicy
				Expect(client.Get(
					ctx,
					ctrlclient.ObjectKeyFromObject(obj),
					&obj2)).To(Succeed())
				Expect(obj2.Status).To(Equal(objStatus))
			})
		})

		When("there is an error getting its status", func() {

			JustBeforeEach(func() {
				provider.GetStoragePolicyStatusFn = func(
					ctx context.Context, profileID string) (infrav1.StoragePolicyStatus, error) {

					return objStatus, errors.New("my fake error")
				}
			})
			It("should return the error", func() {
				result, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("my fake error"))
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})

		When("there is an error patching its status", func() {

			BeforeEach(func() {
				withFuncs.SubResourcePatch = func(
					_ context.Context,
					_ ctrlclient.Client,
					_ string,
					_ ctrlclient.Object,
					_ ctrlclient.Patch,
					_ ...ctrlclient.SubResourcePatchOption) error {

					return errors.New("my fake error")
				}
			})

			It("should return the error", func() {
				result, err := reconciler.Reconcile(ctx, objReq)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("my fake error"))
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})

	})

	Context("when StoragePolicy is being deleted", func() {
		var deletingPolicyEval *infrav1.StoragePolicy

		BeforeEach(func() {
			deletingPolicyEval = &infrav1.StoragePolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "deleting-policy-eval",
					Namespace:  namespace,
					Finalizers: []string{"fake.com/finalizer"},
				},
				Spec: infrav1.StoragePolicySpec{},
			}
			now := metav1.Now()
			deletingPolicyEval.DeletionTimestamp = &now
			withObjs = append(withObjs, deletingPolicyEval)
		})

		It("should successfully reconcile deletion", func() {
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      deletingPolicyEval.Name,
					Namespace: deletingPolicyEval.Namespace,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should successfully handle deletion without error", func() {
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      deletingPolicyEval.Name,
					Namespace: deletingPolicyEval.Namespace,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// The object should be successfully processed for deletion.
			//
			// Note: We can't check the finalizer was removed because the patch
			// operation in the fake client removes the object when finalizers
			// are cleared
		})
	})

})
