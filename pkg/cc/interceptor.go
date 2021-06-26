// Package cc implements a congestion controller interceptor that can be used
// with different congestion control algorithms.
package cc

import (
	"errors"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/types"
	"github.com/pion/interceptor/pkg/gcc"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

var errInvalidSessionID = errors.New("no bandwidth estimation for session ID")

const transportCCURI = "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"

type (
	headerExtensionKey int
	writerKey          int
)

const (
	twccExtension = iota
	streamWriter
)

type PacerFactory func(interceptor.RTPWriter) Pacer

type Pacer interface {
	interceptor.RTPWriter
	AddStream(ssrc uint32, writer interceptor.RTPWriter)
	SetTargetBitrate(types.DataRate)
	Close() error
}

type BandwidthEstimatorFactory func() BandwidthEstimator

// BandwidthEstimator is the interface of a bandwidth estimator
type BandwidthEstimator interface {
	OnPacketSent(ts time.Time, sizeInBytes int)
	OnFeedback([]types.PacketResult)
	GetBandwidthEstimation() types.DataRate
}

type session struct {
	i *ControllerInterceptor
}

type Option func(*ControllerInterceptor) error

func SetBWE(bwe BandwidthEstimatorFactory) Option {
	return func(ci *ControllerInterceptor) error {
		ci.BandwidthEstimator = bwe()
		return nil
	}
}

type ControllerInterceptorFactory struct {
	opts []Option
}

func GCCFactory() BandwidthEstimator {
	return gcc.NewSendSideBandwidthEstimator(150 * types.KiloBitPerSecond)
}

func NewControllerInterceptor(opts ...Option) (cif *ControllerInterceptorFactory, err error) {
	return &ControllerInterceptorFactory{
		opts: opts,
	}, nil
}

// NewInterceptor creates a new ControllerInterceptor
func (f *ControllerInterceptorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	i := &ControllerInterceptor{
		NoOp:                interceptor.NoOp{},
		log:                 logging.NewDefaultLoggerFactory().NewLogger("cc_interceptor"),
		FeedbackAdapter:     *NewFeedbackAdapter(),
		BandwidthEstimator:  GCCFactory(),
		pacer:               NewLeakyBucketPacer(),
		twccFeedbackChan:    make(chan twccFeedback),
		rfc8888FeedbackChan: make(chan rfc8888Feedback),
		incomingPacketChan:  make(chan packetWithAttributes),
		wg:                  sync.WaitGroup{},
		close:               make(chan struct{}),
	}

	for _, opt := range f.opts {
		if err := opt(i); err != nil {
			return nil, err
		}
	}

	go i.loop()

	return i, nil
}

type twccFeedback struct {
	ts time.Time
	*rtcp.TransportLayerCC
}

type rfc8888Feedback struct {
	ts              time.Time
	*rtcp.RawPacket // TODO(mathis) change to RFC8888 packet
}

type packetWithAttributes struct {
	header     rtp.Header
	payload    []byte
	attributes interceptor.Attributes
}

// ControllerInterceptor is an interceptor for congestion control/bandwidth
// estimation
type ControllerInterceptor struct {
	interceptor.NoOp

	log logging.LeveledLogger

	FeedbackAdapter
	BandwidthEstimator

	pacer Pacer

	twccFeedbackChan    chan twccFeedback
	rfc8888FeedbackChan chan rfc8888Feedback
	incomingPacketChan  chan packetWithAttributes

	wg    sync.WaitGroup
	close chan struct{}
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once per sender/receiver, however this might
// change in the future. The returned method will be called once per packet batch.
func (c *ControllerInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(buf []byte, attributes interceptor.Attributes) (int, interceptor.Attributes, error) {
		// TODO(mathis): Put receive timestamp in attributes and populate in
		// first interceptor
		ts := time.Now()

		i, attr, err := reader.Read(buf, attributes)
		if err != nil {
			return 0, nil, err
		}
		if attr == nil {
			attr = make(interceptor.Attributes)
		}

		pkts, err := attr.GetRTCPPackets(buf[:i])
		if err != nil {
			return 0, nil, err
		}
		for _, pkt := range pkts {
			switch feedback := pkt.(type) {
			case *rtcp.TransportLayerCC:
				c.twccFeedbackChan <- twccFeedback{ts, feedback}
			case *rtcp.RawPacket:
				c.rfc8888FeedbackChan <- rfc8888Feedback{ts, feedback}
			}
		}

		return i, attr, nil
	})
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once for per LocalStream. The returned method
// will be called once per rtp packet.
func (c *ControllerInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	// TODO(mathis): figure out if we have to start more loops here or create
	// dedicated controllers/pacer for each stream here.

	var hdrExtID uint8
	for _, e := range info.RTPHeaderExtensions {
		if e.URI == transportCCURI {
			hdrExtID = uint8(e.ID)
			break
		}
	}
	if hdrExtID == 0 { // Don't try to read header extension if ID is 0, because 0 is an invalid extension ID
		return writer
	}

	c.pacer.AddStream(info.SSRC, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {

		c.OnSent(time.Now(), header, attributes)

		return writer.Write(header, payload, attributes)
	}))

	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		if attributes == nil {
			attributes = make(interceptor.Attributes)
		}
		attributes.Set(twccExtension, hdrExtID)
		c.incomingPacketChan <- packetWithAttributes{
			header:     *header,
			payload:    payload,
			attributes: attributes,
		}

		return header.MarshalSize() + len(payload), nil
	})
}

// TODO(mathis): start loop, figure out when and how often. Probably only once
// and then add streams.
// TODO(mathis): Update bandwidth sometimes...
func (c *ControllerInterceptor) loop() {
	for {
		select {
		case <-c.close:
			return

		case pkt := <-c.incomingPacketChan:
			c.pacer.Write(&pkt.header, pkt.payload, pkt.attributes)

		case feedback := <-c.twccFeedbackChan:
			packetResult, err := c.OnIncomingTransportCC(feedback.TransportLayerCC)
			if err != nil {
				// TODO(mathis): handle error
			}
			c.OnFeedback(packetResult)
			c.pacer.SetTargetBitrate(c.GetBandwidthEstimation())

		case feedback := <-c.rfc8888FeedbackChan:
			packetResult, err := c.OnIncomingRFC8888(feedback.RawPacket)
			if err != nil {
				// TODO(mathis): handle error
			}
			c.OnFeedback(packetResult)
			c.pacer.SetTargetBitrate(c.GetBandwidthEstimation())
		}
	}
}

// Close closes the interceptor.
func (c *ControllerInterceptor) Close() error {
	defer c.wg.Wait()

	if !c.isClosed() {
		close(c.close)
	}

	return nil
}

func (c *ControllerInterceptor) isClosed() bool {
	select {
	case <-c.close:
		return true
	default:
		return false
	}
}
