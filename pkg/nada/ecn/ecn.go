// Package ecn provides ExplicitCongestionNotification (ECN) support.
package ecn

import (
	"errors"
	"syscall"
)

var errNoECN = errors.New("no ECN control message")

// CheckExplicitCongestionNotification checks if the given oob data includes an ECN bit set.
func CheckExplicitCongestionNotification(oob []byte) (uint8, error) {
	ctrlMsgs, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return 0, err
	}
	for _, ctrlMsg := range ctrlMsgs {
		if ctrlMsg.Header.Type == syscall.IP_TOS {
			return (ctrlMsg.Data[0] & 0x3), nil
		}
	}
	return 0, errNoECN
}
