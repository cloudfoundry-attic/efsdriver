package voltoolshttp_test

import (
	"net/http"
	"time"

	"code.cloudfoundry.org/clock/fakeclock"

	"bytes"

	"context"

	"code.cloudfoundry.org/efsdriver/efsvoltools"
	"code.cloudfoundry.org/efsdriver/efsvoltools/voltoolshttp"
	"code.cloudfoundry.org/goshims/http_wrap/http_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/voldriver"
	"code.cloudfoundry.org/voldriver/driverhttp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RemoteClient", func() {

	var (
		testLogger          lager.Logger
		testCtx             context.Context
		testEnv             voldriver.Env
		httpClient          *http_fake.FakeClient
		voltools            efsvoltools.VolTools
		invalidHttpResponse *http.Response
		fakeClock           *fakeclock.FakeClock
	)

	BeforeEach(func() {
		testLogger = lagertest.NewTestLogger("LocalDriver Server Test")
		testCtx = context.TODO()
		testEnv = driverhttp.NewHttpDriverEnv(testLogger, testCtx)

		httpClient = new(http_fake.FakeClient)
		fakeClock = fakeclock.NewFakeClock(time.Now())
		voltools = voltoolshttp.NewRemoteClientWithClient("http://127.0.0.1:8080", httpClient, fakeClock)
	})

	Context("when the driver returns as error and the transport is TCP", func() {

		BeforeEach(func() {
			fakeClock = fakeclock.NewFakeClock(time.Now())
			httpClient = new(http_fake.FakeClient)
			voltools = voltoolshttp.NewRemoteClientWithClient("http://127.0.0.1:8080", httpClient, fakeClock)
			invalidHttpResponse = &http.Response{
				StatusCode: 500,
				Body:       stringCloser{bytes.NewBufferString("{\"Err\":\"some error string\"}")},
			}
		})

		It("should not be able to open up permissions", func() {
			httpClient.DoReturns(invalidHttpResponse, nil)

			response := voltools.OpenPerms(testEnv, efsvoltools.OpenPermsRequest{})

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

			response := voltools.OpenPerms(testEnv, efsvoltools.OpenPermsRequest{})

			By("giving back no error")
			Expect(response.Err).To(Equal(""))
		})
	})
})
