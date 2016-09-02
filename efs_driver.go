package efsdriver

import (
	"errors"
	"fmt"
	"os"

	"strings"

	"path/filepath"

	"syscall"

	"code.cloudfoundry.org/efsdriver/efsvoltools"
	"code.cloudfoundry.org/goshims/filepath"
	"code.cloudfoundry.org/goshims/os"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/voldriver"
)

const VolumesRootDir = "_volumes"
const MountsRootDir = "_mounts"

type EfsVolumeInfo struct {
	Ip                   string
	voldriver.VolumeInfo // see voldriver.resources.go
}

type EfsDriver struct {
	volumes       map[string]*EfsVolumeInfo
	os            osshim.Os
	filepath      filepathshim.Filepath
	mountPathRoot string
	mounter       Mounter
}

//go:generate counterfeiter -o efsdriverfakes/fake_mounter.go . Mounter
type Mounter interface {
	Mount(source string, target string, fstype string, flags uintptr, data string) ([]byte, error)
	Unmount(target string, flags int) (err error)
}

func NewEfsDriver(os osshim.Os, filepath filepathshim.Filepath, mountPathRoot string, mounter Mounter) *EfsDriver {
	return &EfsDriver{
		volumes:       map[string]*EfsVolumeInfo{},
		os:            os,
		filepath:      filepath,
		mountPathRoot: mountPathRoot,
		mounter:       mounter,
	}
}

func (d *EfsDriver) Activate(logger lager.Logger) voldriver.ActivateResponse {
	return voldriver.ActivateResponse{
		Implements: []string{"VolumeDriver"},
	}
}

func (d *EfsDriver) Create(logger lager.Logger, createRequest voldriver.CreateRequest) voldriver.ErrorResponse {
	logger = logger.Session("create")
	var ok bool
	var ip string

	if createRequest.Name == "" {
		return voldriver.ErrorResponse{Err: "Missing mandatory 'volume_name'"}
	}

	if ip, ok = createRequest.Opts["ip"].(string); !ok {
		logger.Info("mount-config-missing-ip", lager.Data{"volume_name": createRequest.Name})
		return voldriver.ErrorResponse{Err: `Missing mandatory 'ip' field in 'Opts'`}
	}

	if _, ok = d.volumes[createRequest.Name]; !ok {
		logger.Info("creating-volume", lager.Data{"volume_name": createRequest.Name})

		volInfo := EfsVolumeInfo{
			VolumeInfo: voldriver.VolumeInfo{Name: createRequest.Name},
			Ip:         ip,
		}
		d.volumes[createRequest.Name] = &volInfo
	}

	return voldriver.ErrorResponse{}
}

func (d *EfsDriver) List(logger lager.Logger) voldriver.ListResponse {
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

	var vol *EfsVolumeInfo
	var ok bool
	if vol, ok = d.volumes[mountRequest.Name]; !ok {
		return voldriver.MountResponse{Err: fmt.Sprintf("Volume '%s' must be created before being mounted", mountRequest.Name)}
	}

	mountPath := d.mountPath(logger, vol.Name)
	logger.Info("mounting-volume", lager.Data{"id": vol.Name, "mountpoint": mountPath})

	if vol.MountCount < 1 {
		orig := syscall.Umask(000)
		defer syscall.Umask(orig)

		err := d.mount(logger, vol.Ip, mountPath)
		if err != nil {
			logger.Error("mount-volume-failed", err)
			return voldriver.MountResponse{Err: fmt.Sprintf("Error mounting volume: %s", err.Error())}
		}

		vol.Mountpoint = mountPath
	}

	vol.MountCount++
	logger.Info("volume-mounted", lager.Data{"name": vol.Name, "count": vol.MountCount})

	mountResponse := voldriver.MountResponse{Mountpoint: vol.Mountpoint}
	return mountResponse
}

func (d *EfsDriver) Path(logger lager.Logger, pathRequest voldriver.PathRequest) voldriver.PathResponse {
	logger = logger.Session("path", lager.Data{"volume": pathRequest.Name})

	if pathRequest.Name == "" {
		return voldriver.PathResponse{Err: "Missing mandatory 'volume_name'"}
	}

	mountPath, err := d.get(logger, pathRequest.Name)
	if err != nil {
		logger.Error("failed-no-such-volume-found", err, lager.Data{"mountpoint": mountPath})

		return voldriver.PathResponse{Err: fmt.Sprintf("Volume '%s' not found", pathRequest.Name)}
	}

	if mountPath == "" {
		errText := "Volume not previously mounted"
		logger.Error("failed-mountpoint-not-assigned", errors.New(errText))
		return voldriver.PathResponse{Err: errText}
	}

	return voldriver.PathResponse{Mountpoint: mountPath}
}

func (d *EfsDriver) Unmount(logger lager.Logger, unmountRequest voldriver.UnmountRequest) voldriver.ErrorResponse {
	logger = logger.Session("unmount", lager.Data{"volume": unmountRequest.Name})

	if unmountRequest.Name == "" {
		return voldriver.ErrorResponse{Err: "Missing mandatory 'volume_name'"}
	}

	mountPath, err := d.get(logger, unmountRequest.Name)
	if err != nil {
		logger.Error("failed-no-such-volume-found", err, lager.Data{"mountpoint": mountPath})

		return voldriver.ErrorResponse{Err: fmt.Sprintf("Volume '%s' not found", unmountRequest.Name)}
	}

	if mountPath == "" {
		errText := "Volume not previously mounted"
		logger.Error("failed-mountpoint-not-assigned", errors.New(errText))
		return voldriver.ErrorResponse{Err: errText}
	}

	return d.unmount(logger, unmountRequest.Name, mountPath, false)
}

