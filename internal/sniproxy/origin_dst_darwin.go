package sniproxy

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

const (
	PF_INOUT    = 0
	PF_IN       = 1
	PF_OUT      = 2
	PF_INT      = 3
	diocNatlook = 0xc0204417
)

type pfiocNatlook struct {
	Saddr     [16]byte
	Daddr     [16]byte
	Rsaddr    [16]byte
	Rdaddr    [16]byte
	Sxport    [4]byte
	Dxport    [4]byte
	Af        uint8
	Proto     uint8
	Variant   uint8
	Direction uint8
}

func getOriginalDest(c net.Conn) (string, error) {
	tc, ok := c.(*net.TCPConn)
	if !ok {
		return "", fmt.Errorf("not a TCPConn")
	}

	f, err := tc.File()
	if err != nil {
		return "", err
	}
	f.Close()

	laddr := c.LocalAddr().(*net.TCPAddr)
	raddr := c.RemoteAddr().(*net.TCPAddr)

	pf, err := syscall.Open("/dev/pf", syscall.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("open /dev/pf: %w", err)
	}
	defer syscall.Close(pf)

	var nl pfiocNatlook
	copy(nl.Saddr[:], raddr.IP.To16())
	copy(nl.Daddr[:], laddr.IP.To16())
	nl.Sxport[0] = byte(raddr.Port >> 8)
	nl.Sxport[1] = byte(raddr.Port)
	nl.Dxport[0] = byte(laddr.Port >> 8)
	nl.Dxport[1] = byte(laddr.Port)
	nl.Af = syscall.AF_INET
	nl.Proto = syscall.IPPROTO_TCP
	nl.Direction = PF_OUT

	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(pf), uintptr(diocNatlook), uintptr(unsafe.Pointer(&nl))); e != 0 {
		nl.Af = syscall.AF_INET6
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(pf), uintptr(diocNatlook), uintptr(unsafe.Pointer(&nl))); e != 0 {
			return "", fmt.Errorf("DIOCNATLOOK failed")
		}
	}

	port := int(nl.Dxport[0])<<8 | int(nl.Dxport[1])
	ip := net.IP(nl.Rdaddr[:])
	return net.JoinHostPort(ip.String(), itoa(port)), nil
}
