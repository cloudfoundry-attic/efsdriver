package efsdriver

import "golang.org/x/sys/unix"

type NfsMounter struct {}
func (*NfsMounter) Mount(source string, target string, fstype string, flags uintptr, data string) (err error) {
	return unix.Mount(source, target, fstype, flags, data)
}
func (*NfsMounter) Unmount(target string, flags int) (err error) {
	return unix.Unmount(target, flags)
}

