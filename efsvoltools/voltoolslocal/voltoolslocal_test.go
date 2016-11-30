package voltoolslocal_test

import (
	"context"

	"code.cloudfoundry.org/efsdriver/efsvoltools"
	"code.cloudfoundry.org/efsdriver/efsvoltools/voltoolslocal"
	"code.cloudfoundry.org/goshims/execshim/exec_fake"
	"code.cloudfoundry.org/goshims/filepathshim/filepath_fake"
	"code.cloudfoundry.org/goshims/ioutilshim/ioutil_fake"
	"code.cloudfoundry.org/goshims/osshim/os_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/nfsdriver/nfsdriverfakes"
	"code.cloudfoundry.org/voldriver"
	"code.cloudfoundry.org/voldriver/driverhttp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Efs Driver", func() {
	var logger lager.Logger
	var ctx context.Context
	var env voldriver.Env
	var fakeOs *os_fake.FakeOs
	var fakeFilepath *filepath_fake.FakeFilepath
	var fakeIoutil *ioutil_fake.FakeIoutil
	var fakeMounter *nfsdriverfakes.FakeMounter
	var fakeCmd *exec_fake.FakeCmd
	var efsDriver *voltoolslocal.EfsVolToolsLocal
	var mountDir string
	const volumeName = "test-volume-id"

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("efsdriver-local")
		ctx = context.TODO()
		env = driverhttp.NewHttpDriverEnv(logger, ctx)

		mountDir = "/path/to/mount"

		fakeOs = &os_fake.FakeOs{}
		fakeFilepath = &filepath_fake.FakeFilepath{}
		fakeIoutil = &ioutil_fake.FakeIoutil{}
		fakeMounter = &nfsdriverfakes.FakeMounter{}
		fakeCmd = &exec_fake.FakeCmd{}
	})

	Context("created", func() {
		BeforeEach(func() {
			efsDriver = voltoolslocal.NewEfsVolToolsLocal(fakeOs, fakeFilepath, fakeIoutil, mountDir, fakeMounter)
		})

		Describe("OpenPerms", func() {

			Context("when the volume has been created", func() {
				BeforeEach(func() {
					openPermsSuccessful(env, efsDriver, fakeFilepath, volumeName, "")
				})

				It("should mount the volume on the efs filesystem", func() {
					Expect(fakeFilepath.AbsCallCount()).To(Equal(1))
					Expect(fakeMounter.MountCallCount()).To(Equal(1))
					_, from, to, _ := fakeMounter.MountArgsForCall(0)
					Expect(from).To(Equal("1.1.1.1:/"))
					Expect(to).To(Equal("/path/to/mount/" + volumeName))
				})
			})
		})
	})
})

func openPermsSuccessful(env voldriver.Env, tools efsvoltools.VolTools, fakeFilepath *filepath_fake.FakeFilepath, volumeName string, passcode string) {
	fakeFilepath.AbsReturns("/path/to/mount/", nil)
	opts := map[string]interface{}{"ip": "1.1.1.1"}
	response := tools.OpenPerms(env, efsvoltools.OpenPermsRequest{
		Name: volumeName,
		Opts: opts,
	})
	Expect(response.Err).To(Equal(""))
}
