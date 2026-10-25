// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestLeakyBucketPacer_PayloadSize(t *testing.T) {
	for _, size := range []int{1460, 1461, 4096, 65535} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			pacer := NewLeakyBucketPacer(0)
			t.Cleanup(func() { require.NoError(t, pacer.Close()) })
			received := make(chan []byte, 1)
			pacer.AddStream(1, interceptor.RTPWriterFunc(func(_ *rtp.Header, payload []byte, _ interceptor.Attributes) (int, error) {
				received <- bytes.Clone(payload)

				return len(payload), nil
			}))
			header := &rtp.Header{SSRC: 1}
			payload := bytes.Repeat([]byte{0xab}, size)
			expected := bytes.Clone(payload)
			n, err := pacer.Write(header, payload, nil)
			require.NoError(t, err)
			require.Equal(t, header.MarshalSize()+size, n)
			clear(payload)
			pacer.SetTargetBitrate(10_000_000)
			select {
			case actual := <-received:
				require.Equal(t, expected, actual)
			case <-time.After(time.Second):
				require.FailNow(t, "pacer did not send the payload")
			}
		})
	}
}
