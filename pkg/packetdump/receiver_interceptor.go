// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package packetdump

import (
	"github.com/pion/interceptor"
)

// ReceiverInterceptorFactory is a interceptor.Factory for a ReceiverInterceptor
type ReceiverInterceptorFactory struct {
	opts []PacketDumperOption
}

// NewReceiverInterceptor returns a new ReceiverInterceptor
func NewReceiverInterceptor(opts ...PacketDumperOption) (*ReceiverInterceptorFactory, error) {
	return &ReceiverInterceptorFactory{
		opts: opts,
	}, nil
}

// NewInterceptor returns a new ReceiverInterceptor interceptor.
func (r *ReceiverInterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	dumper, err := NewPacketDumper(r.opts...)
	if err != nil {
		return nil, err
	}
	i := &ReceiverInterceptor{
		NoOp:         interceptor.NoOp{},
		PacketDumper: dumper,
	}

	return i, nil
}

// ReceiverInterceptor interceptor dumps outgoing RTP packets.
type ReceiverInterceptor struct {
	interceptor.NoOp
	*PacketDumper
}

// BindRemoteStream lets you modify any incoming RTP packets. It is called once for per RemoteStream. The returned method
// will be called once per rtp packet.
func (r *ReceiverInterceptor) BindRemoteStream(_ *interceptor.StreamInfo, reader interceptor.RTPReader) interceptor.RTPReader {
	return interceptor.RTPReaderFunc(func(bytes []byte, attributes interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(bytes, attributes)
		if err != nil {
			return 0, nil, err
		}

		if attr == nil {
			attr = make(interceptor.Attributes)
		}
		header, err := attr.GetRTPHeader(bytes)
		if err != nil {
			return 0, nil, err
		}

		r.logRTPPacket(header, bytes[header.MarshalSize():i], attr)
		return i, attr, nil
	})
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once per sender/receiver, however this might
// change in the future. The returned method will be called once per packet batch.
func (r *ReceiverInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(bytes []byte, attributes interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(bytes, attributes)
		if err != nil {
			return 0, nil, err
		}

		if attr == nil {
			attr = make(interceptor.Attributes)
		}
		pkts, err := attr.GetRTCPPackets(bytes[:i])
		if err != nil {
			return 0, nil, err
		}

		r.logRTCPPackets(pkts, attr)
		return i, attr, err
	})
}

// Close closes the interceptor
func (r *ReceiverInterceptor) Close() error {
	return r.PacketDumper.Close()
}
