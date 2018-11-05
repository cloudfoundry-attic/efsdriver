package efsvoltools

import (
	"code.cloudfoundry.org/dockerdriver"
	"github.com/tedsuo/rata"
)

const (
	OpenPermsRoute = "openPerms"
)

var Routes = rata.Routes{
	{Path: "/EfsDriver.OpenPerms", Method: "POST", Name: OpenPermsRoute},
}

//go:generate counterfeiter -o ../efsdriverfakes/fake_vol_tool.go . VolTools

type VolTools interface {
	OpenPerms(env dockerdriver.Env, getRequest OpenPermsRequest) ErrorResponse
}

type OpenPermsRequest struct {
	Name string
	Opts map[string]interface{}
}

type ErrorResponse struct {
	Err string
}
