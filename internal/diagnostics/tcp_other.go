//go:build !linux

package diagnostics

import (
	"errors"
	"net"
)

func TCPInfo(_ net.Conn) (map[string]any, error) {
	return nil, errors.New("TCP_INFO is only available on Linux")
}
