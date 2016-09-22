package efsdriver

import (
	"errors"
	"fmt"
	"os"

	"path/filepath"

	"syscall"

	"encoding/json"
	"sync"

	"code.cloudfoundry.org/efsdriver/efsvoltools"
	"code.cloudfoundry.org/goshims/execshim"
	"code.cloudfoundry.org/goshims/filepathshim"
	"code.cloudfoundry.org/goshims/ioutilshim"
	"code.cloudfoundry.org/goshims/osshim"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/voldriver"
)

type EfsVolumeInfo struct {
	Ip                   string
	voldriver.VolumeInfo // see voldriver.resources.go
}

type EfsDriver struct {
	volumes       map[string]*EfsVolumeInfo
	volumesLock   sync.RWMutex
	os            osshim.Os
	filepath      filepathshim.Filepath
	exec          execshim.Exec
	ioutil        ioutilshim.Ioutil
	mountPathRoot string
	mounter       Mounter
}

//go:generate counterfeiter -o efsdriverfakes/fake_mounter.go . Mounter
type Mounter interface {
	Mount(source string, target string, fstype string, flags uintptr, data string) ([]byte, error)
	Unmount(target string, flags int) (err error)
}

func NewEfsDriver(logger lager.Logger, os osshim.Os, filepath filepathshim.Filepath, ioutil ioutilshim.Ioutil, exec execshim.Exec, mountPathRoot string, mounter Mounter) *EfsDriver {
	d := &EfsDriver{
		volumes:       map[string]*EfsVolumeInfo{},
		os:            os,
		filepath:      filepath,
		ioutil:        ioutil,
		mountPathRoot: mountPathRoot,
		mounter:       mounter,
		exec:          exec,
	}

	d.restoreState(logger)
	d.checkMounts(logger)

	return d
}

func (d *EfsDriver) Activate(logger lager.Logger) voldriver.ActivateResponse {
	return voldriver.ActivateResponse{
		Implements: []string{"VolumeDriver"},
	}
}

func (d *EfsDriver) Create(logger lager.Logger, createRequest voldriver.CreateRequest) voldriver.ErrorResponse {
	logger = logger.Session("create")
	logger.Info("start")
	defer logger.Info("end")

	var ok bool
	var ip string

	if createRequest.Name == "" {
		return voldriver.ErrorResponse{Err: "Missing mandatory 'volume_name'"}
	}

	if ip, ok = createRequest.Opts["ip"].(string); !ok {
		logger.Info("mount-config-missing-ip", lager.Data{"volume_name": createRequest.Name})
		return voldriver.ErrorResponse{Err: `Missing mandatory 'ip' field in 'Opts'`}
	}

	_, err := d.getVolume(logger, createRequest.Name)

	if err != nil {
		logger.Info("creating-volume", lager.Data{"volume_name": createRequest.Name})

		volInfo := EfsVolumeInfo{
			VolumeInfo: voldriver.VolumeInfo{Name: createRequest.Name},
			Ip:         ip,
		}

		d.volumesLock.Lock()
		defer d.volumesLock.Unlock()

		d.volumes[createRequest.Name] = &volInfo

		err := d.persistState(logger)
		if err != nil {
			logger.Error("persist-state-failed", err)
			return voldriver.ErrorResponse{Err: fmt.Sprintf("persist state failed when creating: %s", err.Error())}
		}
	}

	// TODO - if create is called twice the 'newer' create options are not retained
	return voldriver.ErrorResponse{}
}

func (d *EfsDriver) List(logger lager.Logger) voldriver.ListResponse {
	d.volumesLock.RLock()
	defer d.volumesLock.RUnlock()

	listResponse := voldriver.ListResponse{}
	for _, volume := range d.volumes {
		listResponse.Volumes = append(listResponse.Volumes, volume.VolumeInfo)
	}
	listResponse.Err = ""
	return listResponse
}

func (d *EfsDriver) Mount(logger lager.Logger, mountRequest voldriver.MountRequest) voldriver.MountResponse {
	logger = logger.Session("mount", lager.Data{"volume": mountRequest.Name})

	if mountRequest.Name == "" {
		return voldriver.MountResponse{Err: "Missing mandatory 'volume_name'"}
	}

	vol, err := d.getVolume(logger, mountRequest.Name)
	if err != nil {
		return voldriver.MountResponse{Err: fmt.Sprintf("Volume '%s' must be created before being mounted", mountRequest.Name)}
	}

	mountPath := d.mountPath(logger, vol.Name)
	logger.Info("mounting-volume", lager.Data{"id": vol.Name, "mountpoint": mountPath})

	if vol.MountCount < 1 {
		orig := syscall.Umask(000)
		defer syscall.Umask(orig)

		if err := d.mount(logger, vol.Ip, mountPath); err != nil {
			logger.Error("mount-volume-failed", err)
			return voldriver.MountResponse{Err: fmt.Sprintf("Error mounting volume: %s", err.Error())}
		}
	}

	d.volumesLock.Lock()
	defer d.volumesLock.Unlock()

	// The previous vol could be stale (since it's a value copy)
	volume := d.volumes[mountRequest.Name]
	volume.Mountpoint = mountPath
	volume.MountCount++

	logger.Info("volume-mounted", lager.Data{"name": vol.Name, "count": vol.MountCount})

	if err := d.persistState(logger); err != nil {
		logger.Error("persist-state-failed", err)
		return voldriver.MountResponse{Err: fmt.Sprintf("persist state failed when mounting: %s", err.Error())}
	}

	mountResponse := voldriver.MountResponse{Mountpoint: volume.Mountpoint}
	return mountResponse
}

