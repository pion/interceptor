// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"errors"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtp"
)

// InterceptorFactory is a interceptor.Factory for a GeneratorInterceptor.
type InterceptorFactory struct {
	opts []ReceiverInterceptorOption
}

// NewInterceptor constructs a new ReceiverInterceptor with jitter buffer.
func (g *InterceptorFactory) NewInterceptor(logName string) (interceptor.Interceptor, error) {
	if logName == "" {
		logName = "jitterbuffer"
	}
	i := &ReceiverInterceptor{
		close:  make(chan struct{}),
		log:    logging.NewDefaultLoggerFactory().NewLogger(logName),
		buffer: New(),
	}

	for _, opt := range g.opts {
		if err := opt(i); err != nil {
			return nil, err
		}
	}

	return i, nil
}

// ReceiverInterceptor places a JitterBuffer in the chain to smooth packet arrival
// and allow for network jitter
//
//	The Interceptor is designed to fit in a RemoteStream
//	pipeline and buffer incoming packets for a short period (currently
//	defaulting to 50 packets) before emitting packets to be consumed by the
//	next step in the pipeline.
//
//	The caller must ensure they are prepared to handle an
//	ErrPopWhileBuffering in the case that insufficient packets have been
//	received by the jitter buffer. The caller should retry the operation
//	at some point later as the buffer may have been filled in the interim.
//
//	The caller should also be aware that an ErrBufferUnderrun may be
//	returned in the case that the initial buffering was sufficient and
//	playback began but the caller is consuming packets (or they are not
//	arriving) quickly enough.
type ReceiverInterceptor struct {
	interceptor.NoOp
	buffer             *JitterBuffer
	wg                 sync.WaitGroup
	close              chan struct{}
	log                logging.LeveledLogger
	skipMissingPackets bool
}

// NewInterceptor returns a new InterceptorFactory.
func NewInterceptor(opts ...ReceiverInterceptorOption) (*InterceptorFactory, error) {
	return &InterceptorFactory{opts}, nil
}

// BindRemoteStream lets you modify any incoming RTP packets. It is called once for per RemoteStream.
// The returned method will be called once per rtp packet.
func (i *ReceiverInterceptor) BindRemoteStream(
	_ *interceptor.StreamInfo, reader interceptor.RTPReader,
) interceptor.RTPReader {
	return interceptor.RTPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		buf := make([]byte, len(b))
		n, attr, err := reader.Read(buf, a)
		if err != nil {
			return n, attr, err
		}
		packet := &rtp.Packet{}
		if err := packet.Unmarshal(buf[:n]); err != nil {
			return 0, nil, err
		}
		i.buffer.Push(packet)
		if i.buffer.State() == Emitting {
			return i.playout(b, n, attr)
		}

		return n, attr, ErrPopWhileBuffering
	})
}

func (i *ReceiverInterceptor) playout(
	b []byte,
	n int,
	attr interceptor.Attributes,
) (int, interceptor.Attributes, error) {
	for {
		newPkt, err := i.buffer.Pop()
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				if i.skipMissingPackets {
					i.log.Warn("Skipping missing packet")
					i.buffer.SetPlayoutHead(i.buffer.PlayoutHead() + 1)

					continue
				}
			}

			return 0, nil, err
		}
		if newPkt != nil {
			nlen, err := newPkt.MarshalTo(b)

			return nlen, attr, err
		}
		if i.buffer.Length() == 0 {
			break
		}
	}

	return n, attr, ErrPopWhileBuffering
}

// UnbindRemoteStream is called when the Stream is removed. It can be used to clean up any data related to that track.
func (i *ReceiverInterceptor) UnbindRemoteStream(_ *interceptor.StreamInfo) {
	defer i.wg.Wait()
	i.buffer.Clear(true)
}

// Close closes the interceptor.
func (i *ReceiverInterceptor) Close() error {
	defer i.wg.Wait()
	i.buffer.Clear(true)

	return nil
}
