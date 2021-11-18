// Package interceptor contains the Interceptor interface, with some useful interceptors that should be safe to use
// in most cases.
package interceptor

import (
	"io"

	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/rtcp"
)

// SenderInterceptor is an interceptor intended to postprocess packets before they are sent.
type SenderInterceptor interface {
	io.Closer

	Transform(rtpSink rtpio.RTPWriter, rtcpSink rtpio.RTCPWriter, rtcpSrc rtpio.RTCPReader) rtpio.RTPWriter // rtp push src.
}

// SenderChain creates a new SenderChainInter
func SenderChain(i ...SenderInterceptor) SenderInterceptor {
	return &SenderChainInterceptor{SenderInterceptors: i}
}

// SenderChainInterceptor is a chain of interceptors that composes a list of child interceptors.
type SenderChainInterceptor struct {
	SenderInterceptors []SenderInterceptor
}

// Transform transforms a given set of sender interceptor pipes.
func (i *SenderChainInterceptor) Transform(rtpSink rtpio.RTPWriter, rtcpSink rtpio.RTCPWriter, rtcpSrc rtpio.RTCPReader) rtpio.RTPWriter {
	rtcpReaders := make([]rtpio.RTCPReader, len(i.SenderInterceptors))
	rtcpWriters := make([]rtpio.RTCPWriter, len(i.SenderInterceptors))
	for i := range i.SenderInterceptors {
		rtcpReaders[i], rtcpWriters[i] = rtpio.RTCPPipe()
	}
	go func() {
		for {
			pkts := make([]rtcp.Packet, 16)
			_, err := rtcpSrc.ReadRTCP(pkts)
			if err != nil {
				return
			}
			for i := range i.SenderInterceptors {
				_, writeErr := rtcpWriters[i].WriteRTCP(pkts)
				if writeErr != nil {
					continue
				}
			}
		}
	}()
	for i, interceptor := range i.SenderInterceptors {
		rtpSink = interceptor.Transform(rtpSink, rtcpSink, rtcpReaders[i])
	}
	return rtpSink
}

// Close closes all of the child interceptors
func (i *SenderChainInterceptor) Close() error {
	for _, interceptor := range i.SenderInterceptors {
		if err := interceptor.Close(); err != nil {
			return err
		}
	}
	return nil
}

// TransformSender is a convenience method to pipe over a generic io.ReadWriter with a multiplexer.
func TransformSender(rw io.ReadWriter, interceptor SenderInterceptor, mtu int) rtpio.RTPWriter {
	rtpWriter, rtcpWriter := rtpio.NewRTPRTCPMultiplexer(rw)
	return interceptor.Transform(rtpWriter, rtcpWriter, rtpio.NewRTCPReader(rw, mtu))
}

// ReceiverInterceptor is an interceptor intended to preprocess packets before they are received.
type ReceiverInterceptor interface {
	io.Closer

	Transform(rtcpSink rtpio.RTCPWriter, rtpSrc rtpio.RTPReader, rtcpSrc rtpio.RTCPReader) rtpio.RTPReader // rtp pull sink.
}

// ReceiverChain creates a new ReceiverChainInterceptor
func ReceiverChain(i ...ReceiverInterceptor) ReceiverInterceptor {
	return &ReceiverChainInterceptor{ReceiverInterceptors: i}
}

// ReceiverChainInterceptor is a chain of interceptors that composes a list of child interceptors.
type ReceiverChainInterceptor struct {
	ReceiverInterceptors []ReceiverInterceptor
}

// Transform transforms a given set of receiver interceptor pipes.
func (i *ReceiverChainInterceptor) Transform(rtcpSink rtpio.RTCPWriter, rtpSrc rtpio.RTPReader, rtcpSrc rtpio.RTCPReader) rtpio.RTPReader {
	rtcpReaders := make([]rtpio.RTCPReader, len(i.ReceiverInterceptors))
	rtcpWriters := make([]rtpio.RTCPWriter, len(i.ReceiverInterceptors))
	for i := range i.ReceiverInterceptors {
		rtcpReaders[i], rtcpWriters[i] = rtpio.RTCPPipe()
	}
	go func() {
		for {
			pkts := make([]rtcp.Packet, 16)
			_, err := rtcpSrc.ReadRTCP(pkts)
			if err != nil {
				return
			}
			for i := range i.ReceiverInterceptors {
				_, writeErr := rtcpWriters[i].WriteRTCP(pkts)
				if writeErr != nil {
					continue
				}
			}
		}
	}()
	for i, interceptor := range i.ReceiverInterceptors {
		rtpSrc = interceptor.Transform(rtcpSink, rtpSrc, rtcpReaders[i])
	}
	return rtpSrc
}

// Close closes all of the child interceptors
func (i *ReceiverChainInterceptor) Close() error {
	for _, interceptor := range i.ReceiverInterceptors {
		if err := interceptor.Close(); err != nil {
			return err
		}
	}
	return nil
}

// TransformReceiver is a convenience method to pipe over a generic io.ReadWriter with a demultiplexer.
func TransformReceiver(rw io.ReadWriter, interceptor ReceiverInterceptor, mtu int) rtpio.RTPReader {
	rtpReader, rtcpReader := rtpio.NewRTPRTCPDemultiplexer(rw, mtu)
	return interceptor.Transform(rtpio.NewRTCPWriter(rw), rtpReader, rtcpReader)
}
