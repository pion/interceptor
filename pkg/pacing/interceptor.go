package pacing

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtp"
)

type Pacer interface {
	Budget(t time.Time) int
	OnSent(t time.Time, size int)
}

type PacerFactory func() Pacer

type InterceptorFactory struct {
	pf   PacerFactory
	opts []Option
}

type Option func(*Interceptor) error

func NewInterceptor(pf PacerFactory) (*InterceptorFactory, error) {
	return &InterceptorFactory{
		pf: pf,
	}, nil
}

func Interval(interval time.Duration) Option {
	return func(i *Interceptor) error {
		i.interval = interval
		return nil
	}
}

func QueueSize(size int) Option {
	return func(i *Interceptor) error {
		i.queueSize = size
		return nil
	}
}

func (f *InterceptorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	ctx, cancel := context.WithCancel(context.Background())
	i := &Interceptor{
		NoOp:     interceptor.NoOp{},
		lock:     sync.Mutex{},
		log:      logging.NewDefaultLoggerFactory().NewLogger("pacer_interceptor"),
		interval: 5 * time.Millisecond,
		pacer:    f.pf(),
		queue:    nil,
		close:    ctx,
		cancelFn: cancel,
	}
	for _, opt := range f.opts {
		if err := opt(i); err != nil {
			return nil, err
		}
	}
	i.queue = make(chan packetToSend, i.queueSize)
	go i.run()
	return i, nil
}

type Interceptor struct {
	interceptor.NoOp
	lock      sync.Mutex
	log       logging.LeveledLogger
	interval  time.Duration
	pacer     Pacer
	queueSize int
	queue     chan packetToSend
	close     context.Context
	cancelFn  context.CancelFunc
}

type packetToSend struct {
	header     *rtp.Header
	payload    []byte
	attributes interceptor.Attributes
	writer     interceptor.RTPWriter
}

// BindLocalStream implements interceptor.Interceptor.
func (i *Interceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		ch := header.Clone()
		buf := make([]byte, len(payload))
		if n := copy(buf, payload); n != len(payload) {
			return n, errors.New("copied wrong payload length")
		}
		select {
		case i.queue <- packetToSend{
			header:     &ch,
			payload:    buf,
			attributes: attributes,
			writer:     writer,
		}:
		default:
			return header.MarshalSize() + len(payload), errors.New("pacer dropped packet due to queue overflow")
		}
		return header.MarshalSize() + len(payload), nil
	})
}

// Close implements interceptor.Interceptor.
func (i *Interceptor) Close() error {
	i.cancelFn()
	return nil
}

func (i *Interceptor) run() {
	ticker := time.NewTicker(i.interval)
	for {
		select {
		case <-i.close.Done():
			return
		case <-ticker.C:
			for pkt := range i.queue {
				size := pkt.header.MarshalSize() + len(pkt.payload)
				now := time.Now()
				budget := i.pacer.Budget(now)
				if budget < size {
					log.Printf("budget too small: %v, queuesize=%v", budget, len(i.queue))
				}
				n, err := pkt.writer.Write(pkt.header, pkt.payload, pkt.attributes)
				if err != nil {
					log.Printf("error while writing packet: %v", err)
				}
				if n != size {
					log.Printf("copied wrong payload length")
				}
				i.pacer.OnSent(now, size)
			}
			ticker.Reset(i.interval)
		}
	}
}
