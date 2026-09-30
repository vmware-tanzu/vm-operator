// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// scriptedFileStatusTransport intercepts "downloadsession/file get" polls
// made against a real vcsim server, returning a scripted sequence of
// statuses before letting the real (synchronously PREPARED) vcsim response
// through. vcsim itself has no way to simulate a NOT_PREPARED->PREPARED
// transition, so this lets a test observe the exponential-backoff poll loop
// in generateDownloadURLForLibraryItem across multiple iterations.
type scriptedFileStatusTransport struct {
	next http.RoundTripper

	mu        sync.Mutex
	statuses  []string
	pollTimes []time.Time
}

func (t *scriptedFileStatusTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if isDownloadSessionFileAction(req, "get") {
		t.mu.Lock()
		t.pollTimes = append(t.pollTimes, time.Now())
		var status string
		if len(t.statuses) > 0 {
			status, t.statuses = t.statuses[0], t.statuses[1:]
		}
		t.mu.Unlock()

		if status != "" {
			return restValueResponse(req, library.DownloadFile{Status: status})
		}
	}

	return t.next.RoundTrip(req)
}

// erroredSessionTransport fails the first "download-session get" with a
// terminal ErrorMessage, so a test can assert the poll loop stops
// immediately instead of retrying.
type erroredSessionTransport struct {
	next http.RoundTripper

	mu       sync.Mutex
	getCalls int
}

func (t *erroredSessionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if isDownloadSessionGet(req) {
		t.mu.Lock()
		t.getCalls++
		first := t.getCalls == 1
		t.mu.Unlock()

		if first {
			return restValueResponse(req, library.Session{
				ErrorMessage: &rest.LocalizableMessage{DefaultMessage: "boom"},
			})
		}
	}

	return t.next.RoundTrip(req)
}

func isDownloadSessionFileAction(req *http.Request, action string) bool {
	return req.Method == http.MethodPost &&
		strings.Contains(req.URL.Path, "/downloadsession/file/id:") &&
		req.URL.Query().Get("~action") == action
}

func isDownloadSessionGet(req *http.Request) bool {
	return req.Method == http.MethodGet &&
		strings.Contains(req.URL.Path, "/download-session/id:")
}

// restValueResponse builds a fake "/rest" endpoint response: the vapi REST
// client always unwraps responses to that endpoint from a {"value": ...}
// envelope.
func restValueResponse(req *http.Request, value any) (*http.Response, error) {
	body, err := json.Marshal(struct {
		Value any `json:"value"`
	}{value})
	if err != nil {
		return nil, fmt.Errorf("marshal REST response: %w", err)
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    req,
	}, nil
}

// downloadSessionTransport serves the entire download in memory. A real network
// connection would prevent synctest from reliably advancing virtual time.
type downloadSessionTransport struct {
	pendingPolls int

	mu        sync.Mutex
	pollTimes []time.Time
}

func (t *downloadSessionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	switch {
	case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/download-session"):
		return restValueResponse(req, "session-id")
	case isDownloadSessionGet(req):
		return restValueResponse(req, library.Session{})
	case req.Method == http.MethodDelete && strings.Contains(req.URL.Path, "/download-session/id:"):
		return restValueResponse(req, nil)
	case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/downloadsession/file"):
		return restValueResponse(req, []library.DownloadFile{{Name: "test.ovf"}})
	case isDownloadSessionFileAction(req, "prepare"):
		return restValueResponse(req, library.DownloadFile{Status: "UNPREPARED"})
	case isDownloadSessionFileAction(req, "get"):
		t.mu.Lock()
		t.pollTimes = append(t.pollTimes, time.Now())
		stillPending := len(t.pollTimes) <= t.pendingPolls
		t.mu.Unlock()

		if stillPending {
			return restValueResponse(req, library.DownloadFile{Status: "UNPREPARED"})
		}
		return restValueResponse(req, library.DownloadFile{
			Status:           "PREPARED",
			DownloadEndpoint: &library.TransferEndpoint{URI: "https://content-library.test/test.ovf"},
		})
	case req.Method == http.MethodGet && req.URL.Path == "/test.ovf":
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`<Envelope xmlns="http://schemas.dmtf.org/ovf/envelope/1"/>`)),
			Header:     http.Header{"Content-Type": []string{"application/xml"}},
			Request:    req,
		}, nil
	default:
		return nil, fmt.Errorf("unexpected download-session request: %s %s", req.Method, req.URL)
	}
}

func newDownloadSessionTestProvider(transport *downloadSessionTransport, waitSeconds int) contentlibrary.Provider {
	c := rest.NewClient(&vim25.Client{
		Client: soap.NewClient(&url.URL{Scheme: "https", Host: "content-library.test"}, true),
	})
	c.Transport = transport
	return contentlibrary.NewProviderWithWaitSec(c, waitSeconds)
}

