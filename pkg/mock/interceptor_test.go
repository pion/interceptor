// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mock

import (
	"sync/atomic"
	"testing"

	"github.com/pion/interceptor"
	"github.com/stretchr/testify/require"
)

//nolint:cyclop
func TestInterceptor(t *testing.T) {
	dummyRTPWriter := &RTPWriter{}
	dummyRTPReader := &RTPReader{}
	dummyRTCPWriter := &RTCPWriter{}
	dummyRTCPReader := &RTCPReader{}
	dummyStreamInfo := &interceptor.StreamInfo{}

	t.Run("Default", func(t *testing.T) {
		testInterceptor := &Interceptor{}

		require.Equal(t, testInterceptor.BindRTCPWriter(dummyRTCPWriter), dummyRTCPWriter)
		require.Equal(t, testInterceptor.BindRTCPReader(dummyRTCPReader), dummyRTCPReader)
		require.Equal(t, testInterceptor.BindLocalStream(dummyStreamInfo, dummyRTPWriter), dummyRTPWriter)

		testInterceptor.UnbindLocalStream(dummyStreamInfo)
		require.Equal(t, testInterceptor.BindRemoteStream(dummyStreamInfo, dummyRTPReader), dummyRTPReader)

		testInterceptor.UnbindRemoteStream(dummyStreamInfo)
		require.NoError(t, testInterceptor.Close())
	})
	t.Run("Custom", func(t *testing.T) {
		var (
			cntBindRTCPReader     uint32
			cntBindRTCPWriter     uint32
			cntBindLocalStream    uint32
			cntUnbindLocalStream  uint32
			cntBindRemoteStream   uint32
			cntUnbindRemoteStream uint32
			cntClose              uint32
		)
		testInterceptor := &Interceptor{
			BindRTCPReaderFn: func(reader interceptor.RTCPReader) interceptor.RTCPReader {
				atomic.AddUint32(&cntBindRTCPReader, 1)

				return reader
			},
			BindRTCPWriterFn: func(writer interceptor.RTCPWriter) interceptor.RTCPWriter {
				atomic.AddUint32(&cntBindRTCPWriter, 1)

				return writer
			},
			BindLocalStreamFn: func(_ *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
				atomic.AddUint32(&cntBindLocalStream, 1)

				return writer
			},
			UnbindLocalStreamFn: func(*interceptor.StreamInfo) {
				atomic.AddUint32(&cntUnbindLocalStream, 1)
			},
			BindRemoteStreamFn: func(_ *interceptor.StreamInfo, reader interceptor.RTPReader) interceptor.RTPReader {
				atomic.AddUint32(&cntBindRemoteStream, 1)

				return reader
			},
			UnbindRemoteStreamFn: func(*interceptor.StreamInfo) {
				atomic.AddUint32(&cntUnbindRemoteStream, 1)
			},
			CloseFn: func() error {
				atomic.AddUint32(&cntClose, 1)

				return nil
			},
		}

		require.Equal(t, testInterceptor.BindRTCPWriter(dummyRTCPWriter), dummyRTCPWriter)
		require.Equal(t, testInterceptor.BindRTCPReader(dummyRTCPReader), dummyRTCPReader)
		testInterceptor.BindLocalStream(dummyStreamInfo, dummyRTPWriter)
		testInterceptor.BindRemoteStream(dummyStreamInfo, dummyRTPReader)
		testInterceptor.UnbindRemoteStream(dummyStreamInfo)
		testInterceptor.UnbindLocalStream(dummyStreamInfo)
		require.NoError(t, testInterceptor.Close())

		require.Equal(t, atomic.LoadUint32(&cntBindRTCPWriter), uint32(1))
		require.Equal(t, atomic.LoadUint32(&cntBindRTCPReader), uint32(1))
		require.Equal(t, atomic.LoadUint32(&cntBindLocalStream), uint32(1))
		require.Equal(t, atomic.LoadUint32(&cntUnbindLocalStream), uint32(1))
		require.Equal(t, atomic.LoadUint32(&cntBindRemoteStream), uint32(1))
		require.Equal(t, atomic.LoadUint32(&cntUnbindRemoteStream), uint32(1))
		require.Equal(t, atomic.LoadUint32(&cntClose), uint32(1))
	})
}
