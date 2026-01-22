// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mock

import (
	"sync/atomic"
	"testing"

	"github.com/pion/interceptor"
	"github.com/stretchr/testify/assert"
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

		assert.Equal(
			t, dummyRTCPWriter, testInterceptor.BindRTCPWriter(dummyRTCPWriter),
			"Default BindRTCPWriter should return given writer",
		)
		assert.Equal(
			t, dummyRTCPReader, testInterceptor.BindRTCPReader(dummyRTCPReader),
			"Default BindRTCPReader should return given reader",
		)
		assert.Equal(
			t, dummyRTPWriter, testInterceptor.BindLocalStream(dummyStreamInfo, dummyRTPWriter),
			"Default BindLocalStream should return given writer",
		)
		testInterceptor.UnbindLocalStream(dummyStreamInfo)
		assert.Equal(
			t, dummyRTPReader, testInterceptor.BindRemoteStream(dummyStreamInfo, dummyRTPReader),
			"Default BindRemoteStream should return given writer",
		)
		testInterceptor.UnbindRemoteStream(dummyStreamInfo)
		assert.NoError(t, testInterceptor.Close(), "Default Close should return nil")
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

		assert.Equal(
			t, dummyRTCPWriter, testInterceptor.BindRTCPWriter(dummyRTCPWriter),
			"Mocked BindRTCPWriter should return given writer",
		)
		assert.Equal(
			t, dummyRTCPReader, testInterceptor.BindRTCPReader(dummyRTCPReader),
			"Mocked BindRTCPReader should return given reader",
		)
		assert.Equal(
			t, dummyRTPWriter, testInterceptor.BindLocalStream(dummyStreamInfo, dummyRTPWriter),
			"Mocked BindLocalStream should return given writer",
		)
		testInterceptor.UnbindLocalStream(dummyStreamInfo)
		assert.Equal(
			t, dummyRTPReader, testInterceptor.BindRemoteStream(dummyStreamInfo, dummyRTPReader),
			"Mocked BindRemoteStream should return given writer",
		)
		testInterceptor.UnbindRemoteStream(dummyStreamInfo)
		assert.NoError(t, testInterceptor.Close(), "Mocked Close should return nil")

		assert.Equal(t, uint32(1), atomic.LoadUint32(&cntBindRTCPWriter), "BindRTCPWriterFn is expected to be called once")
		assert.Equal(t, uint32(1), atomic.LoadUint32(&cntBindRTCPReader), "BindRTCPReaderFn is expected to be called once")
		assert.Equal(t, uint32(1), atomic.LoadUint32(&cntBindLocalStream), "BindLocalStreamFn is expected to be called once")
		assert.Equal(
			t, uint32(1), atomic.LoadUint32(&cntUnbindLocalStream), "UnbindLocalStreamFn is expected to be called once",
		)
		assert.Equal(
			t, uint32(1), atomic.LoadUint32(&cntBindRemoteStream), "BindRemoteStreamFn is expected to be called once",
		)
		assert.Equal(
			t, uint32(1), atomic.LoadUint32(&cntUnbindRemoteStream), "UnbindRemoteStreamFn is expected to be called once",
		)
		assert.Equal(t, uint32(1), atomic.LoadUint32(&cntClose), "CloseFn is expected to be called once")
	})
}
