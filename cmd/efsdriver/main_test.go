package main_test

import (
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Main", func() {
	var (
		session *gexec.Session
		command *exec.Cmd
		err     error
	)

	BeforeEach(func() {
		command = exec.Command(driverPath)
	})

	JustBeforeEach(func() {
		session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		session.Kill().Wait()
	})

	Context("with a driver path", func() {
		var dir string

		BeforeEach(func() {
			dir, err = ioutil.TempDir("", "driversPath")
			Expect(err).ToNot(HaveOccurred())

			command.Args = append(command.Args, "-driversPath="+dir)
			command.Args = append(command.Args, "-transport=tcp-json")
			command.Args = append(command.Args, `-availabilityZone="foo-foo-2a"`)
		})

		It("listens on tcp/9750 by default", func() {
			EventuallyWithOffset(1, func() error {
				_, err := net.Dial("tcp", "127.0.0.1:9750")
				return err
			}, 5).ShouldNot(HaveOccurred())

			Eventually(func() string {
				specFile := filepath.Join(dir, "efsdriver.json")
				_, err := os.Stat(specFile)
				if err != nil {
					return ""
				}

				specFileContents, err := ioutil.ReadFile(specFile)
				Expect(err).NotTo(HaveOccurred())
				return string(specFileContents)
			}, 1 * time.Minute, 5 * time.Millisecond).Should(MatchJSON(`{
					"Name": "efsdriver",
					"Addr": "http://127.0.0.1:9750",
					"TLSConfig": null,
					"UniqueVolumeIds": false
				}`))
		})

		Context("with unique volume IDs enabled", func() {
			BeforeEach(func() {
				command.Args = append(command.Args, "-uniqueVolumeIds")
			})

			It("sets the uniqueVolumeIds flag in the spec file", func() {
				specFile := filepath.Join(dir, "efsdriver.json")
				Eventually(func() string {
					_, err := os.Stat(specFile)
					if err != nil {
						return ""
					}

					specFileContents, err := ioutil.ReadFile(specFile)
					Expect(err).NotTo(HaveOccurred())
					return string(specFileContents)
				}, 1 * time.Minute, 5 * time.Millisecond).Should(MatchJSON(`{
					"Name": "efsdriver",
					"Addr": "http://127.0.0.1:9750",
					"TLSConfig": null,
					"UniqueVolumeIds": true
				}`))

			})
		})
	})
})
