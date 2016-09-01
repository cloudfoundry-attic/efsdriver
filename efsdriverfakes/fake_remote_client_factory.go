// This file was generated by counterfeiter
package efsdriverfakes

import (
	"sync"

	"code.cloudfoundry.org/efsdriver/efsvoltools"
	"code.cloudfoundry.org/efsdriver/efsvoltools/voltoolshttp"
)

type FakeEfsRemoteClientFactory struct {
	NewRemoteClientStub        func(url string) (efsvoltools.VolTools, error)
	newRemoteClientMutex       sync.RWMutex
	newRemoteClientArgsForCall []struct {
		url string
	}
	newRemoteClientReturns struct {
		result1 efsvoltools.VolTools
		result2 error
	}
}

func (fake *FakeEfsRemoteClientFactory) NewRemoteClient(url string) (efsvoltools.VolTools, error) {
	fake.newRemoteClientMutex.Lock()
	fake.newRemoteClientArgsForCall = append(fake.newRemoteClientArgsForCall, struct {
		url string
	}{url})
	fake.newRemoteClientMutex.Unlock()
	if fake.NewRemoteClientStub != nil {
		return fake.NewRemoteClientStub(url)
	} else {
		return fake.newRemoteClientReturns.result1, fake.newRemoteClientReturns.result2
	}
}

func (fake *FakeEfsRemoteClientFactory) NewRemoteClientCallCount() int {
	fake.newRemoteClientMutex.RLock()
	defer fake.newRemoteClientMutex.RUnlock()
	return len(fake.newRemoteClientArgsForCall)
}

func (fake *FakeEfsRemoteClientFactory) NewRemoteClientArgsForCall(i int) string {
	fake.newRemoteClientMutex.RLock()
	defer fake.newRemoteClientMutex.RUnlock()
	return fake.newRemoteClientArgsForCall[i].url
}

func (fake *FakeEfsRemoteClientFactory) NewRemoteClientReturns(result1 efsvoltools.VolTools, result2 error) {
	fake.NewRemoteClientStub = nil
	fake.newRemoteClientReturns = struct {
		result1 efsvoltools.VolTools
		result2 error
	}{result1, result2}
}

var _ voltoolshttp.EfsRemoteClientFactory = new(FakeEfsRemoteClientFactory)
