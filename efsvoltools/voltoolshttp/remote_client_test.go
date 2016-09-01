package voltoolshttp_test

import (
	"net/http"
	"time"

	"code.cloudfoundry.org/clock/fakeclock"

	"bytes"

	"code.cloudfoundry.org/efsdriver/efsvoltools"
	"code.cloudfoundry.org/efsdriver/efsvoltools/voltoolshttp"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/cloudfoundry/gunk/http_wrap/httpfakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RemoteClient", func() {

	var (
		testLogger          = lagertest.NewTestLogger("LocalDriver Server Test")
		httpClient          *httpfakes.FakeClient
		voltools            efsvoltools.VolTools
		invalidHttpResponse *http.Response
		fakeClock           *fakeclock.FakeClock
	)

	BeforeEach(func() {
		httpClient = new(httpfakes.FakeClient)
		fakeClock = fakeclock.NewFakeClock(time.Now())
		voltools = voltoolshttp.NewRemoteClientWithClient("http://127.0.0.1:8080", httpClient, fakeClock)
	})

	Context("when the driver returns as error and the transport is TCP", func() {

		BeforeEach(func() {
			fakeClock = fakeclock.NewFakeClock(time.Now())
			httpClient = new(httpfakes.FakeClient)
			voltools = voltoolshttp.NewRemoteClientWithClient("http://127.0.0.1:8080", httpClient, fakeClock)
			invalidHttpResponse = &http.Response{
				StatusCode: 500,
				Body:       stringCloser{bytes.NewBufferString("{\"Err\":\"some error string\"}")},
			}
		})

		It("should not be able to open up permissions", func() {
			httpClient.DoReturns(invalidHttpResponse, nil)

			response := voltools.OpenPerms(testLogger, efsvoltools.OpenPermsRequest{})

			By("signaling an error")
			Expect(response.Err).To(Equal("some error string"))
		})
	})

	Context("when the driver returns successful and the transport is TCP", func() {
		It("should be able to open permissions", func() {
			resp := &http.Response{
				StatusCode: 200,
				Body:       stringCloser{bytes.NewBufferString("{}")},
			}
			httpClient.DoReturns(resp, nil)

			response := voltools.OpenPerms(testLogger, efsvoltools.OpenPermsRequest{})

			By("giving back no error")
			Expect(response.Err).To(Equal(""))
		})
	})
})
