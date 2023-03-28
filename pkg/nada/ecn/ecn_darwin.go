package ecn

import (
	"net"
)

// EnableExplicitCongestionNotification enables ECN on the given connection.
func EnableExplicitCongestionNotification(conn *net.UDPConn) error {
	// noop.
	return nil
}
