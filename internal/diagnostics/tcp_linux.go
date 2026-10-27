//go:build linux

package diagnostics

import (
	"errors"
	"net"
	"syscall"
	"unsafe"
)

// readInfo reads kernel-side information from an accepted TCP socket.
func readInfo(conn net.Conn) (syscall.TCPInfo, error) {
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		return syscall.TCPInfo{}, errors.New("not a TCP socket")
	}
	raw, err := tcp.SyscallConn()
	if err != nil {
		return syscall.TCPInfo{}, err
	}
	var info syscall.TCPInfo
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		size := uint32(unsafe.Sizeof(info))
		_, _, errno := syscall.Syscall6(
			syscall.SYS_GETSOCKOPT,
			fd,
			uintptr(syscall.IPPROTO_TCP),
			uintptr(syscall.TCP_INFO),
			uintptr(unsafe.Pointer(&info)),
			uintptr(unsafe.Pointer(&size)),
			0,
		)
		if errno != 0 {
			socketErr = errno
		}
	}); err != nil {
		return syscall.TCPInfo{}, err
	}
	if socketErr != nil {
		return syscall.TCPInfo{}, socketErr
	}
	return info, nil
}

// RetransmissionTotal returns the 32-bit Linux kernel total-retrans counter
// for this accepted TCP connection, independent of a test session.
func RetransmissionTotal(conn net.Conn) (uint32, error) {
	info, err := readInfo(conn)
	if err != nil {
		return 0, err
	}
	return info.Total_retrans, nil
}

// TCPInfo reports kernel-side statistics for the accepted server socket.
// Total retransmissions cover the connection lifetime, not a test window.
func TCPInfo(conn net.Conn) (map[string]any, error) {
	info, err := readInfo(conn)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"status":                   "completed",
		"scope":                    "server_tcp_peer",
		"note":                     "Behind a terminating reverse proxy these metrics describe the proxy-to-server TCP connection.",
		"rttMs":                    float64(info.Rtt) / 1000,
		"rttVarianceMs":            float64(info.Rttvar) / 1000,
		"rtoMs":                    float64(info.Rto) / 1000,
		"totalRetransmissions":     info.Total_retrans,
		"sendMssBytes":             info.Snd_mss,
		"receiveMssBytes":          info.Rcv_mss,
		"pathMtuBytes":             info.Pmtu,
		"congestionWindowSegments": info.Snd_cwnd,
	}, nil
}
