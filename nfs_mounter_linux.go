package efsdriver

import (
	"os/exec"
)

type NfsMounter struct{}

func (*NfsMounter) Mount(source string, target string, fstype string, flags uintptr, data string) ([]byte, error) {
	// mount -t nfs4 -o vers=4.1 mount-target-ip:/  ~/efs-mount-point
	cmd := exec.Command("mount", "-t", "nfs4", "-o", "vers=4.1", source, target)
	return cmd.CombinedOutput()
}
func (*NfsMounter) Unmount(target string, flags int) (err error) {
	cmd := exec.Command("umount", target)
	return cmd.Run()
}
