package ecn

import (
	"net"
	"reflect"
	"syscall"
)

// EnableExplicitCongestionNotification enables ECN on the given connection.
func EnableExplicitCongestionNotification(conn *net.UDPConn) error {
	ptrVal := reflect.ValueOf(*conn)
	fdmember := reflect.Indirect(ptrVal).FieldByName("fd")
	pfdmember := reflect.Indirect(fdmember).FieldByName("pfd")
	netfdmember := reflect.Indirect(pfdmember).FieldByName("Sysfd")
	fd := int(netfdmember.Int())
	return syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_RECVTOS, 1)
}