func downloadSessionBackoffTests(t *testing.T) {
	Describe("Download session backoff with virtual time", Label(testlabels.API), func() {
		DescribeTable("keeps polling at the plateau until prepared", func(waitSeconds int) {
			var (
				err          error
				gotEnvelope  bool
				pollTimes    []time.Time
				firstPollGap time.Duration
			)
			// Only the in-memory operation runs in the bubble. Keep Ginkgo
			// assertions outside it so failures are reported by the spec.
			synctest.Test(t, func(*testing.T) {
				transport := &downloadSessionTransport{pendingPolls: 10}
				provider := newDownloadSessionTestProvider(transport, waitSeconds)
				ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
				defer cancel()
				start := time.Now()
				envelope, retrieveErr := provider.RetrieveOvfEnvelopeFromLibraryItem(ctx, &library.Item{ID: "item-id"})
				err, gotEnvelope = retrieveErr, envelope != nil
				pollTimes = transport.pollTimes
				if len(pollTimes) > 0 {
					firstPollGap = pollTimes[0].Sub(start)
				}
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(gotEnvelope).To(BeTrue())
			Expect(firstPollGap).To(BeZero())
			Expect(pollTimes).To(HaveLen(11))
			multipliers := []int{1, 2, 4, 8, 16, 32, 64, 120, 120, 120}
			for i, multiplier := range multipliers {
				gap := pollTimes[i+1].Sub(pollTimes[i])
				base := time.Duration(waitSeconds*multiplier) * time.Second
				// Virtual time has no scheduling overhead. Retain production
				// jitter and verify its bounds, including the phase transition.
				Expect(gap).To(BeNumerically(">=", base), "poll interval %d", i+1)
				Expect(gap).To(BeNumerically("<", base+base/10), "poll interval %d", i+1)
			}
		},
			Entry("with a one-second seed", 1),
			Entry("with a five-second seed", 5),
		)

		DescribeTable("interrupts a plateau wait at the context deadline", func(timeout time.Duration, polls int) {
			var (
				err         error
				gotEnvelope bool
				elapsed     time.Duration
				pollTimes   []time.Time
			)
			synctest.Test(t, func(*testing.T) {
				transport := &downloadSessionTransport{pendingPolls: 1000}
				provider := newDownloadSessionTestProvider(transport, 1)
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				start := time.Now()
				envelope, retrieveErr := provider.RetrieveOvfEnvelopeFromLibraryItem(ctx, &library.Item{ID: "item-id"})
				err, gotEnvelope = retrieveErr, envelope != nil
				elapsed, pollTimes = time.Since(start), transport.pollTimes
			})

			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
			Expect(gotEnvelope).To(BeFalse())
			Expect(elapsed).To(Equal(timeout))
			Expect(pollTimes).To(HaveLen(polls))
		},
			Entry("before the first plateau poll", 3*time.Minute, 8),
			Entry("after the first plateau poll", 5*time.Minute, 9),
		)
	})
}

func clTests() {
	Describe("Content Library", func() {

		var (
			initObjects []client.Object
			ctx         *builder.TestContextForVCSim
			testConfig  builder.VCSimTestConfig

			clProvider contentlibrary.Provider
		)

		BeforeEach(func() {
			testConfig = builder.VCSimTestConfig{WithContentLibrary: true}
		})

		JustBeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
			clProvider = contentlibrary.NewProvider(ctx, ctx.RestClient)
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
			initObjects = nil
		})

		Context("when items are present in library", func() {

			It("List items id in library", func() {
				items, err := clProvider.GetLibraryItems(ctx, ctx.ContentLibraryID)
				Expect(err).ToNot(HaveOccurred())
				Expect(items).ToNot(BeEmpty())
			})

			It("Does not return error when library does not exist", func() {
				items, err := clProvider.GetLibraryItems(ctx, "dummy-cl")
				Expect(err).ToNot(HaveOccurred())
				Expect(items).To(BeEmpty())
			})

			It("Does not return error when item name is invalid when notFoundReturnErr is set to false", func() {
				item, err := clProvider.GetLibraryItem(ctx, ctx.ContentLibraryID, "dummy-name", true)
				Expect(err).To(HaveOccurred())
				Expect(item).To(BeNil())

				item, err = clProvider.GetLibraryItem(ctx, ctx.ContentLibraryID, "dummy-name", false)
				Expect(err).NotTo(HaveOccurred())
				Expect(item).To(BeNil())
			})

			It("Gets items and returns OVF", func() {
				item, err := clProvider.GetLibraryItem(ctx, ctx.ContentLibraryID, ctx.ContentLibraryItem1Name, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(item).ToNot(BeNil())

				ovfEnvelope, err := clProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, item)
				Expect(err).ToNot(HaveOccurred())
				Expect(ovfEnvelope).ToNot(BeNil())
			})
		})

		Context("when polling for a download session file to be prepared", func() {
			var item *library.Item

			JustBeforeEach(func() {
				var err error
				item, err = clProvider.GetLibraryItem(ctx, ctx.ContentLibraryID, ctx.ContentLibraryItem1Name, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(item).ToNot(BeNil())
			})

			It("polls multiple times before the file is prepared, then succeeds", func() {
				transport := &scriptedFileStatusTransport{
					next:     ctx.RestClient.Transport,
					statuses: []string{"UNPREPARED", "UNPREPARED", "UNPREPARED"},
				}
				ctx.RestClient.Transport = transport

				// NewProviderWithWaitSec only accepts whole seconds, so 1s is
				// the fastest base interval this test can exercise.
				baseWait := 1 * time.Second
				fastProvider := contentlibrary.NewProviderWithWaitSec(ctx.RestClient, 1)

				ovfEnvelope, err := fastProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, item)
				Expect(err).ToNot(HaveOccurred())
				Expect(ovfEnvelope).ToNot(BeNil())

				// 3 scripted polls + 1 real (already-PREPARED) poll.
				Expect(transport.pollTimes).To(HaveLen(4))

				gaps := make([]time.Duration, len(transport.pollTimes)-1)
				for i := range gaps {
					gaps[i] = transport.pollTimes[i+1].Sub(transport.pollTimes[i])
				}

				// First gap is at least the configured seed; each
				// subsequent gap grows (Factor: 2.0), demonstrating the
				// exponential backoff shape.
				Expect(gaps[0]).To(BeNumerically(">=", baseWait))
				for i := 1; i < len(gaps); i++ {
					Expect(gaps[i]).To(BeNumerically(">", gaps[i-1]))
				}
			})

			It("stops immediately when the download session reports a terminal error", func() {
				transport := &erroredSessionTransport{next: ctx.RestClient.Transport}
				ctx.RestClient.Transport = transport

				fastProvider := contentlibrary.NewProviderWithWaitSec(ctx.RestClient, 1)

				ovfEnvelope, err := fastProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, item)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("boom"))
				Expect(ovfEnvelope).To(BeNil())

				// A single check, no retries after the terminal error.
				Expect(transport.getCalls).To(Equal(1))
			})

			It("returns once the context is cancelled instead of polling forever", func() {
				statuses := make([]string, 1000)
				for i := range statuses {
					statuses[i] = "UNPREPARED"
				}
				transport := &scriptedFileStatusTransport{
					next:     ctx.RestClient.Transport,
					statuses: statuses,
				}
				ctx.RestClient.Transport = transport

				fastProvider := contentlibrary.NewProviderWithWaitSec(ctx.RestClient, 1)

				timeoutCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
				defer cancel()

				start := time.Now()
				ovfEnvelope, err := fastProvider.RetrieveOvfEnvelopeFromLibraryItem(timeoutCtx, item)
				// The poll loop races ctx.Done() against the backoff delay
				// in a select, so cancellation is observed promptly rather
				// than only after the current (much longer, 1s+) backoff
				// step completes.
				Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
				Expect(ovfEnvelope).To(BeNil())
				Expect(time.Since(start)).To(BeNumerically("<", 1*time.Second))
			})
		})

		Context("when items are not present in library", func() {

			Context("when invalid item id is passed", func() {

				It("returns an error creating a download session", func() {
					libItem := &library.Item{
						Name:      "fakeItem",
						Type:      "ovf",
						LibraryID: "fakeID",
					}

					ovf, err := clProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, libItem)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("404 Not Found"))
					Expect(ovf).To(BeNil())
				})
			})
		})

		Context("called with an OVF that is invalid because of network connectivity issue", func() {
			var ovfPath string

			AfterEach(func() {
				if ovfPath != "" {
					Expect(os.Remove(ovfPath)).To(Succeed())
				}
			})

			It("returns error", func() {
				ovf, err := os.CreateTemp("", "fake-*.ovf")
				Expect(err).NotTo(HaveOccurred())
				ovfPath = ovf.Name()

				ovfInfo, err := ovf.Stat()
				Expect(err).NotTo(HaveOccurred())

				libItemName := strings.Split(ovfInfo.Name(), ".ovf")[0]
				libItem := library.Item{
					Name:      libItemName,
					Type:      "ovf",
					LibraryID: ctx.LocalContentLibraryID,
				}

				err = clProvider.CreateLibraryItem(ctx, libItem, ovfPath)
				Expect(err).NotTo(HaveOccurred())

				libItem2, err := clProvider.GetLibraryItem(ctx, ctx.LocalContentLibraryID, libItemName, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(libItem2).ToNot(BeNil())
				Expect(libItem2.Name).To(Equal(libItem.Name))

				ovfEnvelope, err := clProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, libItem2)
				Expect(err).To(HaveOccurred())
				Expect(ovfEnvelope).To(BeNil())
			})
		})
	})
}
