// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package pacing implements a pacing interceptor.
package pacing

import (
	"errors"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtp"
)

var (
	errPacerClosed   = errors.New("pacer closed")
	errPacerOverflow = errors.New("pacer queue overflow")
)

type pacerFactory func(initialRate, burst int) pacer

type pacer interface {
	SetRate(rate, burst int)
	Budget(time.Time) float64
	AllowN(time.Time, int) bool
}

// Option is a configuration option for pacing interceptors.
type Option func(*Interceptor) error

// InitialRate configures the initial pacing rate for interceptors created by
// the interceptor factory.
func InitialRate(rate int) Option {
	return func(i *Interceptor) error {
		i.initialRate = rate

		return nil
	}
}

// Interval configures the pacing interval for interceptors created by the
// interceptor factory.
func Interval(interval time.Duration) Option {
	return func(i *Interceptor) error {
		i.interval = interval

		return nil
	}
}

// WithLoggerFactory sets a logger factory for the interceptor.
func WithLoggerFactory(loggerFactory logging.LoggerFactory) Option {
	return func(i *Interceptor) error {
		i.loggerFactory = loggerFactory

		return nil
	}
}

func setPacerFactory(f pacerFactory) Option {
	return func(i *Interceptor) error {
		i.pacerFactory = f

		return nil
	}
}

// InterceptorFactory is a factory for pacing interceptors. It also keeps a map
// of interceptors created in the past by ID.
type InterceptorFactory struct {
	lock         sync.Mutex
	opts         []Option
	interceptors map[string]*Interceptor
}

// NewInterceptor returns a new InterceptorFactory.
func NewInterceptor(opts ...Option) *InterceptorFactory {
	return &InterceptorFactory{
		lock:         sync.Mutex{},
		opts:         opts,
		interceptors: map[string]*Interceptor{},
	}
}

// SetRate updates the pacing rate of the pacing interceptor with the given ID.
func (f *InterceptorFactory) SetRate(id string, r int) {
	f.lock.Lock()
	defer f.lock.Unlock()

	i, ok := f.interceptors[id]
	if !ok {
		return
	}
	i.setRate(r)
}

func (f *InterceptorFactory) remove(id string) {
	f.lock.Lock()
	defer f.lock.Unlock()
	delete(f.interceptors, id)
}

// NewInterceptor creates a new pacing interceptor.
func (f *InterceptorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	interceptor := &Interceptor{
		NoOp:        interceptor.NoOp{},
		initialRate: 1_000_000,
		interval:    5 * time.Millisecond,
		queueSize:   1_000_000,
		pacerFactory: func(initialRate, burst int) pacer {
			return newRateLimitPacer(initialRate, burst)
		},
		limit:   nil,
		queue:   nil,
		closed:  make(chan struct{}),
		wg:      sync.WaitGroup{},
		id:      id,
		onClose: f.remove,
	}
	for _, opt := range f.opts {
		if err := opt(interceptor); err != nil {
			return nil, err
		}
	}
	if interceptor.loggerFactory == nil {
		interceptor.loggerFactory = logging.NewDefaultLoggerFactory()
	}
	interceptor.log = interceptor.loggerFactory.NewLogger("pacer_interceptor")
	interceptor.limit = interceptor.pacerFactory(
		interceptor.initialRate,
		burst(interceptor.initialRate, interceptor.interval),
	)
	interceptor.queue = make(chan packet, interceptor.queueSize)

	f.interceptors[id] = interceptor

	interceptor.wg.Add(1)
	go func() {
		defer interceptor.wg.Done()
		interceptor.loop()
	}()

	return interceptor, nil
}

// Interceptor implements packet pacing using a token bucket filter and sends
// packets at a fixed interval.
type Interceptor struct {
	interceptor.NoOp
	log           logging.LeveledLogger
	loggerFactory logging.LoggerFactory

	// config
	initialRate  int
	interval     time.Duration
	queueSize    int
	pacerFactory pacerFactory

	// limiter and queue
	limit pacer
	queue chan packet

	// shutdown
	closed  chan struct{}
	wg      sync.WaitGroup
	id      string
	onClose func(string)
}

// burst calculates the minimal burst size required to reach the given rate and
// pacing interval.
func burst(rate int, interval time.Duration) int {
	if interval == 0 {
		interval = time.Millisecond
	}
	f := float64(time.Second.Milliseconds() / interval.Milliseconds())

	return 8 * int(float64(rate)/f)
}

// setRate updates the pacing rate and burst of the rate limiter.
func (i *Interceptor) setRate(r int) {
	i.limit.SetRate(r, burst(r, i.interval))
}

// BindLocalStream implements interceptor.Interceptor.
func (i *Interceptor) BindLocalStream(
	info *interceptor.StreamInfo,
	writer interceptor.RTPWriter,
) interceptor.RTPWriter {
	return interceptor.RTPWriterFunc(func(
		header *rtp.Header,
		payload []byte,
		attributes interceptor.Attributes,
	) (int, error) {
		hdr := header.Clone()
		pay := make([]byte, len(payload))
		copy(pay, payload)
		attr := maps.Clone(attributes)
		select {
		case i.queue <- packet{
			writer:     writer,
			header:     &hdr,
			payload:    pay,
			attributes: attr,
		}:
		case <-i.closed:
			return 0, errPacerClosed
		default:
			return 0, errPacerOverflow
		}

		return header.MarshalSize() + len(payload), nil
	})
}

// Close implements interceptor.Interceptor.
func (i *Interceptor) Close() error {
	defer i.wg.Wait()
	close(i.closed)
	i.onClose(i.id)

	return nil
}

func (i *Interceptor) loop() {
	ticker := time.NewTicker(i.interval)
	defer ticker.Stop()
	queue := make([]packet, 0)
	for {
		select {
		case now := <-ticker.C:
			for len(queue) > 0 && i.limit.Budget(now) > 8*float64(queue[0].len()) {
				i.limit.AllowN(now, 8*queue[0].len())
				var next packet
				next, queue = queue[0], queue[1:]
				if _, err := next.writer.Write(next.header, next.payload, next.attributes); err != nil {
					slog.Warn("error on writing RTP packet", "error", err)
				}
			}
		case pkt := <-i.queue:
			queue = append(queue, pkt)
		case <-i.closed:
			return
		}
	}
}

type packet struct {
	writer     interceptor.RTPWriter
	header     *rtp.Header
	payload    []byte
	attributes interceptor.Attributes
}

func (p *packet) len() int {
	return p.header.MarshalSize() + len(p.payload)
}
