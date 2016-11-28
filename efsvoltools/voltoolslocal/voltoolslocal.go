package voltoolslocal

import (
	"errors"
	"fmt"
	"os"

	"path/filepath"

	"syscall"

	"code.cloudfoundry.org/efsdriver/efsvoltools"
	"code.cloudfoundry.org/goshims/execshim"
	"code.cloudfoundry.org/goshims/filepathshim"
	"code.cloudfoundry.org/goshims/ioutilshim"
	"code.cloudfoundry.org/goshims/osshim"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/nfsdriver"
	"code.cloudfoundry.org/voldriver"
	"code.cloudfoundry.org/voldriver/driverhttp"
)

type EfsVolumeInfo struct {
	Ip                   string
	voldriver.VolumeInfo // see voldriver.resources.go
}

type EfsVolToolsLocal struct {
	os            osshim.Os
	filepath      filepathshim.Filepath
	exec          execshim.Exec
	mountPathRoot string
	mounter       nfsdriver.Mounter
}

func NewEfsVolToolsLocal(os osshim.Os, filepath filepathshim.Filepath, ioutil ioutilshim.Ioutil, exec execshim.Exec, mountPathRoot string, mounter nfsdriver.Mounter) *EfsVolToolsLocal {
	d := &EfsVolToolsLocal{
		os:            os,
		filepath:      filepath,
		mountPathRoot: mountPathRoot,
		mounter:       mounter,
		exec:          exec,
	}

	return d
}

// efsvoltools.VolTools methods
func (d *EfsVolToolsLocal) OpenPerms(env voldriver.Env, request efsvoltools.OpenPermsRequest) efsvoltools.ErrorResponse {
	logger := env.Logger().Session("open-perms", lager.Data{"opts": request.Opts})
	logger.Info("start")
	defer logger.Info("end")
	orig := syscall.Umask(000)
	defer syscall.Umask(orig)

	if request.Name == "" {
		return efsvoltools.ErrorResponse{Err: "Missing mandatory 'volume_name'"}
	}

	var ip string
	var ok bool
	if ip, ok = request.Opts["ip"].(string); !ok {
		logger.Info("mount-config-missing-ip", lager.Data{"volume_name": request.Name})
		return efsvoltools.ErrorResponse{Err: `Missing mandatory 'ip' field in 'Opts'`}
	}

	mountPath := d.mountPath(driverhttp.EnvWithLogger(logger, env), request.Name)
	logger.Info("mounting-volume", lager.Data{"id": request.Name, "mountpoint": mountPath})

	err := d.mount(driverhttp.EnvWithLogger(logger, env), ip, mountPath)
	if err != nil {
		logger.Error("mount-volume-failed", err)
		return efsvoltools.ErrorResponse{Err: fmt.Sprintf("Error mounting volume: %s", err.Error())}
	}

	err = d.os.Chmod(mountPath, os.ModePerm)
	if err != nil {
		logger.Error("volume-chmod-failed", err)
		return efsvoltools.ErrorResponse{Err: fmt.Sprintf("Error chmoding volume: %s", err.Error())}
	}

	logger.Info("volume-mounted", lager.Data{"name": request.Name})

	if err := d.unmount(driverhttp.EnvWithLogger(logger, env), request.Name, mountPath); err != nil {
		return efsvoltools.ErrorResponse{Err: err.Error()}
	}

	// TODO - delete mount folder
	return efsvoltools.ErrorResponse{}
}

