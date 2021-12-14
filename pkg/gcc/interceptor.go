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

// ErrUnknownSession indicates that a session ID was not assigned
var ErrUnknownSession = errors.New("unknown session ID")

// Option can be used to set initial options on GCC interceptors
type Option func(*Interceptor) error

// InitialBitrate sets the initial bitrate of new GCC interceptors
func InitialBitrate(rate int) Option {
	return func(g *Interceptor) error {
		g.bitrate = rate
		return nil
	}
}

// InterceptorFactory is a factory for GCC interceptors
type InterceptorFactory struct {
	opts         []Option
	interceptors map[string]*Interceptor
}

// NewInterceptor returns a new GCC interceptor factory
func NewInterceptor(opts ...Option) (*InterceptorFactory, error) {
	return &InterceptorFactory{
		opts:         opts,
		interceptors: map[string]*Interceptor{},
	}, nil
}

// NewInterceptor returns a new GCC interceptor
func (f *InterceptorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	if i, ok := f.interceptors[id]; ok {
		return i, nil
	}
	i := &Interceptor{
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

// GetTargetBitrate returns the target bitrate for the connection with id
func (f *InterceptorFactory) GetTargetBitrate(id string) (int, error) {
	if i, ok := f.interceptors[id]; ok {
		return i.getTargetBitrate(), nil
	}
	return 0, fmt.Errorf("%w: %v", ErrUnknownSession, id)
}

// Stats contains internal statistics of the bandwidth estimator
type Stats struct {
	LossBasedEstimate int
	DelayStats
}

// GetStats returns a sample of the internal statistics of the interceptor
// running on the connection with id
func (f *InterceptorFactory) GetStats(id string) (*Stats, error) {
	if i, ok := f.interceptors[id]; ok {
		return i.getStats(), nil
	}
	return nil, fmt.Errorf("%w: %v", ErrUnknownSession, id)
}

// Pacer is the interface implemented by packet pacers
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

// Interceptor implements Google Congestion Control
type Interceptor struct {
	interceptor.NoOp

	lock    sync.Mutex
	bitrate int

	latestStats *Stats

	pacer Pacer

	*FeedbackAdapter

	loss  *lossBasedBandwidthEstimator
	delay *delayBasedBandwidthEstimator

	packet   chan *packetAndAttributes
	feedback chan []rtcp.Packet
	close    chan struct{}
}

func (c *Interceptor) getTargetBitrate() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.bitrate
}

func (c *Interceptor) getStats() *Stats {
	return c.latestStats
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
func (c *Interceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	var hdrExtID uint8
	for _, e := range info.RTPHeaderExtensions {
		if e.URI == transportCCURI {
			hdrExtID = uint8(e.ID)
			break
		}
	}

	c.pacer.AddStream(info.SSRC, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		// Call adapter.onSent
		if err := c.OnSent(time.Now(), header, len(payload), attributes); err != nil {
			return 0, err
		}

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

// Close closes c
func (c *Interceptor) Close() error {
	close(c.close)
	return c.delay.Close()
}

func (c *Interceptor) loop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	for {
		select {
		case <-c.close:
		case pkt := <-c.packet:
			_, err := c.pacer.Write(&pkt.header, pkt.payload, pkt.attributes)
			if err != nil {
				// TODO
				panic(err)
			}
		case pkts := <-c.feedback:
			for _, pkt := range pkts {
				acks, err := c.OnFeedback(pkt)
				if err != nil {
					// TODO
					panic(err)
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

			c.latestStats = &Stats{
				LossBasedEstimate: lbr,
				DelayStats:        dbr,
			}
		}
	}
}
