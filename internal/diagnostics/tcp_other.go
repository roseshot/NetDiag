//go:build !linux

package diagnostics

import (
	"errors"
	"net"
)

func TCPInfo(_ net.Conn) (map[string]any, error) {
	return nil, errors.New("TCP_INFO is only available on Linux")
}

func RetransmissionTotal(_ net.Conn) (uint32, error) {
	return 0, errors.New("TCP_INFO is only available on Linux")
}
