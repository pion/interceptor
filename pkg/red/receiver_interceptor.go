// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"errors"

	"github.com/pion/interceptor"
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

	return &receiverStream{
		reader:          reader,
		ssrc:            info.SSRC,
		opusPayloadType: info.PayloadType,
		redPayloadType:  info.PayloadTypeForwardErrorCorrection,
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
