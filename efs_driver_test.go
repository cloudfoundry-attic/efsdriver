package efsdriver_test

import (
	"errors"
	"fmt"
	"os"

	"code.cloudfoundry.org/efsdriver"
	"code.cloudfoundry.org/goshims/filepath/filepath_fake"
	"code.cloudfoundry.org/goshims/os/os_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/voldriver"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"code.cloudfoundry.org/efsdriver/efsdriverfakes"
)

var _ = Describe("Efs Driver", func() {
	var logger lager.Logger
	var fakeOs *os_fake.FakeOs
	var fakeFilepath *filepath_fake.FakeFilepath
	var fakeMounter *efsdriverfakes.FakeMounter
	var efsDriver *efsdriver.EfsDriver
	var mountDir string
	const volumeName = "test-volume-id"


	BeforeEach(func() {
		logger = lagertest.NewTestLogger("efsdriver-local")

		mountDir = "/path/to/mount"

		fakeOs = &os_fake.FakeOs{}
		fakeFilepath = &filepath_fake.FakeFilepath{}
		fakeMounter = &efsdriverfakes.FakeMounter{}
		efsDriver = efsdriver.NewEfsDriver(fakeOs, fakeFilepath, mountDir, fakeMounter)
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
			BeforeEach(func() {
				createSuccessful(logger, efsDriver, fakeOs, volumeName, "")
				mountSuccessful(logger, efsDriver, volumeName, fakeFilepath, "")
			})

			AfterEach(func() {
				unmountSuccessful(logger, efsDriver, volumeName)
				removeSuccessful(logger, efsDriver, volumeName)
			})

			It("should mount the volume on the efs filesystem", func() {
				Expect(fakeFilepath.AbsCallCount()).To(Equal(1))
				Expect(fakeMounter.MountCallCount()).To(Equal(1))
				from, to, fstype, _, data := fakeMounter.MountArgsForCall(0)
				Expect(from).To(Equal("1.1.1.1:/"))
				Expect(to).To(Equal("/path/to/mount/_mounts/"+volumeName))
				Expect(fstype).To(Equal("nfs4"))
				Expect(data).To(Equal("rw"))
			})

			It("returns the mount point on a /VolumeDriver.Get response", func() {
				getResponse := getSuccessful(logger, efsDriver, volumeName)
				Expect(getResponse.Volume.Mountpoint).To(Equal("/path/to/mount/_mounts/"+volumeName))
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

				It("/VolumeDriver.Unmount unmounts", func() {
					unmountSuccessful(logger, efsDriver, volumeName)
					Expect(fakeMounter.UnmountCallCount()).To(Equal(1))
					removed, _ := fakeMounter.UnmountArgsForCall(0)
					Expect(removed).To(Equal("/path/to/mount/_mounts/"+volumeName))
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
						Expect(unmountResponse.Err).To(Equal("Volume " + volumeName + " does not exist (path: /path/to/mount/_mounts/" + volumeName + "), nothing to do!"))
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
		Context("when a second create is called with the same volume ID", func() {
			BeforeEach(func() {
				createSuccessful(logger, efsDriver, fakeOs, "volume", "")
			})

			Context("with the same opts", func() {
				It("does nothing", func() {
					createSuccessful(logger, efsDriver, fakeOs, "volume", "")
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
				Expect(pathResponse.Mountpoint).To(Equal("/path/to/mount/_mounts/"+volumeName))
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
			Expect(removeResponse.Err).To(Equal("Volume '" + volumeName + "' not found"))
		})

		Context("when the volume has been created", func() {
			BeforeEach(func() {
				createSuccessful(logger, efsDriver, fakeOs, volumeName, "")
			})

			It("/VolumePlugin.Remove succeeds", func() {
				removeResponse := efsDriver.Remove(logger, voldriver.RemoveRequest{
					Name: volumeName,
				})
				Expect(removeResponse.Err).To(Equal(""))

				getUnsuccessful(logger, efsDriver, volumeName)
			})

			Context("when volume has been mounted", func() {
				It("/VolumePlugin.Remove unmounts volume", func() {
					mountSuccessful(logger, efsDriver, volumeName, fakeFilepath, "")

					removeResponse := efsDriver.Remove(logger, voldriver.RemoveRequest{
						Name: volumeName,
					})
					Expect(removeResponse.Err).To(Equal(""))
					Expect(fakeMounter.UnmountCallCount()).To(Equal(1))

					getUnsuccessful(logger, efsDriver, volumeName)
				})
			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				removeResponse := efsDriver.Remove(logger, voldriver.RemoveRequest{
					Name: volumeName,
				})
				Expect(removeResponse.Err).To(Equal("Volume '" + volumeName + "' not found"))
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
	opts := map[string]interface{}{"ip": "1.1.1.1"}
	createResponse := efsDriver.Create(logger, voldriver.CreateRequest{
		Name: volumeName,
		Opts: opts,
	})
	Expect(createResponse.Err).To(Equal(""))
}

func mountSuccessful(logger lager.Logger, efsDriver voldriver.Driver, volumeName string, fakeFilepath *filepath_fake.FakeFilepath, passcode string) {
	fakeFilepath.AbsReturns("/path/to/mount/", nil)
	opts := map[string]interface{}{}
	mountResponse := efsDriver.Mount(logger, voldriver.MountRequest{
		Name: volumeName,
		Opts: opts,
	})
	Expect(mountResponse.Err).To(Equal(""))
	Expect(mountResponse.Mountpoint).To(Equal("/path/to/mount/_mounts/"+volumeName))
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
