package twcc

import (
	"sync/atomic"

	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/rtp"
)

// NewHeaderExtensionInterceptor returns a SenderInterceptor that adds a rtp.TransportCCExtension header
func NewHeaderExtensionInterceptor(defaultHdrExtID uint8) (*HeaderExtensionInterceptor, error) {
	return &HeaderExtensionInterceptor{defaultHdrExtID: defaultHdrExtID}, nil
}

// HeaderExtensionInterceptor adds transport wide sequence numbers as header extension to each RTP packet
type HeaderExtensionInterceptor struct {
	nextSequenceNr  uint32
	defaultHdrExtID uint8
}

// Transform transforms a given set of receiver interceptor pipes.
func (h *HeaderExtensionInterceptor) Transform(rtpSink rtpio.RTPWriter, rtcpSink rtpio.RTCPWriter, rtcpSrc rtpio.RTCPReader) rtpio.RTPWriter {
	go rtpio.ConsumeRTCP(rtcpSrc)

	return &headerExtensionRTPWriter{
		interceptor: h,
		rtpSink:     rtpSink,
	}
}

// Close does nothing.
func (h *HeaderExtensionInterceptor) Close() error {
	return nil
}

type headerExtensionRTPWriter struct {
	interceptor *HeaderExtensionInterceptor
	rtpSink     rtpio.RTPWriter
}

// WriteRTP returns a writer that adds a rtp.TransportCCExtension
// header with increasing sequence numbers to each outgoing packet.
func (h *headerExtensionRTPWriter) WriteRTP(pkt *rtp.Packet) (int, error) {
	sequenceNumber := atomic.AddUint32(&h.interceptor.nextSequenceNr, 1) - 1

	tcc, err := (&rtp.TransportCCExtension{TransportSequence: uint16(sequenceNumber)}).Marshal()
	if err != nil {
		return 0, err
	}
	if pkt.Header.SetExtension(h.interceptor.defaultHdrExtID, tcc) != nil {
		return 0, err
	}
	return h.rtpSink.WriteRTP(pkt)
}
