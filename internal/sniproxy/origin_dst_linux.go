package sniproxy

import (
	"fmt"
	"net"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

func getOriginalDest(c net.Conn) (string, error) {
	tc, ok := c.(*net.TCPConn)
	if !ok {
		return "", fmt.Errorf("not a TCPConn")
	}
	f, err := tc.File()
	if err != nil {
		return "", err
	}
	defer f.Close()

	var a unix.RawSockaddrInet4
	al := uint32(unix.SizeofSockaddrInet4)
	_, _, e := unix.Syscall6(unix.SYS_GETSOCKOPT, f.Fd(), 0, 80,
		uintptr(unsafe.Pointer(&a)), uintptr(unsafe.Pointer(&al)), 0)
	runtime.KeepAlive(f)
	if e == 0 {
		ip := net.IP(a.Addr[:])
		return net.JoinHostPort(ip.String(), itoa(int(a.Port>>8)|int(a.Port&0xff)<<8)), nil
	}

	var a6 unix.RawSockaddrInet6
	al6 := uint32(unix.SizeofSockaddrInet6)
	_, _, e = unix.Syscall6(unix.SYS_GETSOCKOPT, f.Fd(), 41, 80,
		uintptr(unsafe.Pointer(&a6)), uintptr(unsafe.Pointer(&al6)), 0)
	runtime.KeepAlive(f)
	if e != 0 {
		return "", fmt.Errorf("SO_ORIGINAL_DST failed")
	}
	ip := net.IP(a6.Addr[:])
	return net.JoinHostPort(ip.String(), itoa(int(a6.Port>>8)|int(a6.Port&0xff)<<8)), nil
}
