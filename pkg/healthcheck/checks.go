package healthcheck

import (
	"net"
	"time"
)

// Reference https://godoc.org/net - https://godoc.org/github.com/heptiolabs/healthcheck#TCPDialCheck

// TCPCheck verifies TCP connectivity for the specified endpoint.
func TCPCheck(addr string, timeout time.Duration) error {
	// When using TCP, and the host resolves to multiple IP addresses, Dial will try each IP address in order until one succeeds.
	// The timeout includes name resolution, if required.
	// When using TCP, and the host in the address parameter resolves to multiple IP addresses,
	// the timeout is spread over each consecutive dial, such that each is given an appropriate fraction of the time to connect.
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	return conn.Close()
}
