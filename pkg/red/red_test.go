// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPayloadMarshal(t *testing.T) {
	t.Run("PrimaryOnly", func(t *testing.T) {
		payload := Payload{
			PrimaryBlock: Block{PayloadType: 111, Payload: []byte{0x01, 0x02, 0x03}},
		}

		raw, err := payload.Marshal()
		require.NoError(t, err)
		assert.Equal(t, []byte{0x6f, 0x01, 0x02, 0x03}, raw)
		assert.Equal(t, len(raw), payload.MarshalSize())
	})

	t.Run("RedundantBlock", func(t *testing.T) {
		payload := Payload{
			RedundantBlocks: []Block{{
				PayloadType:     7,
				TimestampOffset: 160,
				Payload:         []byte{0x11, 0x22},
			}},
			PrimaryBlock: Block{PayloadType: 5, Payload: []byte{0x33, 0x44, 0x55}},
		}

		raw, err := payload.Marshal()
		require.NoError(t, err)
		assert.Equal(t, []byte{
			0x87, 0x02, 0x80, 0x02,
			0x05,
			0x11, 0x22,
			0x33, 0x44, 0x55,
		}, raw)
		assert.Equal(t, len(raw), payload.MarshalSize())
	})

	t.Run("MaximumFieldValues", func(t *testing.T) {
		payload := Payload{
			RedundantBlocks: []Block{{
				PayloadType:     maxPayloadType,
				TimestampOffset: maxTimestampOffset,
				Payload:         make([]byte, maxBlockLength),
			}},
			PrimaryBlock: Block{PayloadType: maxPayloadType},
		}

		raw, err := payload.Marshal()
		require.NoError(t, err)
		assert.Equal(t, []byte{0xff, 0xff, 0xff, 0xff, 0x7f}, raw[:5])
	})
}

func TestPayloadMarshalToShortBuffer(t *testing.T) {
	payload := Payload{PrimaryBlock: Block{PayloadType: 111, Payload: []byte{0x01}}}

	n, err := payload.MarshalTo(make([]byte, payload.MarshalSize()-1))
	assert.Zero(t, n)
	assert.ErrorIs(t, err, io.ErrShortBuffer)
}

func TestPayloadMarshalValidation(t *testing.T) {
	tests := []struct {
		name    string
		payload Payload
		err     error
	}{
		{
			name:    "PrimaryPayloadType",
			payload: Payload{PrimaryBlock: Block{PayloadType: maxPayloadType + 1}},
			err:     errPayloadTypeOutOfRange,
		},
		{
			name: "RedundantPayloadType",
			payload: Payload{
				RedundantBlocks: []Block{{PayloadType: maxPayloadType + 1}},
			},
			err: errPayloadTypeOutOfRange,
		},
		{
			name: "TimestampOffset",
			payload: Payload{
				RedundantBlocks: []Block{{TimestampOffset: maxTimestampOffset + 1}},
			},
			err: errTimestampOffsetOutOfRange,
		},
		{
			name: "BlockLength",
			payload: Payload{
				RedundantBlocks: []Block{{Payload: make([]byte, maxBlockLength+1)}},
			},
			err: errBlockLengthOutOfRange,
		},
		{
			name: "TooManyBlocks",
			payload: Payload{
				RedundantBlocks: make([]Block, maxRedundantBlocks+1),
			},
			err: errTooManyBlocks,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.payload.Marshal()
			assert.ErrorIs(t, err, test.err)
		})
	}
}

func TestPayloadUnmarshal(t *testing.T) {
	raw := []byte{
		0xef, 0x1e, 0x00, 0x03,
		0xee, 0x0f, 0x00, 0x02,
		0x6f,
		0x01, 0x02, 0x03,
		0x04, 0x05,
		0x06, 0x07, 0x08,
	}

	var payload Payload
	require.NoError(t, payload.Unmarshal(raw))
	assert.Equal(t, []Block{
		{PayloadType: 111, TimestampOffset: 1920, Payload: []byte{0x01, 0x02, 0x03}},
		{PayloadType: 110, TimestampOffset: 960, Payload: []byte{0x04, 0x05}},
	}, payload.RedundantBlocks)
	assert.Equal(t, Block{PayloadType: 111, Payload: []byte{0x06, 0x07, 0x08}}, payload.PrimaryBlock)

	marshaled, err := payload.Marshal()
	require.NoError(t, err)
	assert.Equal(t, raw, marshaled)

	raw[9] = 0xff
	assert.Equal(t, byte(0xff), payload.RedundantBlocks[0].Payload[0], "parsed payloads should reference the input buffer")
}

func TestPayloadUnmarshalMalformed(t *testing.T) {
	tooManyBlocks := make([]byte, (maxRedundantBlocks+1)*redundantHeaderSize+primaryHeaderSize)
	for i := range maxRedundantBlocks + 1 {
		tooManyBlocks[i*redundantHeaderSize] = followBit
	}

	tests := []struct {
		name string
		raw  []byte
		err  error
	}{
		{name: "Empty", raw: nil, err: errMalformedPayload},
		{name: "TruncatedHeaderOneByte", raw: []byte{followBit}, err: errMalformedPayload},
		{name: "TruncatedHeaderTwoBytes", raw: []byte{followBit, 0}, err: errMalformedPayload},
		{name: "TruncatedHeaderThreeBytes", raw: []byte{followBit, 0, 0}, err: errMalformedPayload},
		{name: "TruncatedBlock", raw: []byte{followBit, 0, 0, 2, 0, 1}, err: errMalformedPayload},
		{name: "TooManyBlocks", raw: tooManyBlocks, err: errTooManyBlocks},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := Payload{
				RedundantBlocks: []Block{{Payload: []byte{0x01}}},
				PrimaryBlock:    Block{Payload: []byte{0x02}},
			}

			err := payload.Unmarshal(test.raw)
			assert.ErrorIs(t, err, test.err)
			assert.Empty(t, payload.RedundantBlocks)
			assert.Equal(t, Block{}, payload.PrimaryBlock)
		})
	}
}

func FuzzPayloadUnmarshal(f *testing.F) {
	f.Add([]byte{0x6f, 0x01, 0x02, 0x03})
	f.Add([]byte{0x87, 0x02, 0x80, 0x02, 0x05, 0x11, 0x22, 0x33})
	f.Add([]byte{followBit})

	f.Fuzz(func(t *testing.T, raw []byte) {
		var payload Payload
		if err := payload.Unmarshal(raw); err != nil {
			return
		}

		marshaled, err := payload.Marshal()
		require.NoError(t, err)
		assert.Equal(t, raw, marshaled)
	})
}
