package gcc

import (
	"errors"
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

// Pacer is the interface implemented by packet pacers
type Pacer interface {
	interceptor.RTPWriter
	AddStream(ssrc uint32, writer interceptor.RTPWriter)
	SetTargetBitrate(int)
	Close() error
}

// Option can be used to set initial options on GCC interceptors
type Option func(*Interceptor) error

// InitialBitrate sets the initial bitrate of new GCC interceptors
func InitialBitrate(rate int) Option {
	return func(g *Interceptor) error {
		g.bitrate = rate
		return nil
	}
}

func SetPacer(pacer Pacer) Option {
	return func(g *Interceptor) error {
		g.pacer = pacer
		return nil
	}
}

type BandwidthEstimator interface {
	GetTargetBitrate() int
	GetStats() map[string]interface{}
	OnTargetBitrateChange(f func(bitrate int))
}

type NewPeerConnectionCallback func(id string, estimator BandwidthEstimator)

// InterceptorFactory is a factory for GCC interceptors
type InterceptorFactory struct {
	opts              []Option
	addPeerConnection NewPeerConnectionCallback
}

// NewInterceptor returns a new GCC interceptor factory
func NewInterceptor(opts ...Option) (*InterceptorFactory, error) {
	return &InterceptorFactory{
		opts: opts,
	}, nil
}

func (f *InterceptorFactory) OnNewPeerConnection(cb NewPeerConnectionCallback) {
	f.addPeerConnection = cb
}

// NewInterceptor returns a new GCC interceptor
func (f *InterceptorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	i := &Interceptor{
		NoOp:            interceptor.NoOp{},
		lock:            sync.Mutex{},
		bitrate:         100_000,
		latestStats:     &Stats{},
		pacer:           nil,
		FeedbackAdapter: nil,
		loss:            nil,
		delay:           nil,
		feedback:        make(chan []rtcp.Packet),
		close:           make(chan struct{}),
	}

	for _, opt := range f.opts {
		if err := opt(i); err != nil {
			return nil, err
		}
	}

	if i.pacer == nil {
		i.pacer = NewLeakyBucketPacer(i.bitrate)
	}
	i.FeedbackAdapter = NewFeedbackAdapter()
	i.loss = newLossBasedBWE(i.bitrate)
	i.delay = newDelayBasedBWE(i.bitrate)

	go i.loop()
	if f.addPeerConnection != nil {
		f.addPeerConnection(id, i)
	}
	return i, nil
}

// Stats contains internal statistics of the bandwidth estimator
type Stats struct {
	LossBasedEstimate int
	DelayStats
}

// Interceptor implements Google Congestion Control
type Interceptor struct {
	interceptor.NoOp

	lock    sync.Mutex
	bitrate int

	statsLock   sync.Mutex
	latestStats *Stats

	pacer Pacer

	*FeedbackAdapter

	loss  *lossBasedBandwidthEstimator
	delay *delayBasedBandwidthEstimator

	feedback chan []rtcp.Packet
	close    chan struct{}

	onTargetBitrateChange func(bitrate int)
}

func (c *Interceptor) GetTargetBitrate() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.bitrate
}

func (c *Interceptor) OnTargetBitrateChange(f func(bitrate int)) {
	c.onTargetBitrateChange = f
}

func (c *Interceptor) GetStats() map[string]interface{} {
	c.statsLock.Lock()
	defer c.statsLock.Unlock()
	return map[string]interface{}{
		"lossEstimate":  c.latestStats.LossBasedEstimate,
		"delayEstimate": c.latestStats.Bitrate,
		"estimate":      c.latestStats.Estimate,
		"thresh":        c.latestStats.Threshold,
		"rtt":           c.latestStats.RTT,
	}
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
		if err := c.OnSent(time.Now(), header, len(payload), attributes); err != nil && err != errMissingTWCCExtension {
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
		c.pacer.Write(header, payload, attributes)

		return header.MarshalSize() + len(payload), nil
	})
}

// Close closes c
func (c *Interceptor) Close() error {
	close(c.close)
	return c.delay.Close()
}

func (c *Interceptor) loop() {
	ticker := time.NewTicker(200 * time.Millisecond)
	for {
		select {
		case <-c.close:
		case pkts := <-c.feedback:
			for _, pkt := range pkts {
				acks, err := c.OnFeedback(time.Now(), pkt)
				if err != nil {
					// TODO Add log warning?
				}
				c.loss.updateLossStats(acks)
				c.delay.incomingFeedback(acks)
			}

		case <-ticker.C:
			dbr := c.delay.getEstimate()
			lbr := c.loss.getEstimate(dbr.Bitrate)
			c.lock.Lock()
			bitrateChanged := false
			bitrate := min(dbr.Bitrate, lbr)
			if bitrate != c.bitrate {
				bitrateChanged = true
				c.bitrate = bitrate
				c.pacer.SetTargetBitrate(c.bitrate)
			}
			c.lock.Unlock()

			c.statsLock.Lock()
			c.latestStats = &Stats{
				LossBasedEstimate: lbr,
				DelayStats:        dbr,
			}
			c.statsLock.Unlock()

			if bitrateChanged && c.onTargetBitrateChange != nil {
				c.onTargetBitrateChange(bitrate)
			}
		}
	}
}
