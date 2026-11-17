//go:build windows

package shots

import "syscall"

// hideDir sets the hidden attribute so ".screenmit" stays out of the way in
// Explorer, like it would on other systems.
func hideDir(p string) {
	u, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return
	}
	attrs, err := syscall.GetFileAttributes(u)
	if err != nil {
		return
	}
	_ = syscall.SetFileAttributes(u, attrs|syscall.FILE_ATTRIBUTE_HIDDEN)
}
