// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"errors"
	"fmt"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

var (
	errInvalidREDPayload            = errors.New("invalid RED payload")
	errUnexpectedPrimaryPayloadType = errors.New("unexpected RED primary payload type")
	errEmptyPrimaryPayload          = errors.New("RED primary payload is empty")
)

// ReceiverInterceptorFactory creates ReceiverInterceptors.
type ReceiverInterceptorFactory struct{}

// ReceiverInterceptor extracts primary Opus packets from incoming RFC 2198 RED packets.
type ReceiverInterceptor struct {
	interceptor.NoOp
}

// NewReceiverInterceptor returns a new ReceiverInterceptorFactory.
func NewReceiverInterceptor() (*ReceiverInterceptorFactory, error) {
	return &ReceiverInterceptorFactory{}, nil
}

// NewInterceptor constructs a new ReceiverInterceptor.
func (factory *ReceiverInterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	return &ReceiverInterceptor{}, nil
}

// BindRemoteStream unwraps RED packets when negotiated RED information is available.
func (receiver *ReceiverInterceptor) BindRemoteStream(
	info *interceptor.StreamInfo, reader interceptor.RTPReader,
) interceptor.RTPReader {
	if !isOpusREDStream(info) {
		return reader
	}

	ssrc := info.SSRC
	opusPayloadType := info.PayloadType
	redPayloadType := info.PayloadTypeForwardErrorCorrection

	return interceptor.RTPReaderFunc(func(
		buffer []byte, attributes interceptor.Attributes,
	) (int, interceptor.Attributes, error) {
		n, readAttributes, err := reader.Read(buffer, attributes)
		if err != nil {
			return n, readAttributes, err
		}

		header, err := readAttributes.GetRTPHeader(buffer[:n])
		if err != nil {
			return 0, readAttributes, err
		}
		if header.SSRC != ssrc || header.PayloadType != redPayloadType {
			return n, readAttributes, nil
		}

		return extractPrimary(buffer[:n], buffer, readAttributes, opusPayloadType)
	})
}

func extractPrimary(
	raw, output []byte, attributes interceptor.Attributes, opusPayloadType uint8,
) (int, interceptor.Attributes, error) {
	var packet rtp.Packet
	if err := packet.Unmarshal(raw); err != nil {
		return 0, attributes, err
	}

	var redPayload Payload
	if err := redPayload.Unmarshal(packet.Payload); err != nil {
		return 0, attributes, fmt.Errorf("%w: %w", errInvalidREDPayload, err)
	}
	if redPayload.PrimaryBlock.PayloadType != opusPayloadType {
		return 0, attributes, fmt.Errorf(
			"%w: got %d, expected %d",
			errUnexpectedPrimaryPayloadType,
			redPayload.PrimaryBlock.PayloadType,
			opusPayloadType,
		)
	}
	if len(redPayload.PrimaryBlock.Payload) == 0 {
		return 0, attributes, errEmptyPrimaryPayload
	}

	primaryPacket := rtp.Packet{
		Header:  packet.Header.Clone(),
		Payload: append([]byte(nil), redPayload.PrimaryBlock.Payload...),
	}
	primaryPacket.PayloadType = opusPayloadType
	n, err := primaryPacket.MarshalTo(output)
	if err != nil {
		return 0, attributes, err
	}

	outputAttributes := cloneAttributes(attributes)
	outputAttributes.SetRTPHeader(&primaryPacket.Header)

	return n, outputAttributes, nil
}

func cloneAttributes(attributes interceptor.Attributes) interceptor.Attributes {
	if attributes == nil {
		return nil
	}

	cloned := make(interceptor.Attributes, len(attributes))
	for key, value := range attributes {
		cloned.Set(key, value)
	}

	return cloned
}
