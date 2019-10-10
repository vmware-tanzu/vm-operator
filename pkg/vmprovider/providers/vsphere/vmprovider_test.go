// +build !integration

package vsphere_test

import (
	"context"
	"errors"

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

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		mockVmProviderInterface = mocks.NewMockOvfPropertyRetriever(mockController)
	})

	AfterEach(func() {
		mockController.Finish()

	})

	Context("when annotate flag is set to false", func() {
		It("returns a virtualmachineimage object from the VM without annotations", func() {

			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
				obj := object.NewVirtualMachine(c, svm.Reference())

				resVm, err := res.NewVMFromObject(obj)
				Expect(err).To(BeNil())

				image, err := vsphere.ResVmToVirtualMachineImage(context.TODO(), "default", resVm, vsphere.DoNotAnnotateVmImage, nil)
				Expect(err).To(BeNil())
				Expect(image).ToNot(BeNil())
				Expect(image.Name).Should(Equal(obj.Name()))
				Expect(image.Annotations).To(BeEmpty())
			})
		})

		It("returns a virtualmachineimage object from the library without annotations", func() {
			item := library.Item{
				Name:      "fakeItem",
				Type:      "ovf",
				LibraryID: "fakeID",
			}
			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, "default", vsphere.DoNotAnnotateVmImage, nil)
			Expect(err).To(BeNil())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).To(BeEmpty())
		})
	})

	Context("when annotate flag is set to true", func() {
		It("returns a virtualmachineimage object from the library with annotations", func() {
			item := library.Item{
				Name:      "fakeItem",
				Type:      "ovf",
				LibraryID: "fakeID",
			}
			annotations := map[string]string{}
			annotations["version"] = "1.15"
			mockVmProviderInterface.EXPECT().
				FetchOvfPropertiesFromLibrary(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(annotations, nil).
				AnyTimes()
			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, "default", vsphere.AnnotateVmImage, mockVmProviderInterface)
			Expect(err).To(BeNil())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).NotTo(BeEmpty())
			Expect(len(image.Annotations)).To(BeEquivalentTo(1))
			Expect(image.Annotations).Should(HaveKey("version"))
			Expect(image.Annotations["version"]).Should(Equal("1.15"))
		})

		It("returns a virtualmachineimage object from the VM with annotations", func() {
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
				obj := object.NewVirtualMachine(c, svm.Reference())

				resVm, err := res.NewVMFromObject(obj)
				Expect(err).To(BeNil())

				annotations := map[string]string{}
				annotations["version"] = "1.15"
				mockVmProviderInterface.EXPECT().
					FetchOvfPropertiesFromVM(gomock.Any(), gomock.Any()).
					Return(annotations, nil).
					AnyTimes()

				image, err := vsphere.ResVmToVirtualMachineImage(context.TODO(), "default", resVm, vsphere.AnnotateVmImage, mockVmProviderInterface)
				Expect(err).To(BeNil())
				Expect(image).ToNot(BeNil())
				Expect(image.Name).Should(Equal(obj.Name()))
				Expect(image.Annotations).ToNot(BeEmpty())
				Expect(len(image.Annotations)).To(BeEquivalentTo(1))
				Expect(image.Annotations).To(HaveKey("version"))
				Expect(image.Annotations["version"]).To(Equal("1.15"))
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
				FetchOvfPropertiesFromLibrary(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil, errors.New("error occurred when downloading library content")).
				AnyTimes()
			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, "default", vsphere.AnnotateVmImage, mockVmProviderInterface)
			Expect(err).NotTo(BeNil())
			Expect(image).To(BeNil())
			Expect(err).Should(MatchError("error occurred when downloading library content"))
		})
	})
})
