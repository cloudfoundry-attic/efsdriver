package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"

	"testing"
)

func TestEfsdriver(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Efs Main Suite")
}

var driverPath string

var _ = BeforeSuite(func() {
	var err error
	driverPath, err = Build("code.cloudfoundry.org/efsdriver/cmd/efsdriver")
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	CleanupBuildArtifacts()
})
