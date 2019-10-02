// +build !integration

package vsphere_test

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/mocks"
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
		It("returns a virtualmachineimage object without annotations", func() {
			item := library.Item{
				Name:      "fakeItem",
				Type:      "ovf",
				LibraryID: "fakeID",
			}
			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, "default", vsphere.DoNotAnnotateVmImage, nil)
			Expect(err).To(BeNil())
			Expect(image).ToNot(BeNil())
			Expect(image.Annotations).To(BeEmpty())
			Expect(image.Name).Should(Equal("fakeItem"))
		})
	})

	Context("when annotate flag is set to true", func() {
		It("returns a virtualmachineimage object with annotations", func() {
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