func (d *EfsVolToolsLocal) exists(path string) (bool, error) {
	_, err := d.os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func (d *EfsVolToolsLocal) mountPath(env voldriver.Env, volumeId string) string {
	logger := env.Logger().Session("mount-path")
	orig := syscall.Umask(000)
	defer syscall.Umask(orig)

	dir, err := d.filepath.Abs(d.mountPathRoot)
	if err != nil {
		logger.Fatal("abs-failed", err)
	}

	if err := d.os.MkdirAll(dir, os.ModePerm); err != nil {
		logger.Fatal("mkdir-rootpath-failed", err)
	}

	return filepath.Join(dir, volumeId)
}

func (d *EfsVolToolsLocal) mount(env voldriver.Env, ip, mountPath string) error {
	logger := env.Logger().Session("mount", lager.Data{"ip": ip, "target": mountPath})
	logger.Info("start")
	defer logger.Info("end")

	orig := syscall.Umask(000)
	defer syscall.Umask(orig)

	err := d.os.MkdirAll(mountPath, os.ModePerm)
	if err != nil {
		logger.Error("create-mountdir-failed", err)
		return err
	}

	// TODO--permissions & flags?
	err = d.mounter.Mount(env.Logger(), env.Context(), ip+":/", mountPath, nil)
	if err != nil {
		logger.Error("mount-failed", err)
	}
	return err
}

//func (d *EfsDriver) persistState(env voldriver.Env) error {
//	// TODO--why are we passing state instead of using the one in d?
//
//	logger := env.Logger().Session("persist-state")
//	logger.Info("start")
//	defer logger.Info("end")
//
//	orig := syscall.Umask(000)
//	defer syscall.Umask(orig)
//
//	stateFile := filepath.Join(d.mountPathRoot, "efs-broker-state.json")
//
//	stateData, err := json.Marshal(d.volumes)
//	if err != nil {
//		logger.Error("failed-to-marshall-state", err)
//		return err
//	}
//
//	err = d.ioutil.WriteFile(stateFile, stateData, os.ModePerm)
//	if err != nil {
//		logger.Error(fmt.Sprintf("failed-to-write-state-file: %s", stateFile), err)
//		return err
//	}
//
//	logger.Debug("state-saved", lager.Data{"state-file": stateFile})
//	return nil
//}

//func (d *EfsDriver) restoreState(env voldriver.Env) {
//	logger := env.Logger().Session("restore-state")
//	logger.Info("start")
//	defer logger.Info("end")
//
//	stateFile := filepath.Join(d.mountPathRoot, "efs-broker-state.json")
//
//	stateData, err := d.ioutil.ReadFile(stateFile)
//	if err != nil {
//		logger.Info(fmt.Sprintf("failed-to-read-state-file: %s", stateFile), lager.Data{"err": err})
//		return
//	}
//
//	state := map[string]*EfsVolumeInfo{}
//	err = json.Unmarshal(stateData, &state)
//
//	if err != nil {
//		logger.Error(fmt.Sprintf("failed-to-unmarshall-state from state-file: %s", stateFile), err)
//		return
//	}
//	logger.Info("state-restored", lager.Data{"state-file": stateFile})
//
//	d.volumesLock.Lock()
//	defer d.volumesLock.Unlock()
//	d.volumes = state
//}

func (d *EfsVolToolsLocal) unmount(env voldriver.Env, name string, mountPath string) error {
	logger := env.Logger().Session("unmount")
	logger.Info("start")
	defer logger.Info("end")

	exists, err := d.exists(mountPath)
	if err != nil {
		logger.Error("failed-retrieving-mount-info", err, lager.Data{"mountpoint": mountPath})
		return errors.New("Error establishing whether volume exists")
	}

	if !exists {
		errText := fmt.Sprintf("Volume %s does not exist (path: %s), nothing to do!", name, mountPath)
		logger.Error("failed-mountpoint-not-found", errors.New(errText))
		return errors.New(errText)
	}

	logger.Info("unmount-volume-folder", lager.Data{"mountpath": mountPath})

	err = d.mounter.Unmount(env.Logger(), env.Context(), mountPath)
	if err != nil {
		logger.Error("unmount-failed", err)
		return fmt.Errorf("Error unmounting volume: %s", err.Error())
	}
	err = d.os.RemoveAll(mountPath)
	if err != nil {
		logger.Error("create-mountdir-failed", err)
		return fmt.Errorf("Error creating mountpoint: %s", err.Error())
	}

	logger.Info("unmounted-volume")

	return nil
}

//func (d *EfsDriver) checkMounts(env voldriver.Env) {
//	logger := env.Logger().Session("check-mounts")
//	logger.Info("start")
//	defer logger.Info("end")
//
//	for key, mount := range d.volumes {
//		ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
//		cmd := d.exec.CommandContext(ctx, "mountpoint", "-q", mount.VolumeInfo.Mountpoint)
//
//		if err := cmd.Start(); err != nil {
//			logger.Error("unexpected-command-invocation-error", err)
//			continue
//		}
//
//		if err := cmd.Wait(); err != nil {
//			// Note: Created volumes (with no mounts) will be removed
//			//       since VolumeInfo.Mountpoint will be an empty string
//			logger.Info(fmt.Sprintf("unable to verify volume %s (%s)", key, err.Error()))
//			delete(d.volumes, key)
//		}
//	}
//}
