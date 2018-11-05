package voltoolshttp

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"strings"

	"fmt"

	"code.cloudfoundry.org/cfhttp"
	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/lager"
	"github.com/tedsuo/rata"

	os_http "net/http"

	"code.cloudfoundry.org/dockerdriver"
	"code.cloudfoundry.org/dockerdriver/driverhttp"
	"code.cloudfoundry.org/efsdriver/efsvoltools"
	"code.cloudfoundry.org/goshims/http_wrap"
)

type reqFactory struct {
	reqGen  *rata.RequestGenerator
	route   string
	payload []byte
}

func newReqFactory(reqGen *rata.RequestGenerator, route string, payload []byte) *reqFactory {
	return &reqFactory{
		reqGen:  reqGen,
		route:   route,
		payload: payload,
	}
}

func (r *reqFactory) Request() (*os_http.Request, error) {
	return r.reqGen.CreateRequest(r.route, nil, bytes.NewBuffer(r.payload))
}

type remoteClient struct {
	HttpClient http_wrap.Client
	reqGen     *rata.RequestGenerator
	clock      clock.Clock
}

func NewRemoteClient(url string) (*remoteClient, error) {
	client := cfhttp.NewClient()

	if strings.Contains(url, ".sock") {
		client = cfhttp.NewUnixClient(url)
		url = fmt.Sprintf("unix://%s", url)
	}
	return NewRemoteClientWithClient(url, client, clock.NewClock()), nil
}

func NewRemoteClientWithClient(socketPath string, client http_wrap.Client, clock clock.Clock) *remoteClient {
	return &remoteClient{
		HttpClient: client,
		reqGen:     rata.NewRequestGenerator(socketPath, efsvoltools.Routes),
		clock:      clock,
	}
}

func (r *remoteClient) OpenPerms(env dockerdriver.Env, request efsvoltools.OpenPermsRequest) efsvoltools.ErrorResponse {
	logger := env.Logger().Session("open-perms", lager.Data{"request": request})
	logger.Info("start")
	defer logger.Info("end")

	payload, err := json.Marshal(request)
	if err != nil {
		logger.Error("failed-marshalling-request", err)
		return efsvoltools.ErrorResponse{Err: err.Error()}
	}

	httpRequest := newReqFactory(r.reqGen, efsvoltools.OpenPermsRoute, payload)

	response, err := r.do(driverhttp.EnvWithLogger(logger, env), httpRequest)
	if err != nil {
		logger.Error("failed-creating-volume", err)
		return efsvoltools.ErrorResponse{Err: err.Error()}
	}

	if response.StatusCode == http.StatusInternalServerError {
		var remoteError efsvoltools.ErrorResponse
		if err := unmarshallJSON(logger, response.Body, &remoteError); err != nil {
			logger.Error("failed-parsing-error-response", err)
			return efsvoltools.ErrorResponse{Err: err.Error()}
		}
		return remoteError
	}

	return efsvoltools.ErrorResponse{}
}

func unmarshallJSON(logger lager.Logger, reader io.ReadCloser, jsonResponse interface{}) error {
	body, err := ioutil.ReadAll(reader)
	if err != nil {
		logger.Error("Error in Reading HTTP Response body from remote.", err)
	}
	err = json.Unmarshal(body, jsonResponse)

	return err
}

func (r *remoteClient) clientError(logger lager.Logger, err error, msg string) string {
	logger.Error(msg, err)
	return err.Error()
}

func (r *remoteClient) do(env dockerdriver.Env, requestFactory *reqFactory) (*os_http.Response, error) {
	var (
		response *os_http.Response
		err      error
		request  *os_http.Request
	)

	logger := env.Logger().Session("do")

	request, err = requestFactory.Request()
	if err != nil {
		logger.Error("request-gen-failed", err)
		return nil, err
	}

	response, err = r.HttpClient.Do(request)
	if err != nil {
		logger.Error("request-failed", err)
		return response, err
	}
	logger.Debug("response", lager.Data{"response": response.Status})

	return response, nil
}
