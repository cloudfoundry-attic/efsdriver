package voltoolshttp_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"fmt"

	"code.cloudfoundry.org/efsdriver/efsdriverfakes"
	"code.cloudfoundry.org/efsdriver/efsvoltools"
	"code.cloudfoundry.org/efsdriver/efsvoltools/voltoolshttp"
	"code.cloudfoundry.org/lager/lagertest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Volman Driver Handlers", func() {

	Context("when generating http handlers", func() {
		var testLogger = lagertest.NewTestLogger("HandlersTest")

		It("should produce a handler with an openPerms route", func() {
			By("faking out the driver")
			voltools := &efsdriverfakes.FakeVolTools{}
			voltools.OpenPermsReturns(efsvoltools.ErrorResponse{})
			handler, err := voltoolshttp.NewHandler(testLogger, voltools)
			Expect(err).NotTo(HaveOccurred())

			By("then fake serving the response using the handler")
			route, found := efsvoltools.Routes.FindRouteByName(efsvoltools.OpenPermsRoute)
			Expect(found).To(BeTrue())

			path := fmt.Sprintf("http://0.0.0.0%s", route.Path)
			openPermsReq := efsvoltools.OpenPermsRequest{Opts: map[string]interface{}{"ip": "12.12.12.12"}}
			jsonReq, err := json.Marshal(openPermsReq)
			Expect(err).NotTo(HaveOccurred())
			httpRequest, err := http.NewRequest("POST", path, bytes.NewReader(jsonReq))
			Expect(err).NotTo(HaveOccurred())

			httpResponseRecorder := httptest.NewRecorder()
			handler.ServeHTTP(httpResponseRecorder, httpRequest)

			By("then deserialing the HTTP response")
			response := efsvoltools.ErrorResponse{}
			body, err := ioutil.ReadAll(httpResponseRecorder.Body)
			err = json.Unmarshal(body, &response)

			By("then expecting correct JSON conversion")
			Expect(err).ToNot(HaveOccurred())
			Expect(response.Err).Should(BeEmpty())
		})

	})
})
