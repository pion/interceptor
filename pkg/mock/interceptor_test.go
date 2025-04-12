// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package mock

import (
	"sync/atomic"
	"testing"

	"github.com/pion/interceptor"
)

//nolint:cyclop
func TestInterceptor(t *testing.T) {
	dummyRTPWriter := &RTPWriter{}
	dummyRTPProcessor := &RTPProcessor{}
	dummyRTCPWriter := &RTCPWriter{}
	dummyRTCPReader := &RTCPReader{}
	dummyStreamInfo := &interceptor.StreamInfo{}

	t.Run("Default", func(t *testing.T) {
		testInterceptor := &Interceptor{}

		if testInterceptor.BindRTCPWriter(dummyRTCPWriter) != dummyRTCPWriter {
			t.Error("Default BindRTCPWriter should return given writer")
		}
		if testInterceptor.BindRTCPReader(dummyRTCPReader) != dummyRTCPReader {
			t.Error("Default BindRTCPReader should return given reader")
		}
		if testInterceptor.BindLocalStream(dummyStreamInfo, dummyRTPWriter) != dummyRTPWriter {
			t.Error("Default BindLocalStream should return given writer")
		}
		testInterceptor.UnbindLocalStream(dummyStreamInfo)
		if testInterceptor.BindRemoteStream(dummyStreamInfo, dummyRTPProcessor) != dummyRTPProcessor {
			t.Error("Default BindRemoteStream should return given reader")
		}
		testInterceptor.UnbindRemoteStream(dummyStreamInfo)
		if testInterceptor.Close() != nil {
			t.Error("Default Close should return nil")
		}
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
			BindRemoteStreamFn: func(_ *interceptor.StreamInfo, reader interceptor.RTPProcessor) interceptor.RTPProcessor {
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

		if testInterceptor.BindRTCPWriter(dummyRTCPWriter) != dummyRTCPWriter {
			t.Error("Mocked BindRTCPWriter should return given writer")
		}
		if testInterceptor.BindRTCPReader(dummyRTCPReader) != dummyRTCPReader {
			t.Error("Mocked BindRTCPReader should return given reader")
		}
		if testInterceptor.BindLocalStream(dummyStreamInfo, dummyRTPWriter) != dummyRTPWriter {
			t.Error("Mocked BindLocalStream should return given writer")
		}
		testInterceptor.UnbindLocalStream(dummyStreamInfo)
		if testInterceptor.BindRemoteStream(dummyStreamInfo, dummyRTPProcessor) != dummyRTPProcessor {
			t.Error("Mocked BindRemoteStream should return given reader")
		}
		testInterceptor.UnbindRemoteStream(dummyStreamInfo)
		if testInterceptor.Close() != nil {
			t.Error("Mocked Close should return nil")
		}

		if cnt := atomic.LoadUint32(&cntBindRTCPWriter); cnt != 1 {
			t.Errorf("BindRTCPWriterFn is expected to be called once, but called %d times", cnt)
		}
		if cnt := atomic.LoadUint32(&cntBindRTCPReader); cnt != 1 {
			t.Errorf("BindRTCPReaderFn is expected to be called once, but called %d times", cnt)
		}
		if cnt := atomic.LoadUint32(&cntBindLocalStream); cnt != 1 {
			t.Errorf("BindLocalStreamFn is expected to be called once, but called %d times", cnt)
		}
		if cnt := atomic.LoadUint32(&cntUnbindLocalStream); cnt != 1 {
			t.Errorf("UnbindLocalStreamFn is expected to be called once, but called %d times", cnt)
		}
		if cnt := atomic.LoadUint32(&cntBindRemoteStream); cnt != 1 {
			t.Errorf("BindRemoteStreamFn is expected to be called once, but called %d times", cnt)
		}
		if cnt := atomic.LoadUint32(&cntUnbindRemoteStream); cnt != 1 {
			t.Errorf("UnbindRemoteStreamFn is expected to be called once, but called %d times", cnt)
		}
		if cnt := atomic.LoadUint32(&cntClose); cnt != 1 {
			t.Errorf("CloseFn is expected to be called once, but called %d times", cnt)
		}
	})
}
