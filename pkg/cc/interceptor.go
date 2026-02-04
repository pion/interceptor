// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package cc implements an interceptor for bandwidth estimation that can be
// used with different BandwidthEstimators.
//
// Deprecated: Directly use the bwe implementation in
// https://github.com/pion/bwe instead.
package cc

import (
	"fmt"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/gcc" //nolint
	"github.com/pion/interceptor/pkg/rtpfb"
	"github.com/pion/rtcp"
)

// Option can be used to set initial options on CC interceptors.
//
// Deprecated: See package comment.
type Option func(*Interceptor) error

// BandwidthEstimatorFactory creates new BandwidthEstimators.
//
// Deprecated: See package comment.
type BandwidthEstimatorFactory func() (BandwidthEstimator, error)

// BandwidthEstimator is the interface that will be returned by a
// NewPeerConnectionCallback and can be used to query current bandwidth
// metrics and add feedback manually.
//
// Deprecated: See package comment.
type BandwidthEstimator interface {
	AddStream(*interceptor.StreamInfo, interceptor.RTPWriter) interceptor.RTPWriter
	WriteRTCP([]rtcp.Packet, interceptor.Attributes) error
	GetTargetBitrate() int
	OnTargetBitrateChange(f func(bitrate int))
	GetStats() map[string]any
	Close() error
}

// NewPeerConnectionCallback returns the BandwidthEstimator for the
// PeerConnection with id.
//
// Deprecated: See package comment.
type NewPeerConnectionCallback func(id string, estimator BandwidthEstimator)

// InterceptorFactory is a factory for CC interceptors.
//
// Deprecated: See package comment.
type InterceptorFactory struct {
	opts              []Option
	bweFactory        func() (BandwidthEstimator, error)
	addPeerConnection NewPeerConnectionCallback
	rtpfbFactory      *rtpfb.InterceptorFactory
}

// NewInterceptor returns a new CC interceptor factory.
//
// Deprecated: See package comment.
func NewInterceptor(factory BandwidthEstimatorFactory, opts ...Option) (*InterceptorFactory, error) {
	if factory == nil {
		factory = func() (BandwidthEstimator, error) {
			return gcc.NewSendSideBWE()
		}
	}
	fbi, err := rtpfb.NewInterceptor()
	if err != nil {
		return nil, fmt.Errorf("failed to create rtp feedback interceptor factory: %w", err)
	}

	return &InterceptorFactory{
		opts:              opts,
		bweFactory:        factory,
		addPeerConnection: nil,
		rtpfbFactory:      fbi,
	}, nil
}

// OnNewPeerConnection sets a callback that is called when a new CC interceptor
// is created.
//
// Deprecated: See package comment.
func (f *InterceptorFactory) OnNewPeerConnection(cb NewPeerConnectionCallback) {
	f.addPeerConnection = cb
}

// NewInterceptor returns a new CC interceptor.
//
// Deprecated: See package comment.
func (f *InterceptorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	bwe, err := f.bweFactory()
	if err != nil {
		return nil, err
	}
	interceptorInstance := &Interceptor{
		NoOp:      interceptor.NoOp{},
		estimator: bwe,
		feedback:  make(chan []rtcp.Packet),
		close:     make(chan struct{}),
	}

	for _, opt := range f.opts {
		if err = opt(interceptorInstance); err != nil {
			return nil, err
		}
	}

	if f.addPeerConnection != nil {
		f.addPeerConnection(id, interceptorInstance.estimator)
	}
	fbi, err := f.rtpfbFactory.NewInterceptor(id)
	if err != nil {
		return nil, err
	}

	return interceptor.NewChain([]interceptor.Interceptor{fbi, interceptorInstance}), nil
}

// Interceptor implements Google Congestion Control.
//
// Deprecated: See package comment.
type Interceptor struct {
	interceptor.NoOp
	estimator BandwidthEstimator
	feedback  chan []rtcp.Packet
	close     chan struct{}
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once
// per sender/receiver, however this might change in the future. The returned
// method will be called once per packet batch.
//
// Deprecated: See package comment.
func (c *Interceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}
		buf := make([]byte, i)

		copy(buf, b[:i])

		if attr == nil {
			attr = make(interceptor.Attributes)
		}

		pkts, err := attr.GetRTCPPackets(buf[:i])
		if err != nil {
			return 0, nil, err
		}
		if err = c.estimator.WriteRTCP(pkts, attr); err != nil {
			return 0, nil, err
		}

		return i, attr, nil
	})
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once
// for per LocalStream. The returned method will be called once per rtp packet.
//
// Deprecated: See package comment.
func (c *Interceptor) BindLocalStream(
	info *interceptor.StreamInfo, writer interceptor.RTPWriter,
) interceptor.RTPWriter {
	return c.estimator.AddStream(info, writer)
}

// Close closes the interceptor and the associated bandwidth estimator.
//
// Deprecated: See package comment.
func (c *Interceptor) Close() error {
	return c.estimator.Close()
}
