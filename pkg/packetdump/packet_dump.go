// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package packetdump implements RTP & RTCP packet dumpers.
package packetdump

import (
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

type rtpDump struct {
	attributes interceptor.Attributes
	packet     *rtp.Packet
}

type rtcpDump struct {
	attributes interceptor.Attributes
	packets    []rtcp.Packet
}
