package efsdriver_test

import (
	"errors"
	"fmt"
	"os"
	"path"

	"code.cloudfoundry.org/goshims/filepath/filepath_fake"
	"code.cloudfoundry.org/goshims/os/os_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/efsdriver"
	"code.cloudfoundry.org/voldriver"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Efs Driver", func() {
	var logger lager.Logger
	var fakeOs *os_fake.FakeOs
	var fakeFilepath *filepath_fake.FakeFilepath
	var efsDriver *efsdriver.EfsDriver
	var mountDir string

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("efsdriver-local")

		mountDir = "/path/to/mount"

		fakeOs = &os_fake.FakeOs{}
		fakeFilepath = &filepath_fake.FakeFilepath{}
		efsDriver = efsdriver.NewEfsDriver(fakeOs, fakeFilepath, mountDir)
	})

	Describe("#Activate", func() {
		It("returns Implements: VolumeDriver", func() {
			activateResponse := efsDriver.Activate(logger)
			Expect(len(activateResponse.Implements)).To(BeNumerically(">", 0))
			Expect(activateResponse.Implements[0]).To(Equal("VolumeDriver"))
		})
	})

	Describe("Mount", func() {

		Context("when the volume has been created", func() {
			const volumeName = "test-volume-name"

			BeforeEach(func() {
				createSuccessful(logger, efsDriver, fakeOs, volumeName, "")
				mountSuccessful(logger, efsDriver, volumeName, fakeFilepath, "")
			})

			AfterEach(func() {
				unmountSuccessful(logger, efsDriver, volumeName)
				removeSuccessful(logger, efsDriver, volumeName)
			})

			It("should mount the volume on the efs filesystem", func() {
				Expect(fakeFilepath.AbsCallCount()).To(Equal(3))
				Expect(fakeOs.MkdirAllCallCount()).To(Equal(4))
				Expect(fakeOs.SymlinkCallCount()).To(Equal(1))
				from, to := fakeOs.SymlinkArgsForCall(0)
				Expect(from).To(Equal("/path/to/mount/_volumes/test-volume-id"))
				Expect(to).To(Equal("/path/to/mount/_mounts/test-volume-id"))
			})

			It("returns the mount point on a /VolumeDriver.Get response", func() {
				getResponse := getSuccessful(logger, efsDriver, volumeName)
				Expect(getResponse.Volume.Mountpoint).To(Equal("/path/to/mount/_mounts/test-volume-id"))
			})
		})

		Context("when the volume has been created with a passcode", func() {
			const volumeName = "test-volume-name"
			const passcode = "aPassc0de"

			BeforeEach(func() {
				createSuccessful(logger, efsDriver, fakeOs, volumeName, passcode)
			})

			AfterEach(func() {
				removeSuccessful(logger, efsDriver, volumeName)
			})

			Context("when mounting with the right passcode", func() {
				BeforeEach(func() {
					mountSuccessful(logger, efsDriver, volumeName, fakeFilepath, passcode)
				})
				AfterEach(func() {
					unmountSuccessful(logger, efsDriver, volumeName)
				})

				It("should mount the volume on the efs filesystem", func() {
					Expect(fakeFilepath.AbsCallCount()).To(Equal(3))
					Expect(fakeOs.MkdirAllCallCount()).To(Equal(4))
					Expect(fakeOs.SymlinkCallCount()).To(Equal(1))
					from, to := fakeOs.SymlinkArgsForCall(0)
					Expect(from).To(Equal("/path/to/mount/_volumes/test-volume-id"))
					Expect(to).To(Equal("/path/to/mount/_mounts/test-volume-id"))
				})

				It("returns the mount point on a /VolumeDriver.Get response", func() {
					getResponse := getSuccessful(logger, efsDriver, volumeName)
					Expect(getResponse.Volume.Mountpoint).To(Equal("/path/to/mount/_mounts/test-volume-id"))
				})
			})

			Context("when mounting with the wrong passcode", func() {
				It("returns an error", func() {
					mountResponse := efsDriver.Mount(logger, voldriver.MountRequest{
						Name: volumeName,
						Opts: map[string]interface{}{"passcode": "wrong"},
					})
					Expect(mountResponse.Err).To(Equal("Volume " + volumeName + " access denied"))
				})
			})

			Context("when mounting with the wrong passcode type", func() {
				It("returns an error", func() {
					mountResponse := efsDriver.Mount(logger, voldriver.MountRequest{
						Name: volumeName,
						Opts: map[string]interface{}{"passcode": nil},
					})
					Expect(mountResponse.Err).To(Equal("Opts.passcode must be a string value"))
				})
			})

			Context("when mounting with no passcode", func() {
				It("returns an error", func() {
					mountResponse := efsDriver.Mount(logger, voldriver.MountRequest{
						Name: volumeName,
					})
					Expect(mountResponse.Err).To(Equal("Volume " + volumeName + " requires a passcode"))
				})
			})

		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				mountResponse := efsDriver.Mount(logger, voldriver.MountRequest{
					Name: "bla",
				})
				Expect(mountResponse.Err).To(Equal("Volume 'bla' must be created before being mounted"))
			})
		})
	})

	Describe("Unmount", func() {
		const volumeName = "volumeName"

		Context("when a volume has been created", func() {
			BeforeEach(func() {
				createSuccessful(logger, efsDriver, fakeOs, volumeName, "")
			})

			Context("when a volume has been mounted", func() {
				BeforeEach(func() {
					mountSuccessful(logger, efsDriver, volumeName, fakeFilepath, "")
				})

				It("After unmounting /VolumeDriver.Get returns no mountpoint", func() {
					unmountSuccessful(logger, efsDriver, volumeName)
					getResponse := getSuccessful(logger, efsDriver, volumeName)
					Expect(getResponse.Volume.Mountpoint).To(Equal(""))
				})

				It("/VolumeDriver.Unmount doesn't remove mountpath from OS", func() {
					unmountSuccessful(logger, efsDriver, volumeName)
					Expect(fakeOs.RemoveCallCount()).To(Equal(1))
					removed := fakeOs.RemoveArgsForCall(0)
					Expect(removed).To(Equal("/path/to/mount/_mounts/test-volume-id"))
				})

				Context("when the same volume is mounted a second time then unmounted", func() {
					BeforeEach(func() {
						mountSuccessful(logger, efsDriver, volumeName, fakeFilepath, "")
						unmountSuccessful(logger, efsDriver, volumeName)
					})

					It("should not report empty mountpoint until unmount is called again", func() {
						getResponse := getSuccessful(logger, efsDriver, volumeName)
						Expect(getResponse.Volume.Mountpoint).NotTo(Equal(""))

						unmountSuccessful(logger, efsDriver, volumeName)
						getResponse = getSuccessful(logger, efsDriver, volumeName)
						Expect(getResponse.Volume.Mountpoint).To(Equal(""))
					})
				})
				Context("when the mountpath is not found on the filesystem", func() {
					var unmountResponse voldriver.ErrorResponse

					BeforeEach(func() {
						fakeOs.StatReturns(nil, os.ErrNotExist)
						unmountResponse = efsDriver.Unmount(logger, voldriver.UnmountRequest{
							Name: volumeName,
						})
					})

					It("returns an error", func() {
						Expect(unmountResponse.Err).To(Equal("Volume volumeName does not exist (path: /path/to/mount/_mounts/test-volume-id), nothing to do!"))
					})

					It("/VolumeDriver.Get still returns the mountpoint", func() {
						getResponse := getSuccessful(logger, efsDriver, volumeName)
						Expect(getResponse.Volume.Mountpoint).NotTo(Equal(""))
					})
				})

				Context("when the mountpath cannot be accessed", func() {
					var unmountResponse voldriver.ErrorResponse

					BeforeEach(func() {
						fakeOs.StatReturns(nil, errors.New("something weird"))
						unmountResponse = efsDriver.Unmount(logger, voldriver.UnmountRequest{
							Name: volumeName,
						})
					})

					It("returns an error", func() {
						Expect(unmountResponse.Err).To(Equal("Error establishing whether volume exists"))
					})

					It("/VolumeDriver.Get still returns the mountpoint", func() {
						getResponse := getSuccessful(logger, efsDriver, volumeName)
						Expect(getResponse.Volume.Mountpoint).NotTo(Equal(""))
					})
				})
			})

			Context("when the volume has not been mounted", func() {
				It("returns an error", func() {
					unmountResponse := efsDriver.Unmount(logger, voldriver.UnmountRequest{
						Name: volumeName,
					})

					Expect(unmountResponse.Err).To(Equal("Volume not previously mounted"))
				})
			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				unmountResponse := efsDriver.Unmount(logger, voldriver.UnmountRequest{
					Name: volumeName,
				})

				Expect(unmountResponse.Err).To(Equal(fmt.Sprintf("Volume '%s' not found", volumeName)))
			})
		})
	})

	Describe("Create", func() {
		Context("when a volume ID is not provided", func() {
			It("returns an error", func() {
				createResponse := efsDriver.Create(logger, voldriver.CreateRequest{
					Name: "volume",
					Opts: map[string]interface{}{
						"nonsense": "bla",
					},
				})

				Expect(createResponse.Err).To(Equal("Missing mandatory 'volume_id' field in 'Opts'"))
			})
		})

		Context("when a passcode is wrong type", func() {
			It("returns an error", func() {
				createResponse := efsDriver.Create(logger, voldriver.CreateRequest{
					Name: "volume",
					Opts: map[string]interface{}{
						"volume_id": "something_different_than_test",
						"passcode":  nil,
					},
				})

				Expect(createResponse.Err).To(Equal("Opts.passcode must be a string value"))
			})
		})

		Context("when a second create is called with the same volume ID", func() {
			BeforeEach(func() {
				createSuccessful(logger, efsDriver, fakeOs, "volume", "")
			})

			Context("with the same opts", func() {
				It("does nothing", func() {
					createSuccessful(logger, efsDriver, fakeOs, "volume", "")
				})
			})

			Context("with a different opts", func() {
				It("returns an error", func() {
					createResponse := efsDriver.Create(logger, voldriver.CreateRequest{
						Name: "volume",
						Opts: map[string]interface{}{
							"volume_id": "something_different_than_test",
						},
					})

					Expect(createResponse.Err).To(Equal("Volume 'volume' already exists with a different volume ID"))
				})
			})
		})
	})

	Describe("Get", func() {
		Context("when the volume has been created", func() {
			It("returns the volume name", func() {
				volumeName := "test-volume"
				createSuccessful(logger, efsDriver, fakeOs, volumeName, "")
				getSuccessful(logger, efsDriver, volumeName)
			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				volumeName := "test-volume"
				getUnsuccessful(logger, efsDriver, volumeName)
			})
		})
	})

	Describe("Path", func() {
		Context("when a volume is mounted", func() {
			var (
				volumeName string
			)
			BeforeEach(func() {
				volumeName = "my-volume"
				createSuccessful(logger, efsDriver, fakeOs, volumeName, "")
				mountSuccessful(logger, efsDriver, volumeName, fakeFilepath, "")
			})

			It("returns the mount point on a /VolumeDriver.Path", func() {
				pathResponse := efsDriver.Path(logger, voldriver.PathRequest{
					Name: volumeName,
				})
				Expect(pathResponse.Err).To(Equal(""))
				Expect(pathResponse.Mountpoint).To(Equal("/path/to/mount/_mounts/test-volume-id"))
			})
		})

		Context("when a volume is not created", func() {
			It("returns an error on /VolumeDriver.Path", func() {
				pathResponse := efsDriver.Path(logger, voldriver.PathRequest{
					Name: "volume-that-does-not-exist",
				})
				Expect(pathResponse.Err).NotTo(Equal(""))
				Expect(pathResponse.Mountpoint).To(Equal(""))
			})
		})

		Context("when a volume is created but not mounted", func() {
			var (
				volumeName string
			)
			BeforeEach(func() {
				volumeName = "my-volume"
				createSuccessful(logger, efsDriver, fakeOs, volumeName, "")
			})

			It("returns an error on /VolumeDriver.Path", func() {
				pathResponse := efsDriver.Path(logger, voldriver.PathRequest{
					Name: "volume-that-does-not-exist",
				})
				Expect(pathResponse.Err).NotTo(Equal(""))
				Expect(pathResponse.Mountpoint).To(Equal(""))
			})
		})
	})

	Describe("List", func() {
		Context("when there are volumes", func() {
			var volumeName string
			BeforeEach(func() {
				volumeName = "test-volume-id"
				createSuccessful(logger, efsDriver, fakeOs, volumeName, "")
			})

			It("returns the list of volumes", func() {
				listResponse := efsDriver.List(logger)

				Expect(listResponse.Err).To(Equal(""))
				Expect(listResponse.Volumes[0].Name).To(Equal(volumeName))

			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				volumeName := "test-volume"
				getUnsuccessful(logger, efsDriver, volumeName)
			})
		})
	})

	Describe("Remove", func() {
		const volumeName = "test-volume"

		It("should fail if no volume name provided", func() {
			removeResponse := efsDriver.Remove(logger, voldriver.RemoveRequest{
				Name: "",
			})
			Expect(removeResponse.Err).To(Equal("Missing mandatory 'volume_name'"))
		})

		It("should fail if no volume was created", func() {
			removeResponse := efsDriver.Remove(logger, voldriver.RemoveRequest{
				Name: volumeName,
			})
			Expect(removeResponse.Err).To(Equal("Volume 'test-volume' not found"))
		})

		Context("when the volume has been created", func() {
			BeforeEach(func() {
				createSuccessful(logger, efsDriver, fakeOs, volumeName, "")
			})

			It("/VolumePlugin.Remove destroys volume", func() {
				removeResponse := efsDriver.Remove(logger, voldriver.RemoveRequest{
					Name: volumeName,
				})
				Expect(removeResponse.Err).To(Equal(""))
				Expect(fakeOs.RemoveAllCallCount()).To(Equal(1))

				getUnsuccessful(logger, efsDriver, volumeName)
			})

			Context("when volume has been mounted", func() {
				It("/VolumePlugin.Remove unmounts and destroys volume", func() {
					mountSuccessful(logger, efsDriver, volumeName, fakeFilepath, "")

					removeResponse := efsDriver.Remove(logger, voldriver.RemoveRequest{
						Name: volumeName,
					})
					Expect(removeResponse.Err).To(Equal(""))
					Expect(fakeOs.RemoveCallCount()).To(Equal(1))
					Expect(fakeOs.RemoveAllCallCount()).To(Equal(1))

					getUnsuccessful(logger, efsDriver, volumeName)
				})
			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				removeResponse := efsDriver.Remove(logger, voldriver.RemoveRequest{
					Name: volumeName,
				})
				Expect(removeResponse.Err).To(Equal("Volume 'test-volume' not found"))
			})
		})
	})
})

