package gcc

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

const twccExtensionAttributesKey = iota

const transportCCURI = "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"

var ErrUnknownSession = errors.New("unknown session ID")

type Option func(*GCCInterceptor) error

func InitialBitrate(rate int) Option {
	return func(g *GCCInterceptor) error {
		g.bitrate = rate
		return nil
	}
}

type GCCInterceptorFactory struct {
	opts         []Option
	interceptors map[string]*GCCInterceptor
}

func NewGCCInterceptor(opts ...Option) (*GCCInterceptorFactory, error) {
	return &GCCInterceptorFactory{
		opts:         opts,
		interceptors: map[string]*GCCInterceptor{},
	}, nil
}

func (f *GCCInterceptorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	if i, ok := f.interceptors[id]; ok {
		return i, nil
	}
	i := &GCCInterceptor{
		NoOp:            interceptor.NoOp{},
		lock:            sync.Mutex{},
		bitrate:         0,
		pacer:           nil,
		FeedbackAdapter: nil,
		loss:            nil,
		delay:           nil,
		packet:          make(chan *packetAndAttributes),
		feedback:        make(chan []rtcp.Packet),
		close:           make(chan struct{}),
	}

	for _, opt := range f.opts {
		if err := opt(i); err != nil {
			return nil, err
		}
	}

	i.pacer = NewLeakyBucketPacer()
	i.FeedbackAdapter = NewFeedbackAdapter()
	i.loss = newLossBasedBWE(i.bitrate)
	i.delay = newDelayBasedBWE(i.bitrate)

	f.interceptors[id] = i
	go i.loop()
	return i, nil
}

func (f *GCCInterceptorFactory) GetTargetBitrate(id string) (int, error) {
	if i, ok := f.interceptors[id]; ok {
		return i.getTargetBitrate(), nil
	}
	return 0, fmt.Errorf("%w: %v", ErrUnknownSession, id)
}

type GCCStats struct {
	LossBasedEstimate int
	DelayStats
}

func (f *GCCInterceptorFactory) GetStats(id string) (*GCCStats, error) {
	if i, ok := f.interceptors[id]; ok {
		return i.getStats(), nil
	}
	return nil, fmt.Errorf("%w: %v", ErrUnknownSession, id)
}

type Pacer interface {
	interceptor.RTPWriter
	AddStream(ssrc uint32, writer interceptor.RTPWriter)
	SetTargetBitrate(int)
	Close() error
}

type packetAndAttributes struct {
	header     rtp.Header
	payload    []byte
	attributes interceptor.Attributes
}

type GCCInterceptor struct {
	interceptor.NoOp

	lock    sync.Mutex
	bitrate int

	latestStats *GCCStats

	pacer Pacer

	*FeedbackAdapter

	loss  *lossBasedBandwidthEstimator
	delay *delayBasedBandwidthEstimator

	packet   chan *packetAndAttributes
	feedback chan []rtcp.Packet
	close    chan struct{}
}

func (c *GCCInterceptor) getTargetBitrate() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.bitrate
}

func (c *GCCInterceptor) getStats() *GCCStats {
	return c.latestStats
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once
// per sender/receiver, however this might change in the future. The returned
// method will be called once per packet batch.
func (c *GCCInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {

		i, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}
		if attr == nil {
			attr = make(interceptor.Attributes)
		}

		pkts, err := attr.GetRTCPPackets(b[:i])
		if err != nil {
			return 0, nil, err
		}
		c.feedback <- pkts

		return i, attr, nil
	})
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once
// for per LocalStream. The returned method will be called once per rtp packet.
func (c *GCCInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {

	var hdrExtID uint8
	for _, e := range info.RTPHeaderExtensions {
		if e.URI == transportCCURI {
			hdrExtID = uint8(e.ID)
			break
		}
	}

	c.pacer.AddStream(info.SSRC, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {

		// Call adapter.onSent
		c.OnSent(time.Now(), header, len(payload), attributes)

		return writer.Write(header, payload, attributes)
	}))

	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {

		if hdrExtID != 0 {
			if attributes == nil {
				attributes = make(interceptor.Attributes)
			}
			attributes.Set(twccExtensionAttributesKey, hdrExtID)
		}
		c.packet <- &packetAndAttributes{
			header:     *header,
			payload:    payload,
			attributes: attributes,
		}

		return header.MarshalSize() + len(payload), nil
	})
}

func (c *GCCInterceptor) Close() error {
	close(c.close)
	return c.delay.Close()
}

func (c *GCCInterceptor) loop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	for {
		select {
		case <-c.close:
		case pkt := <-c.packet:
			c.pacer.Write(&pkt.header, pkt.payload, pkt.attributes)
		case pkts := <-c.feedback:
			for _, pkt := range pkts {
				acks, err := c.OnFeedback(pkt)
				if err != nil {
					// TODO
				}
				c.loss.updateLossStats(acks)
				c.delay.incomingFeedback(acks)
			}

		case <-ticker.C:
			dbr := c.delay.getEstimate()
			lbr := c.loss.getEstimate(dbr.Bitrate)
			c.lock.Lock()
			c.bitrate = min(dbr.Bitrate, lbr)
			c.pacer.SetTargetBitrate(c.bitrate)
			c.lock.Unlock()

			c.latestStats = &GCCStats{
				LossBasedEstimate: lbr,
				DelayStats:        dbr,
			}
		}
	}
}
