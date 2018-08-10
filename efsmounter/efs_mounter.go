package efsmounter

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/voldriver"
	"code.cloudfoundry.org/voldriver/driverhttp"
	"code.cloudfoundry.org/voldriver/invoker"
)

//go:generate counterfeiter -o nfsdriverfakes/fake_mounter.go . Mounter
type Mounter interface {
	Mount(env voldriver.Env, source string, target string, opts map[string]interface{}) error
	Unmount(env voldriver.Env, target string) error
	Check(env voldriver.Env, name, mountPoint string) bool
	Purge(env voldriver.Env, path string)
}

type efsMounter struct {
	invoker     invoker.Invoker
	fstype      string
	defaultOpts string
	awsAZ       string
}

func NewEfsMounter(invoker invoker.Invoker, fstype, defaultOpts string, awsAZ string) Mounter {
	return &efsMounter{invoker, fstype, defaultOpts, awsAZ}
}

func (m *efsMounter) Mount(env voldriver.Env, source string, target string, opts map[string]interface{}) error {
	azMap, mapOk := opts["az-map"].(map[string]interface{})
	if mapOk {
		if mapSource, ok := azMap[m.awsAZ].(string); ok {
			source = mapSource
		}
	}

	_, err := m.invoker.Invoke(env, "mount", []string{"-t", m.fstype, "-o", m.defaultOpts, source, target})
	return err
}

func (m *efsMounter) Unmount(env voldriver.Env, target string) error {
	_, err := m.invoker.Invoke(env, "umount", []string{target})
	return err
}

func (m *efsMounter) Check(env voldriver.Env, name, mountPoint string) bool {
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	env = driverhttp.EnvWithContext(ctx, env)
	_, err := m.invoker.Invoke(env, "mountpoint", []string{"-q", mountPoint})

	if err != nil {
		// Note: Created volumes (with no mounts) will be removed
		//       since VolumeInfo.Mountpoint will be an empty string
		env.Logger().Info(fmt.Sprintf("unable to verify volume %s (%s)", name, err.Error()))
		return false
	}
	return true
}

func (m *efsMounter) Purge(env voldriver.Env, path string) {
	return
}