func getUnsuccessful(logger lager.Logger, efsDriver voldriver.Driver, volumeName string) {
	getResponse := efsDriver.Get(logger, voldriver.GetRequest{
		Name: volumeName,
	})

	Expect(getResponse.Err).To(Equal("Volume not found"))
	Expect(getResponse.Volume.Name).To(Equal(""))
}

func getSuccessful(logger lager.Logger, efsDriver voldriver.Driver, volumeName string) voldriver.GetResponse {
	getResponse := efsDriver.Get(logger, voldriver.GetRequest{
		Name: volumeName,
	})

	Expect(getResponse.Err).To(Equal(""))
	Expect(getResponse.Volume.Name).To(Equal(volumeName))
	return getResponse
}

func createSuccessful(logger lager.Logger, efsDriver voldriver.Driver, fakeOs *os_fake.FakeOs, volumeName string, passcode string) {
	opts := map[string]interface{}{
		"volume_id": "test-volume-id",
	}
	if passcode != "" {
		opts["passcode"] = passcode
	}
	createResponse := efsDriver.Create(logger, voldriver.CreateRequest{
		Name: volumeName,
		Opts: opts,
	})
	Expect(createResponse.Err).To(Equal(""))

	Expect(fakeOs.MkdirAllCallCount()).Should(Equal(2))

	volumeDir, fileMode := fakeOs.MkdirAllArgsForCall(1)
	Expect(path.Base(volumeDir)).To(Equal("test-volume-id"))
	Expect(fileMode).To(Equal(os.ModePerm))
}

func mountSuccessful(logger lager.Logger, efsDriver voldriver.Driver, volumeName string, fakeFilepath *filepath_fake.FakeFilepath, passcode string) {
	fakeFilepath.AbsReturns("/path/to/mount/", nil)
	opts := map[string]interface{}{}
	if passcode != "" {
		opts["passcode"] = passcode
	}
	mountResponse := efsDriver.Mount(logger, voldriver.MountRequest{
		Name: volumeName,
		Opts: opts,
	})
	Expect(mountResponse.Err).To(Equal(""))
	Expect(mountResponse.Mountpoint).To(Equal("/path/to/mount/_mounts/test-volume-id"))
}

func unmountSuccessful(logger lager.Logger, efsDriver voldriver.Driver, volumeName string) {
	unmountResponse := efsDriver.Unmount(logger, voldriver.UnmountRequest{
		Name: volumeName,
	})
	Expect(unmountResponse.Err).To(Equal(""))
}

func removeSuccessful(logger lager.Logger, efsDriver voldriver.Driver, volumeName string) {
	removeResponse := efsDriver.Remove(logger, voldriver.RemoveRequest{
		Name: volumeName,
	})
	Expect(removeResponse.Err).To(Equal(""))
}
