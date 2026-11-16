// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package interceptor

// RTPHeaderExtension represents a negotiated RFC5285 RTP header extension.
type RTPHeaderExtension struct {
	URI string
	ID  int
}

// StreamInfo is the Context passed when a StreamLocal or StreamRemote has been Binded or Unbinded.
type StreamInfo struct {
	ID                                string
	Attributes                        Attributes
	SSRC                              uint32
	SSRCRetransmission                uint32
	SSRCForwardErrorCorrection        uint32
	PayloadType                       uint8
	PayloadTypeRetransmission         uint8
	PayloadTypeForwardErrorCorrection uint8
	RTPHeaderExtensions               []RTPHeaderExtension
	MimeType                          string
	ClockRate                         uint32
	Channels                          uint16
	SDPFmtpLine                       string
	RTCPFeedback                      []RTCPFeedback

	// RetransmissionPayloadTypes maps media payload types to their RTX payload types
	// (RFC 4588). It lets a stream whose codec changes while bound retransmit each
	// packet with the RTX payload type of that packet's codec. When the map is set,
	// payload types missing from it have no RTX and are retransmitted on SSRC as-is.
	// When the map is nil, every packet uses PayloadTypeRetransmission.
	RetransmissionPayloadTypes map[uint8]uint8
}

// RTCPFeedback signals the connection to use additional RTCP packet types.
// https://draft.ortc.org/#dom-rtcrtcpfeedback
type RTCPFeedback struct {
	// Type is the type of feedback.
	// see: https://draft.ortc.org/#dom-rtcrtcpfeedback
	// valid: ack, ccm, nack, goog-remb, transport-cc
	Type string

	// The parameter value depends on the type.
	// For example, type="nack" parameter="pli" will send Picture Loss Indicator packets.
	Parameter string
}