func (d *EfsDriver) Path(logger lager.Logger, pathRequest voldriver.PathRequest) voldriver.PathResponse {
	logger = logger.Session("path", lager.Data{"volume": pathRequest.Name})

	if pathRequest.Name == "" {
		return voldriver.PathResponse{Err: "Missing mandatory 'volume_name'"}
	}

	vol, err := d.getVolume(logger, pathRequest.Name)
	if err != nil {
		logger.Error("failed-no-such-volume-found", err, lager.Data{"mountpoint": vol.Mountpoint})

		return voldriver.PathResponse{Err: fmt.Sprintf("Volume '%s' not found", pathRequest.Name)}
	}

	if vol.Mountpoint == "" {
		errText := "Volume not previously mounted"
		logger.Error("failed-mountpoint-not-assigned", errors.New(errText))
		return voldriver.PathResponse{Err: errText}
	}

	return voldriver.PathResponse{Mountpoint: vol.Mountpoint}
}

func (d *EfsDriver) Unmount(logger lager.Logger, unmountRequest voldriver.UnmountRequest) voldriver.ErrorResponse {
	logger = logger.Session("unmount", lager.Data{"volume": unmountRequest.Name})

	if unmountRequest.Name == "" {
		return voldriver.ErrorResponse{Err: "Missing mandatory 'volume_name'"}
	}

	vol, err := d.getVolume(logger, unmountRequest.Name)
	if err != nil {
		logger.Error("failed-no-such-volume-found", err, lager.Data{"mountpoint": vol.Mountpoint})

		return voldriver.ErrorResponse{Err: fmt.Sprintf("Volume '%s' not found", unmountRequest.Name)}
	}

	if vol.Mountpoint == "" {
		errText := "Volume not previously mounted"
		logger.Error("failed-mountpoint-not-assigned", errors.New(errText))
		return voldriver.ErrorResponse{Err: errText}
	}

	if vol.MountCount == 1 {
		if err := d.unmount(logger, unmountRequest.Name, vol.Mountpoint); err != nil {
			return voldriver.ErrorResponse{Err: err.Error()}
		}
	}

	d.volumesLock.Lock()
	defer d.volumesLock.Unlock()

	// The previous vol could be stale (since it's a value copy)
	if volume, ok := d.volumes[unmountRequest.Name]; ok {
		volume.MountCount--

		if volume.MountCount < 1 {
			delete(d.volumes, unmountRequest.Name)
		}

		if err = d.persistState(logger); err != nil {
			return voldriver.ErrorResponse{Err: fmt.Sprintf("failed to persist state when unmounting: %s", err.Error())}
		}
	}

	return voldriver.ErrorResponse{}
}

func (d *EfsDriver) Remove(logger lager.Logger, removeRequest voldriver.RemoveRequest) voldriver.ErrorResponse {
	logger = logger.Session("remove", lager.Data{"volume": removeRequest})
	logger.Info("start")
	defer logger.Info("end")

	if removeRequest.Name == "" {
		return voldriver.ErrorResponse{Err: "Missing mandatory 'volume_name'"}
	}

	vol, err := d.getVolume(logger, removeRequest.Name)

	if err != nil {
		logger.Error("warning-volume-removal", fmt.Errorf(fmt.Sprintf("Volume %s not found", removeRequest.Name)))
		return voldriver.ErrorResponse{}
	}

	if vol.Mountpoint != "" {
		if err := d.unmount(logger, removeRequest.Name, vol.Mountpoint); err != nil {
			return voldriver.ErrorResponse{Err: err.Error()}
		}
	}

	logger.Info("removing-volume", lager.Data{"name": removeRequest.Name})

	d.volumesLock.Lock()
	defer d.volumesLock.Unlock()
	delete(d.volumes, removeRequest.Name)

	if err := d.persistState(logger); err != nil {
		return voldriver.ErrorResponse{Err: fmt.Sprintf("failed to persist state when removing: %s", err.Error())}
	}

	return voldriver.ErrorResponse{}
}

func (d *EfsDriver) Get(logger lager.Logger, getRequest voldriver.GetRequest) voldriver.GetResponse {
	volume, err := d.getVolume(logger, getRequest.Name)
	if err != nil {
		return voldriver.GetResponse{Err: err.Error()}
	}

	return voldriver.GetResponse{
		Volume: voldriver.VolumeInfo{
			Name:       getRequest.Name,
			Mountpoint: volume.Mountpoint,
		},
	}
}

