// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"errors"
	"strings"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

const (
	defaultMaxPacketSize     = 1200
	maxSenderRedundantBlocks = 2
	mimeTypeOpus             = "audio/opus"
)

var errInvalidMaxPacketSize = errors.New("maximum RED packet size must be greater than zero")

// SenderInterceptorFactory creates SenderInterceptors.
type SenderInterceptorFactory struct {
	opts []SenderOption
}

// SenderInterceptor adds previous Opus payloads to outgoing packets using RFC 2198 RED.
type SenderInterceptor struct {
	interceptor.NoOp
	maxPacketSize int
}

type senderHistoryEntry struct {
	sequenceNumber uint16
	timestamp      uint32
	payload        []byte
}

type senderStream struct {
	mutex           sync.Mutex
	ssrc            uint32
	opusPayloadType uint8
	redPayloadType  uint8
	maxPacketSize   int
	history         [maxSenderRedundantBlocks]senderHistoryEntry
	historyCount    int
}

// NewSenderInterceptor returns a new SenderInterceptorFactory.
func NewSenderInterceptor(opts ...SenderOption) (*SenderInterceptorFactory, error) {
	return &SenderInterceptorFactory{opts: opts}, nil
}

// NewInterceptor constructs a new SenderInterceptor.
func (factory *SenderInterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	sender := &SenderInterceptor{maxPacketSize: defaultMaxPacketSize}
	for _, opt := range factory.opts {
		if err := opt(sender); err != nil {
			return nil, err
		}
	}

	return sender, nil
}

// BindLocalStream wraps an outgoing Opus stream with RED when negotiated RED information is available.
func (sender *SenderInterceptor) BindLocalStream(
	info *interceptor.StreamInfo, writer interceptor.RTPWriter,
) interceptor.RTPWriter {
	if !isOpusREDStream(info) {
		return writer
	}

	stream := &senderStream{
		ssrc:            info.SSRC,
		opusPayloadType: info.PayloadType,
		redPayloadType:  info.PayloadTypeForwardErrorCorrection,
		maxPacketSize:   sender.maxPacketSize,
	}

	return interceptor.RTPWriterFunc(func(
		header *rtp.Header, payload []byte, attributes interceptor.Attributes,
	) (int, error) {
		if header == nil || header.SSRC != stream.ssrc {
			return writer.Write(header, payload, attributes)
		}

		stream.mutex.Lock()
		outputHeader, outputPayload, err := stream.encode(header, payload)
		stream.mutex.Unlock()
		if err != nil {
			return 0, err
		}

		return writer.Write(outputHeader, outputPayload, attributes)
	})
}

func (stream *senderStream) encode(header *rtp.Header, payload []byte) (*rtp.Header, []byte, error) {
	if header.PayloadType != stream.opusPayloadType {
		stream.reset()

		return header, payload, nil
	}
	if len(payload) == 0 || len(payload) > maxBlockLength {
		stream.reset()

		return header, payload, nil
	}

	stream.resetIfDiscontinuous(header)
	blocksNewestFirst := stream.selectRedundantBlocks(header, payload)

	stream.remember(header, payload)
	if len(blocksNewestFirst) == 0 {
		return header, payload, nil
	}

	blocks := make([]Block, len(blocksNewestFirst))
	for index, block := range blocksNewestFirst {
		blocks[len(blocks)-1-index] = block
	}

	redPayload, err := (Payload{
		RedundantBlocks: blocks,
		PrimaryBlock: Block{
			PayloadType: stream.opusPayloadType,
			Payload:     payload,
		},
	}).Marshal()
	if err != nil {
		return nil, nil, err
	}

	redHeader := header.Clone()
	redHeader.PayloadType = stream.redPayloadType

	return &redHeader, redPayload, nil
}

func (stream *senderStream) resetIfDiscontinuous(header *rtp.Header) {
	if stream.historyCount == 0 {
		return
	}

	newest := stream.history[stream.historyCount-1]
	timestampOffset := header.Timestamp - newest.timestamp
	if header.SequenceNumber != newest.sequenceNumber+1 ||
		timestampOffset == 0 || timestampOffset > maxTimestampOffset {
		stream.reset()
	}
}

func (stream *senderStream) selectRedundantBlocks(header *rtp.Header, payload []byte) []Block {
	blocksNewestFirst := make([]Block, 0, maxSenderRedundantBlocks)
	packetSize := header.MarshalSize() + int(header.PaddingSize) + primaryHeaderSize + len(payload)
	for index := stream.historyCount - 1; index >= 0; index-- {
		entry := stream.history[index]
		timestampOffset := header.Timestamp - entry.timestamp
		if timestampOffset == 0 || timestampOffset > maxTimestampOffset {
			break
		}

		blockSize := redundantHeaderSize + len(entry.payload)
		if packetSize+blockSize > stream.maxPacketSize {
			break
		}

		blocksNewestFirst = append(blocksNewestFirst, Block{
			PayloadType:     stream.opusPayloadType,
			TimestampOffset: uint16(timestampOffset), //nolint:gosec // Offset is bounded above.
			Payload:         entry.payload,
		})
		packetSize += blockSize
	}

	return blocksNewestFirst
}

func (stream *senderStream) remember(header *rtp.Header, payload []byte) {
	entry := senderHistoryEntry{
		sequenceNumber: header.SequenceNumber,
		timestamp:      header.Timestamp,
		payload:        append([]byte(nil), payload...),
	}

	if stream.historyCount < maxSenderRedundantBlocks {
		stream.history[stream.historyCount] = entry
		stream.historyCount++

		return
	}

	copy(stream.history[:], stream.history[1:])
	stream.history[len(stream.history)-1] = entry
}

func (stream *senderStream) reset() {
	for index := range stream.historyCount {
		stream.history[index] = senderHistoryEntry{}
	}
	stream.historyCount = 0
}

func isOpusREDStream(info *interceptor.StreamInfo) bool {
	return info != nil &&
		strings.EqualFold(info.MimeType, mimeTypeOpus) &&
		info.PayloadType <= maxPayloadType &&
		info.PayloadTypeForwardErrorCorrection != 0 &&
		info.PayloadTypeForwardErrorCorrection <= maxPayloadType &&
		info.PayloadType != info.PayloadTypeForwardErrorCorrection
}
