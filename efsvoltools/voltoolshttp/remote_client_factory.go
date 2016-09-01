package voltoolshttp

import "code.cloudfoundry.org/efsdriver/efsvoltools"

//go:generate counterfeiter -o ../../efsdriverfakes/fake_remote_client_factory.go . EfsRemoteClientFactory

type EfsRemoteClientFactory interface {
	NewRemoteClient(url string) (efsvoltools.VolTools, error)
}

func NewRemoteClientFactory() EfsRemoteClientFactory {
	return &remoteClientFactory{}
}

type remoteClientFactory struct{}

func (_ *remoteClientFactory) NewRemoteClient(url string) (efsvoltools.VolTools, error) {
	return NewRemoteClient(url)
}
