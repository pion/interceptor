// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package cc implements an interceptor for bandwidth estimation that can be
// used with different BandwidthEstimators.
package cc

import (
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/gcc"
	"github.com/pion/rtcp"
)

// Option can be used to set initial options on CC interceptors
type Option func(*Interceptor) error

// BandwidthEstimatorFactory creates new BandwidthEstimators
type BandwidthEstimatorFactory func() (BandwidthEstimator, error)

// BandwidthEstimator is the interface that will be returned by a
// NewPeerConnectionCallback and can be used to query current bandwidth
// metrics and add feedback manually.
type BandwidthEstimator interface {
	AddStream(*interceptor.StreamInfo, interceptor.RTPWriter) interceptor.RTPWriter
	WriteRTCP([]rtcp.Packet, interceptor.Attributes) error
	GetTargetBitrate() int
	OnTargetBitrateChange(f func(bitrate int))
	GetStats() map[string]interface{}
	Close() error
}

// NewPeerConnectionCallback returns the BandwidthEstimator for the
// PeerConnection with id
type NewPeerConnectionCallback func(id string, estimator BandwidthEstimator)

// InterceptorFactory is a factory for CC interceptors
type InterceptorFactory struct {
	opts              []Option
	bweFactory        func() (BandwidthEstimator, error)
	addPeerConnection NewPeerConnectionCallback
}

// NewInterceptor returns a new CC interceptor factory
func NewInterceptor(factory BandwidthEstimatorFactory, opts ...Option) (*InterceptorFactory, error) {
	if factory == nil {
		factory = func() (BandwidthEstimator, error) {
			return gcc.NewSendSideBWE()
		}
	}
	return &InterceptorFactory{
		opts:              opts,
		bweFactory:        factory,
		addPeerConnection: nil,
	}, nil
}

// OnNewPeerConnection sets a callback that is called when a new CC interceptor
// is created.
func (f *InterceptorFactory) OnNewPeerConnection(cb NewPeerConnectionCallback) {
	f.addPeerConnection = cb
}

// NewInterceptor returns a new CC interceptor
func (f *InterceptorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	bwe, err := f.bweFactory()
	if err != nil {
		return nil, err
	}
	i := &Interceptor{
		NoOp:      interceptor.NoOp{},
		estimator: bwe,
		feedback:  make(chan []rtcp.Packet),
		close:     make(chan struct{}),
	}

	for _, opt := range f.opts {
		if err := opt(i); err != nil {
			return nil, err
		}
	}

	if f.addPeerConnection != nil {
		f.addPeerConnection(id, i.estimator)
	}
	return i, nil
}

// Interceptor implements Google Congestion Control
type Interceptor struct {
	interceptor.NoOp
	estimator BandwidthEstimator
	feedback  chan []rtcp.Packet
	close     chan struct{}
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once
// per sender/receiver, however this might change in the future. The returned
// method will be called once per packet batch.
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
func (c *Interceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	return c.estimator.AddStream(info, writer)
}

// Close closes the interceptor and the associated bandwidth estimator.
func (c *Interceptor) Close() error {
	return c.estimator.Close()
}
