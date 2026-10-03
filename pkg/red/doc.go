// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package red implements the RTP payload format for redundant audio data described by RFC 2198.
// It also provides interceptors for protecting and recovering Opus packets with copy redundancy.
//
// # Stream information
//
// The sender and receiver interceptors bind only to Opus streams that provide both negotiated
// payload types in interceptor.StreamInfo. PayloadType identifies Opus and
// PayloadTypeForwardErrorCorrection identifies RED. Both formats use StreamInfo.SSRC; RED does
// not use a separate FEC SSRC. SDP parsing and validation are the caller's responsibility.
//
// # Sender
//
// The sender retains owned copies of the previous two contiguous Opus payloads. It sends the
// first packet as plain Opus and uses RED only when at least one useful redundant block fits.
// By default, adding RED will not make the complete RTP packet larger than 1200 bytes. Use
// SenderMaxPacketSize to change that limit.
//
// # Receiver
//
// The receiver accepts plain Opus and RED on the same SSRC. For RED, it emits missing redundant
// Opus packets from oldest to newest and then emits the primary packet. It assumes each redundant
// block is a copy of an immediately preceding Opus RTP packet, which allows sequence numbers to be
// reconstructed from block order. The Payload parser itself can still parse general RFC 2198 data.
//
// RFC 2198 does not preserve the original marker bit, padding, or RTP header extensions of a
// redundant block. Recovered packets therefore omit those values. The current packet's CSRC list
// is retained as recommended by RFC 2198.
//
// The package does not negotiate SDP, advertise RTCP feedback, or choose whether NACK should be
// enabled. Those policies belong to the application or WebRTC integration that installs the
// interceptors.
package red
