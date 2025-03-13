// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package test provides helpers for testing interceptors
package test

import (
	"errors"
	"io"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// MockStream is a helper struct for testing interceptors.
type MockStream struct {
	interceptor interceptor.Interceptor

	rtcpReader   interceptor.RTCPReader
	rtcpWriter   interceptor.RTCPWriter
	rtpReader    interceptor.RTPReader
	rtpProcessor interceptor.RTPProcessor
	rtpWriter    interceptor.RTPWriter

	rtcpIn chan []rtcp.Packet
	rtpIn  chan *rtp.Packet

	rtcpOutModified chan []rtcp.Packet
	rtpOutModified  chan *rtp.Packet

	rtcpInModified chan RTCPWithError
	rtpInModified  chan RTPWithError
}

// RTPWithError is used to send an rtp packet or an error on a channel.
type RTPWithError struct {
	Packet *rtp.Packet
	Err    error
}

// RTCPWithError is used to send a batch of rtcp packets or an error on a channel.
type RTCPWithError struct {
	Packets []rtcp.Packet
	Err     error
}

// NewMockStream creates a new MockStream.
func NewMockStream(info *interceptor.StreamInfo, i interceptor.Interceptor) *MockStream { //nolint
	mockStream := &MockStream{
		interceptor:     i,
		rtcpIn:          make(chan []rtcp.Packet, 1000),
		rtpIn:           make(chan *rtp.Packet, 1000),
		rtcpOutModified: make(chan []rtcp.Packet, 1000),
		rtpOutModified:  make(chan *rtp.Packet, 1000),
		rtcpInModified:  make(chan RTCPWithError, 1000),
		rtpInModified:   make(chan RTPWithError, 1000),
	}
	mockStream.rtcpWriter = i.BindRTCPWriter(
		interceptor.RTCPWriterFunc(func(pkts []rtcp.Packet, _ interceptor.Attributes) (int, error) {
			select {
			case mockStream.rtcpOutModified <- pkts:
			default:
			}

			return 0, nil
		}),
	)
	mockStream.rtcpReader = i.BindRTCPReader(interceptor.RTCPReaderFunc(
		func(b []byte, attrs interceptor.Attributes) (int, interceptor.Attributes, error) {
			pkts, ok := <-mockStream.rtcpIn
			if !ok {
				return 0, nil, io.EOF
			}

			marshaled, err := rtcp.Marshal(pkts)
			if err != nil {
				return 0, nil, io.EOF
			} else if len(marshaled) > len(b) {
				return 0, nil, io.ErrShortBuffer
			}

			copy(b, marshaled)

			return len(marshaled), attrs, err
		},
	))
	mockStream.rtpWriter = i.BindLocalStream(
		info, interceptor.RTPWriterFunc(
			func(header *rtp.Header, payload []byte, _ interceptor.Attributes) (int, error) {
				select {
				case mockStream.rtpOutModified <- &rtp.Packet{Header: *header, Payload: payload}:
				default:
				}

				return 0, nil
			},
		),
	)
	// Bind rtpReader to the remote stream
	mockStream.rtpReader = interceptor.RTPReaderFunc(
		func(b []byte, attrs interceptor.Attributes) (int, interceptor.Attributes, error) {
			p, ok := <-mockStream.rtpIn
			if !ok {
				return 0, nil, io.EOF
			}

			marshaled, err := p.Marshal()
			if err != nil {
				return 0, nil, io.EOF
			} else if len(marshaled) > len(b) {
				return 0, nil, io.ErrShortBuffer
			}

			copy(b, marshaled)

			return len(marshaled), attrs, err
		},
	)

	// Bind rtpProcessor to process RTP packets and pass them to rtpWriter
	mockStream.rtpProcessor = i.BindRemoteStream(
		info, interceptor.RTPProcessorFunc(
			func(i int, b []byte, attrs interceptor.Attributes) (int, interceptor.Attributes, error) {
				return i, attrs, nil
			},
		),
	)

	go func() {
		buf := make([]byte, 1500)
		for {
			i, _, err := mockStream.rtcpReader.Read(buf, interceptor.Attributes{})
			if err != nil {
				if !errors.Is(err, io.EOF) {
					mockStream.rtcpInModified <- RTCPWithError{Err: err}
				}

				return
			}

			pkts, err := rtcp.Unmarshal(buf[:i])
			if err != nil {
				mockStream.rtcpInModified <- RTCPWithError{Err: err}

				return
			}

			mockStream.rtcpInModified <- RTCPWithError{Packets: pkts}
		}
	}()
	go func() {
		buf := make([]byte, 1500)
		for {
			i, attrs, err := mockStream.rtpReader.Read(buf, interceptor.Attributes{})
			if err != nil {
				if errors.Is(err, io.EOF) {
					mockStream.rtpInModified <- RTPWithError{Err: err}
				}

				return
			}

			// Process the RTP packet through the interceptor pipeline
			_, _, err = mockStream.rtpProcessor.Process(i, buf[:i], attrs)
			if err != nil {
				continue
			}

			p := &rtp.Packet{}
			if err := p.Unmarshal(buf[:i]); err != nil {
				mockStream.rtpInModified <- RTPWithError{Err: err}

				return
			}

			//fmt.Println(p)
			mockStream.rtpInModified <- RTPWithError{Packet: p}
		}
	}()

	return mockStream
}

// WriteRTCP writes a batch of rtcp packet to the stream, using the interceptor.
func (s *MockStream) WriteRTCP(pkts []rtcp.Packet) error {
	_, err := s.rtcpWriter.Write(pkts, interceptor.Attributes{})

	return err
}

// WriteRTP writes an rtp packet to the stream, using the interceptor.
func (s *MockStream) WriteRTP(p *rtp.Packet) error {
	_, err := s.rtpWriter.Write(&p.Header, p.Payload, interceptor.Attributes{})

	return err
}

// ReceiveRTCP schedules a new rtcp batch, so it can be read by the stream.
func (s *MockStream) ReceiveRTCP(pkts []rtcp.Packet) {
	s.rtcpIn <- pkts
}

// ReceiveRTP schedules a rtp packet, so it can be read by the stream.
func (s *MockStream) ReceiveRTP(packet *rtp.Packet) {
	s.rtpIn <- packet
}

// WrittenRTCP returns a channel containing the rtcp batches written, modified by the interceptor.
func (s *MockStream) WrittenRTCP() chan []rtcp.Packet {
	return s.rtcpOutModified
}

// WrittenRTP returns a channel containing rtp packets written, modified by the interceptor.
func (s *MockStream) WrittenRTP() chan *rtp.Packet {
	return s.rtpOutModified
}

// ReadRTCP returns a channel containing the rtcp batched read, modified by the interceptor.
func (s *MockStream) ReadRTCP() chan RTCPWithError {
	return s.rtcpInModified
}

// ReadRTP returns a channel containing the rtp packets read, modified by the interceptor.
func (s *MockStream) ReadRTP() chan RTPWithError {
	return s.rtpInModified
}

// Close closes the stream and the underlying interceptor.
func (s *MockStream) Close() error {
	close(s.rtcpIn)
	close(s.rtpIn)

	return s.interceptor.Close()
}