func (d *EfsDriver) Remove(logger lager.Logger, removeRequest voldriver.RemoveRequest) voldriver.ErrorResponse {
	logger = logger.Session("remove", lager.Data{"volume": removeRequest})
	logger.Info("start")
	defer logger.Info("end")

	if removeRequest.Name == "" {
		return voldriver.ErrorResponse{Err: "Missing mandatory 'volume_name'"}
	}

	var response voldriver.ErrorResponse
	var vol *EfsVolumeInfo
	var exists bool
	if vol, exists = d.volumes[removeRequest.Name]; !exists {
		logger.Error("failed-volume-removal", fmt.Errorf(fmt.Sprintf("Volume %s not found", removeRequest.Name)))
		return voldriver.ErrorResponse{fmt.Sprintf("Volume '%s' not found", removeRequest.Name)}
	}

	if vol.Mountpoint != "" {
		response = d.unmount(logger, removeRequest.Name, vol.Mountpoint, false)
		if response.Err != "" {
			return response
		}
	}

	volumePath := d.volumePath(logger, vol.Name)

	logger.Info("remove-volume-folder", lager.Data{"volume": volumePath})
	err := d.os.RemoveAll(volumePath)
	if err != nil {
		logger.Error("failed-removing-volume", err)
		return voldriver.ErrorResponse{Err: fmt.Sprintf("Failed removing mount path: %s", err)}
	}

	logger.Info("removing-volume", lager.Data{"name": removeRequest.Name})
	delete(d.volumes, removeRequest.Name)
	return voldriver.ErrorResponse{}
}

func (d *EfsDriver) Get(logger lager.Logger, getRequest voldriver.GetRequest) voldriver.GetResponse {
	mountpoint, err := d.get(logger, getRequest.Name)
	if err != nil {
		return voldriver.GetResponse{Err: err.Error()}
	}

	return voldriver.GetResponse{Volume: voldriver.VolumeInfo{Name: getRequest.Name, Mountpoint: mountpoint}}
}

func (d *EfsDriver) get(logger lager.Logger, volumeName string) (string, error) {
	if vol, ok := d.volumes[volumeName]; ok {
		logger.Info("getting-volume", lager.Data{"name": volumeName})
		return vol.Mountpoint, nil
	}

	return "", errors.New("Volume not found")
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

	ret := d.unmount(logger, request.Name, mountPath, true)
	return efsvoltools.ErrorResponse{Err: ret.Err}
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

	if !strings.HasSuffix(dir, "/") {
		dir = fmt.Sprintf("%s/", dir)
	}

	mountsPathRoot := fmt.Sprintf("%s%s", dir, MountsRootDir)
	d.os.MkdirAll(mountsPathRoot, os.ModePerm)

	return fmt.Sprintf("%s/%s", mountsPathRoot, volumeId)
}

func (d *EfsDriver) volumePath(logger lager.Logger, volumeId string) string {
	dir, err := d.filepath.Abs(d.mountPathRoot)
	if err != nil {
		logger.Fatal("abs-failed", err)
	}

	volumesPathRoot := filepath.Join(dir, VolumesRootDir)
	d.os.MkdirAll(volumesPathRoot, os.ModePerm)

	return filepath.Join(volumesPathRoot, volumeId)
}

func (d *EfsDriver) mount(logger lager.Logger, ip, mountPath string) error {
	logger.Info("mount", lager.Data{"ip": ip, "target": mountPath})

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

func (d *EfsDriver) unmount(logger lager.Logger, name string, mountPath string, ephemeral bool) voldriver.ErrorResponse {
	logger = logger.Session("unmount")
	logger.Info("start")
	defer logger.Info("end")

	exists, err := d.exists(mountPath)
	if err != nil {
		logger.Error("failed-retrieving-mount-info", err, lager.Data{"mountpoint": mountPath})
		return voldriver.ErrorResponse{Err: "Error establishing whether volume exists"}
	}

	if !exists {
		errText := fmt.Sprintf("Volume %s does not exist (path: %s), nothing to do!", name, mountPath)
		logger.Error("failed-mountpoint-not-found", errors.New(errText))
		return voldriver.ErrorResponse{Err: errText}
	}

	if !ephemeral {
		d.volumes[name].MountCount--
	}

	if ephemeral || (d.volumes[name].MountCount == 0) {
		logger.Info("unmount-volume-folder", lager.Data{"mountpath": mountPath})
		err := d.mounter.Unmount(mountPath, 0)
		if err != nil {
			logger.Error("unmount-failed", err)
			return voldriver.ErrorResponse{Err: fmt.Sprintf("Error unmounting volume: %s", err.Error())}
		}
		err = d.os.RemoveAll(mountPath)
		if err != nil {
			logger.Error("create-mountdir-failed", err)
			return voldriver.ErrorResponse{Err: fmt.Sprintf("Error creating mountpoint: %s", err.Error())}
		}
	} else {
		logger.Info("volume-still-in-use", lager.Data{"name": name, "count": d.volumes[name].MountCount})
		return voldriver.ErrorResponse{}
	}

	logger.Info("unmounted-volume")

	if !ephemeral {
		d.volumes[name].Mountpoint = ""
	}

	return voldriver.ErrorResponse{}
}
