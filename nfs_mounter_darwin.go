package efsdriver

type NfsMounter struct {}
func (*NfsMounter) Mount(source string, target string, fstype string, flags uintptr, data string) (err error) {
	panic("unimplmented!")
}
func (*NfsMounter) Unmount(target string, flags int) (err error) {
	panic("unimplmented!")
}
