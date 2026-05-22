package sniproxy

import (
	"fmt"
	"net"
)

func getOriginalDest(c net.Conn) (string, error) {
	return "", fmt.Errorf("transparent proxy not supported on Windows")
}
