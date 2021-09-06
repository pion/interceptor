package nack

import (
	"testing"

	"github.com/pion/rtp/v2"
	"github.com/stretchr/testify/assert"
)

func TestSendBuffer(t *testing.T) {
	for _, start := range []uint16{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 511, 512, 513, 32767, 32768, 32769, 65527, 65528, 65529, 65530, 65531, 65532, 65533, 65534, 65535} {
		start := start

		sb, err := newSendBuffer(8)
		assert.NoError(t, err)

		add := func(nums ...uint16) {
			for _, n := range nums {
				seq := start + n
				sb.add(&rtp.Packet{Header: rtp.Header{SequenceNumber: seq}})
			}
		}

		assertGet := func(nums ...uint16) {
			t.Helper()
			for _, n := range nums {
				seq := start + n
				packet := sb.get(seq)
				if packet == nil {
					t.Errorf("packet not found: %d", seq)
					continue
				}
				if packet.SequenceNumber != seq {
					t.Errorf("packet for %d returned with incorrect SequenceNumber: %d", seq, packet.SequenceNumber)
				}
			}
		}
		assertNOTGet := func(nums ...uint16) {
			t.Helper()
			for _, n := range nums {
				seq := start + n
				packet := sb.get(seq)
				if packet != nil {
					t.Errorf("packet found for %d: %d", seq, packet.SequenceNumber)
				}
			}
		}

		add(0, 1, 2, 3, 4, 5, 6, 7)
		assertGet(0, 1, 2, 3, 4, 5, 6, 7)

		add(8)
		assertGet(8)
		assertNOTGet(0)

		add(10)
		assertGet(10)
		assertNOTGet(1, 2, 9)

		add(22)
		assertGet(22)
		assertNOTGet(3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21)
	}
}

// this test is only useful when being run with the race detector, it won't fail otherwise:
//
//     go test -race ./pkg/nack/
func TestSendBuffer_Race(t *testing.T) {
	for _, start := range []uint16{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 511, 512, 513, 32767, 32768, 32769, 65527, 65528, 65529, 65530, 65531, 65532, 65533, 65534, 65535} {
		start := start

		sb, err := newSendBuffer(8)
		assert.NoError(t, err)

		add := func(nums ...uint16) {
			for _, n := range nums {
				seq := start + n
				sb.add(&rtp.Packet{Header: rtp.Header{SequenceNumber: seq}})
			}
		}

		get := func(nums ...uint16) {
			t.Helper()
			for _, n := range nums {
				seq := start + n
				sb.get(seq)
			}
		}

		go add(0, 1, 2, 3, 4, 5, 6, 7)
		go get(0, 1, 2, 3, 4, 5, 6, 7)
	}
}