func (d *EfsDriver) getVolume(logger lager.Logger, volumeName string) (EfsVolumeInfo, error) {
	d.volumesLock.RLock()
	defer d.volumesLock.RUnlock()

	if vol, ok := d.volumes[volumeName]; ok {
		logger.Info("getting-volume", lager.Data{"name": volumeName})
		return *vol, nil
	}

	return EfsVolumeInfo{}, errors.New("Volume not found")
}

func (d *EfsDriver) Capabilities(logger lager.Logger) voldriver.CapabilitiesResponse {
	return voldriver.CapabilitiesResponse{
		Capabilities: voldriver.CapabilityInfo{Scope: "local"},
	}
}

// efsvoltools.VolTools methods
func (d *EfsDriver) OpenPerms(logger lager.Logger, request efsvoltools.OpenPermsRequest) efsvoltools.ErrorResponse {
	logger = logger.Session("open-perms", lager.Data{"opts": request.Opts})
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

	mountPath := d.mountPath(logger, request.Name)
	logger.Info("mounting-volume", lager.Data{"id": request.Name, "mountpoint": mountPath})

	err := d.mount(logger, ip, mountPath)
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

	if err := d.unmount(logger, request.Name, mountPath); err != nil {
		return efsvoltools.ErrorResponse{Err: err.Error()}
	}

	// TODO - delete mount folder
	return efsvoltools.ErrorResponse{}
}

func (d *EfsDriver) exists(path string) (bool, error) {
	_, err := d.os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func (d *EfsDriver) mountPath(logger lager.Logger, volumeId string) string {
	dir, err := d.filepath.Abs(d.mountPathRoot)
	if err != nil {
		logger.Fatal("abs-failed", err)
	}

	if err := d.os.MkdirAll(dir, os.ModePerm); err != nil {
		logger.Fatal("mkdir-rootpath-failed", err)
	}

	return filepath.Join(dir, volumeId)
}

func (d *EfsDriver) mount(logger lager.Logger, ip, mountPath string) error {
	logger = logger.Session("mount", lager.Data{"ip": ip, "target": mountPath})
	logger.Info("start")
	defer logger.Info("end")

	err := d.os.MkdirAll(mountPath, os.ModePerm)
	if err != nil {
		logger.Error("create-mountdir-failed", err)
		return err
	}

	// TODO--permissions & flags?
	output, err := d.mounter.Mount(ip+":/", mountPath, "nfs4", 0, "rw")
	if err != nil {
		logger.Error("mount-failed: "+string(output), err)
	}
	return err
}

func (d *EfsDriver) persistState(logger lager.Logger) error {
	// TODO--why are we passing state instead of using the one in d?

	logger = logger.Session("persist-state")
	logger.Info("start")
	defer logger.Info("end")

	stateFile := filepath.Join(d.mountPathRoot, "efs-broker-state.json")

	stateData, err := json.Marshal(d.volumes)
	if err != nil {
		logger.Error("failed-to-marshall-state", err)
		return err
	}

	err = d.ioutil.WriteFile(stateFile, stateData, os.ModePerm)
	if err != nil {
		logger.Error(fmt.Sprintf("failed-to-write-state-file: %s", stateFile), err)
		return err
	}

	logger.Debug("state-saved", lager.Data{"state-file": stateFile})
	return nil
}

func (d *EfsDriver) restoreState(logger lager.Logger) {
	logger = logger.Session("restore-state")
	logger.Info("start")
	defer logger.Info("end")

	stateFile := filepath.Join(d.mountPathRoot, "efs-broker-state.json")

	stateData, err := d.ioutil.ReadFile(stateFile)
	if err != nil {
		logger.Info(fmt.Sprintf("failed-to-read-state-file: %s", stateFile), lager.Data{"err": err})
		return
	}

	state := map[string]*EfsVolumeInfo{}
	err = json.Unmarshal(stateData, &state)

	if err != nil {
		logger.Error(fmt.Sprintf("failed-to-unmarshall-state from state-file: %s", stateFile), err)
		return
	}
	logger.Info("state-restored", lager.Data{"state-file": stateFile})

	d.volumesLock.Lock()
	defer d.volumesLock.Unlock()
	d.volumes = state
}

func (d *EfsDriver) unmount(logger lager.Logger, name string, mountPath string) error {
	logger = logger.Session("unmount")
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
	err = d.mounter.Unmount(mountPath, 0)
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

func (d *EfsDriver) checkMounts(logger lager.Logger) {
	// Note: Created volumes (with no mounts) will be removed
	//       since VolumeInfo.Mountpoint will be an empty string
	for key, mount := range d.volumes {
		cmd := d.exec.Command("mountpoint", "-q", mount.VolumeInfo.Mountpoint)

		if err := cmd.Start(); err != nil {
			logger.Error("unexpected-command-invocation-error", err)
			continue
		}

		if err := cmd.Wait(); err != nil {
			delete(d.volumes, key)
		}
	}
}
