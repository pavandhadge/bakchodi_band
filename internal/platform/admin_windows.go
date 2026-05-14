//go:build windows

package platform

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func IsAdmin() bool {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer token.Close()

	var isElevated uint32
	var outLen uint32
	err = windows.GetTokenInformation(
		token,
		windows.TokenElevation,
		(*byte)(unsafe.Pointer(&isElevated)),
		uint32(unsafe.Sizeof(isElevated)),
		&outLen,
	)
	return err == nil && isElevated != 0
}
