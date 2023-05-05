// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package intervalpli

import (
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
)

// ReceiverInterceptorFactory is a interceptor.Factory for a ReceiverInterceptor
type ReceiverInterceptorFactory struct {
	opts []GeneratorOption
}

// NewReceiverInterceptor returns a new ReceiverInterceptor
func NewReceiverInterceptor(opts ...GeneratorOption) (*ReceiverInterceptorFactory, error) {
	return &ReceiverInterceptorFactory{
		opts: opts,
	}, nil
}

// NewInterceptor returns a new ReceiverInterceptor interceptor.
func (r *ReceiverInterceptorFactory) NewInterceptor(string) (interceptor.Interceptor, error) {
	return NewGeneratorInterceptor(r.opts...)
}

// GeneratorInterceptor interceptor sends PLI packets.
// Implements PLI in a naive way: sends a PLI for each new track that support PLI, periodically.
type GeneratorInterceptor struct {
	interceptor.NoOp

	interval           time.Duration
	streams            sync.Map
	immediatePLINeeded chan []uint32

	log logging.LeveledLogger
	m   sync.Mutex
	wg  sync.WaitGroup

	close chan struct{}
}

// NewGeneratorInterceptor returns a new GeneratorInterceptor interceptor.
func NewGeneratorInterceptor(opts ...GeneratorOption) (*GeneratorInterceptor, error) {
	r := &GeneratorInterceptor{
		interval:           3 * time.Second,
		log:                logging.NewDefaultLoggerFactory().NewLogger("pli_generator"),
		immediatePLINeeded: make(chan []uint32, 1),
		close:              make(chan struct{}),
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	return r, nil
}

func (r *GeneratorInterceptor) isClosed() bool {
	select {
	case <-r.close:
		return true
	default:
		return false
	}
}

// Close closes the interceptor.
func (r *GeneratorInterceptor) Close() error {
	defer r.wg.Wait()
	r.m.Lock()
	defer r.m.Unlock()

	if !r.isClosed() {
		close(r.close)
	}

	return nil
}

// BindRTCPWriter lets you modify any outgoing RTCP packets. It is called once per PeerConnection. The returned method
// will be called once per packet batch.
func (r *GeneratorInterceptor) BindRTCPWriter(writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	r.m.Lock()
	defer r.m.Unlock()

	if r.isClosed() {
		return writer
	}

	r.wg.Add(1)

	go r.loop(writer)

	return writer
}

func (r *GeneratorInterceptor) loop(rtcpWriter interceptor.RTCPWriter) {
	defer r.wg.Done()

	ticker, tickerChan := r.createLoopTicker()

	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()

	for {
		select {
		case ssrcs := <-r.immediatePLINeeded:
			r.writePLIs(rtcpWriter, ssrcs)

		case <-tickerChan:
			ssrcs := make([]uint32, 0)

			r.streams.Range(func(k, value interface{}) bool {
				key, ok := k.(uint32)
				if !ok {
					return false
				}

				ssrcs = append(ssrcs, key)
				return true
			})

			r.writePLIs(rtcpWriter, ssrcs)

		case <-r.close:
			return
		}
	}
}

func (r *GeneratorInterceptor) createLoopTicker() (*time.Ticker, <-chan time.Time) {
	if r.interval > 0 {
		ticker := time.NewTicker(r.interval)
		return ticker, ticker.C
	}

	return nil, make(chan time.Time)
}

func (r *GeneratorInterceptor) writePLIs(rtcpWriter interceptor.RTCPWriter, ssrcs []uint32) {
	if len(ssrcs) == 0 {
		return
	}

	pkts := []rtcp.Packet{}

	for _, ssrc := range ssrcs {
		pkts = append(pkts, &rtcp.PictureLossIndication{MediaSSRC: ssrc})
	}

	if _, err := rtcpWriter.Write(pkts, interceptor.Attributes{}); err != nil {
		r.log.Warnf("failed sending: %+v", err)
	}
}

// BindRemoteStream lets you modify any incoming RTP packets. It is called once for per RemoteStream. The returned method
// will be called once per rtp packet.
func (r *GeneratorInterceptor) BindRemoteStream(info *interceptor.StreamInfo, reader interceptor.RTPReader) interceptor.RTPReader {
	if !streamSupportPli(info) {
		return reader
	}

	r.streams.Store(info.SSRC, nil)
	// New streams need to receive a PLI as soon as possible.
	r.ForcePLI(info.SSRC)

	return reader
}

// UnbindLocalStream is called when the Stream is removed. It can be used to clean up any data related to that track.
func (r *GeneratorInterceptor) UnbindLocalStream(info *interceptor.StreamInfo) {
	r.streams.Delete(info.SSRC)
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once per sender/receiver, however this might
// change in the future. The returned method will be called once per packet batch.
func (r *GeneratorInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return reader
}

// ForcePLI sends a PLI request to the tracks matching the provided SSRCs.
func (r *GeneratorInterceptor) ForcePLI(ssrc ...uint32) {
	r.immediatePLINeeded <- ssrc
}
