// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package isohttpserver_test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/isohttpserver"
)

func getAvailablePort() int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		Expect(listener.Close()).To(Succeed())
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

var _ = Describe("NewServer", func() {
	When("addr or root is empty", func() {
		It("returns an error", func() {
			_, err := isohttpserver.NewServer("", "/data")
			Expect(err).To(MatchError("server addr and root cannot be empty"))

			_, err = isohttpserver.NewServer("localhost:8080", "")
			Expect(err).To(MatchError("server addr and root cannot be empty"))
		})
	})

	When("addr and root are provided", func() {
		It("initializes a new Server", func() {
			server, err := isohttpserver.NewServer("localhost:8080", "/data")
			Expect(err).ToNot(HaveOccurred())
			Expect(server.Addr).To(Equal("localhost:8080"))
			Expect(server.Root).To(Equal("/data"))
		})
	})
})

var _ = Describe("Run", func() {
	var (
		root string
		port int
	)

	BeforeEach(func() {
		var err error
		root, err = os.MkdirTemp("", "isohttpserver-test-")
		Expect(err).ToNot(HaveOccurred())

		Expect(os.MkdirAll(filepath.Join(root, "my-secret"), 0o755)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(root, "my-secret", "user-data"), []byte("#cloud-config\n"), 0o644)).To(Succeed())

		port = getAvailablePort()
	})

	AfterEach(func() {
		Expect(os.RemoveAll(root)).To(Succeed())
	})

	It("serves files at /<dir>/<file> as application/octet-stream", func() {
		server, err := isohttpserver.NewServer(fmt.Sprintf("127.0.0.1:%d", port), root)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			_ = server.Run()
		}()

		var resp *http.Response
		Eventually(func(g Gomega) {
			var err error
			resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/my-secret/user-data", port))
			g.Expect(err).ToNot(HaveOccurred())
		}).WithTimeout(2 * time.Second).Should(Succeed())

		defer func() { Expect(resp.Body.Close()).To(Succeed()) }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/octet-stream"))

		body := make([]byte, 32)
		n, _ := resp.Body.Read(body)
		Expect(string(body[:n])).To(Equal("#cloud-config\n"))
	})

	It("returns 404 for a file that does not exist", func() {
		server, err := isohttpserver.NewServer(fmt.Sprintf("127.0.0.1:%d", port), root)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			_ = server.Run()
		}()

		var resp *http.Response
		Eventually(func(g Gomega) {
			var err error
			resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/does-not-exist", port))
			g.Expect(err).ToNot(HaveOccurred())
		}).WithTimeout(2 * time.Second).Should(Succeed())

		defer func() { Expect(resp.Body.Close()).To(Succeed()) }()
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})
})
