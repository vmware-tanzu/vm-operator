// +build !integration

package vsphere_test

import (
	"context"
	"errors"
	"time"

	"github.com/vmware/govmomi/ovf"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vim25"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/mocks"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

var _ = Describe("virtualmachine images", func() {

	var mockVmProviderInterface *mocks.MockOvfPropertyRetriever
	var mockController *gomock.Controller
	var (
		versionKey    = "vmware-system-version"
		versionVal    = "1.15"
		imgKey        = "img-foo-1"
		imgVal        = "bar-1"
		imgValUpdated = imgVal + "-updated"
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		mockVmProviderInterface = mocks.NewMockOvfPropertyRetriever(mockController)
	})

	AfterEach(func() {
		mockController.Finish()

	})

	Context("when adding VM annotation", func() {
		It("should fetch annotations from OVF properties", func() {

			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
				obj := object.NewVirtualMachine(c, svm.Reference())

				expectedAnnotations := map[string]string{
					versionKey:                        versionVal,
					imgKey:                            imgVal,
					vsphere.VmOperatorVMImagePropsKey: "false",
				}

				resVm, err := res.NewVMFromObject(obj)
				Expect(err).To(BeNil())
				vmAnnotations := map[string]string{}
				vmAnnotations[versionKey] = versionVal

				imgAnnotations := map[string]string{}
				imgAnnotations[imgKey] = imgVal
				mockVmProviderInterface.EXPECT().
					GetOvfInfoFromVM(context.Background(), resVm).
					Return(imgAnnotations, nil).
					Times(1)

				// Test we fetch them the first time
				err = vsphere.AddVmImageAnnotations(vmAnnotations, ctx, mockVmProviderInterface, resVm)
				Expect(err).To(BeNil())
				Expect(vmAnnotations).Should(Equal(expectedAnnotations))

				By("not fetching them again if already there")
				// Test we don't fetch them the second time
				err = vsphere.AddVmImageAnnotations(vmAnnotations, ctx, mockVmProviderInterface, resVm)
				Expect(err).To(BeNil())
				Expect(vmAnnotations).Should(Equal(expectedAnnotations))

				By("not fetching them again even if there are changed OVF properties on image")
				// Test that we don't fetch if we have updated image annotations
				imgAnnotations[imgKey] = imgValUpdated
				mockVmProviderInterface.EXPECT().
					GetOvfInfoFromVM(gomock.Any(), gomock.Any()).
					Return(imgAnnotations, nil).
					Times(0)

				err = vsphere.AddVmImageAnnotations(vmAnnotations, ctx, mockVmProviderInterface, resVm)
				Expect(err).To(BeNil())
				Expect(vmAnnotations).Should(Equal(expectedAnnotations))

				By("re-fetch image properties when we reset the cache key")
				vmAnnotations[vsphere.VmOperatorVMImagePropsKey] = "true"
				expectedAnnotations[imgKey] = imgValUpdated
				mockVmProviderInterface.EXPECT().
					GetOvfInfoFromVM(context.Background(), resVm).
					Return(imgAnnotations, nil).
					Times(1)
				err = vsphere.AddVmImageAnnotations(vmAnnotations, ctx, mockVmProviderInterface, resVm)
				Expect(err).To(BeNil())
				Expect(vmAnnotations).Should(Equal(expectedAnnotations))
			})
		})
		It("handles errors from VC", func() {
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
				obj := object.NewVirtualMachine(c, svm.Reference())

				expectedAnnotations := map[string]string{
					versionKey: versionVal,
				}
				resVm, err := res.NewVMFromObject(obj)
				Expect(err).To(BeNil())
				vmAnnotations := map[string]string{}
				vmAnnotations[versionKey] = versionVal

				ovfFetchErr := errors.New("VC error foo bar is closed")
				mockVmProviderInterface.EXPECT().
					GetOvfInfoFromVM(context.Background(), resVm).
					Return(map[string]string{}, ovfFetchErr).
					Times(1)
				err = vsphere.AddVmImageAnnotations(vmAnnotations, ctx, mockVmProviderInterface, resVm)
				Expect(err).To(Equal(ovfFetchErr))
				Expect(vmAnnotations).Should(Not(BeEmpty()))
				Expect(vmAnnotations).Should(Equal(expectedAnnotations))
			})
		})
	})

	Context("when annotate flag is set to false", func() {

		It("returns a virtualmachineimage object from the VM without annotations", func() {

			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
				obj := object.NewVirtualMachine(c, svm.Reference())

				resVm, err := res.NewVMFromObject(obj)
				Expect(err).To(BeNil())

				image, err := vsphere.ResVmToVirtualMachineImage(context.TODO(), resVm, vsphere.DoNotAnnotateVmImage, nil)
				Expect(err).To(BeNil())
				Expect(image).ToNot(BeNil())
				Expect(image.Name).Should(Equal(obj.Name()))
				Expect(image.Annotations).To(BeEmpty())
			})
		})

		It("returns a virtualmachineimage object from the library without annotations", func() {
			ts := time.Now()

			item := library.Item{
				Name:         "fakeItem",
				Type:         "ovf",
				LibraryID:    "fakeID",
				CreationTime: &ts,
			}

			ovfEnvelope := &ovf.Envelope{
				VirtualSystem: &ovf.VirtualSystem{
					Product: []ovf.ProductSection{
						{
							Vendor:      "vendor",
							Product:     "product",
							FullVersion: "fullversion",
							Version:     "version",
							Property: []ovf.Property{{
								Key:     versionKey,
								Default: &versionVal,
							},
							},
						}},
				},
			}

			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(ovfEnvelope, nil).
				AnyTimes()

			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.DoNotAnnotateVmImage, mockVmProviderInterface)
			Expect(err).To(BeNil())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).To(BeEmpty())

			Expect((image.Spec.ProductInfo.Vendor)).Should(Equal("vendor"))
			Expect((image.Spec.ProductInfo.Product)).Should(Equal("product"))
			Expect((image.Spec.ProductInfo.FullVersion)).Should(Equal("fullversion"))
			Expect((image.Spec.ProductInfo.Version)).Should(Equal("version"))
		})
	})

	Context("when annotate flag is set to true", func() {
		It("returns a virtualmachineimage object from the library with annotations", func() {
			ts := time.Now()
			item := library.Item{
				Name:         "fakeItem",
				Type:         "ovf",
				LibraryID:    "fakeID",
				CreationTime: &ts,
			}

			ovfEnvelope := &ovf.Envelope{
				VirtualSystem: &ovf.VirtualSystem{
					Product: []ovf.ProductSection{
						{
							Vendor:      "vendor",
							Product:     "product",
							FullVersion: "fullversion",
							Version:     "version",
							Property: []ovf.Property{{
								Key:     versionKey,
								Default: &versionVal,
							},
							},
						}},
				},
			}

			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(ovfEnvelope, nil).
				AnyTimes()

			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.AnnotateVmImage, mockVmProviderInterface)
			Expect(err).To(BeNil())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).NotTo(BeEmpty())
			Expect(len(image.Annotations)).To(BeEquivalentTo(1))
			Expect(image.Annotations).Should(HaveKey("vmware-system-version"))
			Expect(image.Annotations["vmware-system-version"]).Should(Equal("1.15"))
			Expect(image.CreationTimestamp).To(BeEquivalentTo(v1.NewTime(ts)))

			Expect((image.Spec.ProductInfo.Vendor)).Should(Equal("vendor"))
			Expect((image.Spec.ProductInfo.Product)).Should(Equal("product"))
			Expect((image.Spec.ProductInfo.FullVersion)).Should(Equal("fullversion"))
			Expect((image.Spec.ProductInfo.Version)).Should(Equal("version"))
		})

		It("returns a virtualmachineimage object from the VM with annotations", func() {
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
				obj := object.NewVirtualMachine(c, svm.Reference())

				resVm, err := res.NewVMFromObject(obj)
				Expect(err).To(BeNil())

				annotations := map[string]string{}
				annotations[versionKey] = versionVal
				mockVmProviderInterface.EXPECT().
					GetOvfInfoFromVM(gomock.Any(), gomock.Any()).
					Return(annotations, nil).
					AnyTimes()

				image, err := vsphere.ResVmToVirtualMachineImage(context.TODO(), resVm, vsphere.AnnotateVmImage, mockVmProviderInterface)
				Expect(err).To(BeNil())
				Expect(image).ToNot(BeNil())
				Expect(image.Name).Should(Equal(obj.Name()))
				Expect(image.Annotations).ToNot(BeEmpty())
				Expect(len(image.Annotations)).To(BeEquivalentTo(1))
				Expect(image.Annotations).To(HaveKey(versionKey))
				Expect(image.Annotations[versionKey]).To(Equal(versionVal))
			})
		})
	})

	Context("when annotate flag is set to true and error occurs in fetching ovf properties", func() {
		It("returns a err", func() {
			item := library.Item{
				Name:      "fakeItem",
				Type:      "ovf",
				LibraryID: "fakeID",
			}
			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil, errors.New("error occurred when downloading library content")).
				AnyTimes()
			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.AnnotateVmImage, mockVmProviderInterface)
			Expect(err).NotTo(BeNil())
			Expect(image).To(BeNil())
			Expect(err).Should(MatchError("error occurred when downloading library content"))
		})
	})
})
