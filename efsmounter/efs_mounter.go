package efsmounter

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/dockerdriver"
	"code.cloudfoundry.org/dockerdriver/driverhttp"
	"code.cloudfoundry.org/dockerdriver/invoker"
)

//go:generate counterfeiter -o ../efsdriverfakes/fake_mounter.go . Mounter
type Mounter interface {
	Mount(env dockerdriver.Env, source string, target string, opts map[string]interface{}) error
	Unmount(env dockerdriver.Env, target string) error
	Check(env dockerdriver.Env, name, mountPoint string) bool
	Purge(env dockerdriver.Env, path string)
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

func (m *efsMounter) Mount(env dockerdriver.Env, source string, target string, opts map[string]interface{}) error {
	azMap, mapOk := opts["az-map"].(map[string]interface{})
	if mapOk {
		if mapSource, ok := azMap[m.awsAZ].(string); ok {
			source = mapSource
		}
	}

	_, err := m.invoker.Invoke(env, "mount", []string{"-t", m.fstype, "-o", m.defaultOpts, source, target})
	return err
}

func (m *efsMounter) Unmount(env dockerdriver.Env, target string) error {
	_, err := m.invoker.Invoke(env, "umount", []string{target})
	return err
}

func (m *efsMounter) Check(env dockerdriver.Env, name, mountPoint string) bool {
	ctx, cncl := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	defer cncl()
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

func (m *efsMounter) Purge(env dockerdriver.Env, path string) {
	return
}
