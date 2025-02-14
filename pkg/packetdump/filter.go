// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package packetdump

import (
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// RTPFilterCallback can be used to filter RTP packets to dump.
// The callback returns whether or not to print dump the packet's content.
type RTPFilterCallback func(pkt *rtp.Packet) bool

// RTCPFilterCallback can be used to filter RTCP packets to dump.
// The callback returns whether or not to print dump the packet's content.
// Deprecated: prefer RTCPPerPacketFilterCallback.
type RTCPFilterCallback func(pkt []rtcp.Packet) bool

// RTCPPerPacketFilterCallback can be used to filter RTCP packets to dump.
// It's called once per every packet opposing to RTCPFilterCallback which is called once per packet batch.
type RTCPPerPacketFilterCallback func(pkt rtcp.Packet) bool
