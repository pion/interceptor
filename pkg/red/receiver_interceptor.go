// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"errors"

	"github.com/pion/interceptor"
)

var (
	errInvalidREDPayload             = errors.New("invalid RED payload")
	errUnexpectedPrimaryPayloadType  = errors.New("unexpected RED primary payload type")
	errEmptyPrimaryPayload           = errors.New("RED primary payload is empty")
	errInvalidReceiverPacketDuration = errors.New("RED receiver packet duration must be a multiple of 2.5ms between 2.5ms and 120ms")
)

// ReceiverInterceptorFactory creates ReceiverInterceptors.
type ReceiverInterceptorFactory struct {
	opts []ReceiverOption
}

// ReceiverInterceptor extracts primary Opus packets and recovers redundant audio
// when its timestamp can be mapped to an original RTP sequence number.
// Redundant Opus blocks must copy previously transmitted primary Opus packets.
// Those packets must have distinct timestamps increasing in RTP sequence order.
type ReceiverInterceptor struct {
	interceptor.NoOp
	timestampStep uint32
}

// NewReceiverInterceptor returns a new ReceiverInterceptorFactory.
// By default, recovery requires redundant copies to cover every missing sequence
// number between known Opus packets. Unanchored or partially covered gaps are
// skipped because RED does not carry the original sequence numbers.
// ReceiverPacketDuration enables broader recovery under an explicit cadence contract.
func NewReceiverInterceptor(opts ...ReceiverOption) (*ReceiverInterceptorFactory, error) {
	return &ReceiverInterceptorFactory{opts: opts}, nil
}

// NewInterceptor constructs a new ReceiverInterceptor.
func (factory *ReceiverInterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	receiver := &ReceiverInterceptor{}
	for _, opt := range factory.opts {
		if err := opt(receiver); err != nil {
			return nil, err
		}
	}

	return receiver, nil
}

// BindRemoteStream unwraps RED packets when negotiated RED information is available.
func (receiver *ReceiverInterceptor) BindRemoteStream(
	info *interceptor.StreamInfo, reader interceptor.RTPReader,
) interceptor.RTPReader {
	if !isOpusREDStream(info) {
		return reader
	}

	return &receiverStream{
		reader:          reader,
		ssrc:            info.SSRC,
		opusPayloadType: info.PayloadType,
		redPayloadType:  info.PayloadTypeForwardErrorCorrection,
		timestampStep:   receiver.timestampStep,
	}
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
