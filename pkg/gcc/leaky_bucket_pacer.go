// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"container/list"
	"errors"
	"io"
	"maps"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/rtpbuffer"
	"github.com/pion/logging"
	"github.com/pion/rtp"
)

var errLeakyBucketPacerPoolCastFailed = errors.New("failed to access leaky bucket pacer pool, cast failed")

type item struct {
	header     *rtp.Header
	payload    *[]byte
	attributes interceptor.Attributes
	resend     *rtpbuffer.Resend
}

// LeakyBucketPacer implements a leaky bucket pacing algorithm.
type LeakyBucketPacer struct {
	log logging.LeveledLogger

	f                 float64
	targetBitrate     int
	targetBitrateLock sync.Mutex

	pacingInterval time.Duration

	qLock sync.RWMutex
	queue *list.List
	done  chan struct{}

	ssrcToWriter map[uint32]interceptor.RTPWriter
	writerLock   sync.RWMutex

	pool *sync.Pool
}

// NewLeakyBucketPacer initializes a new LeakyBucketPacer.
func NewLeakyBucketPacer(initialBitrate int) *LeakyBucketPacer {
	return newLeakyBucketPacer(initialBitrate, logging.NewDefaultLoggerFactory())
}

func newLeakyBucketPacer(initialBitrate int, loggerFactory logging.LoggerFactory) *LeakyBucketPacer {
	pacer := &LeakyBucketPacer{
		log:            loggerFactory.NewLogger("pacer"),
		f:              1.5,
		targetBitrate:  initialBitrate,
		pacingInterval: 5 * time.Millisecond,
		qLock:          sync.RWMutex{},
		queue:          list.New(),
		done:           make(chan struct{}),
		ssrcToWriter:   map[uint32]interceptor.RTPWriter{},
		pool:           &sync.Pool{},
	}
	pacer.pool = &sync.Pool{
		New: func() any {
			b := make([]byte, 1460)

			return &b
		},
	}

	go pacer.Run()

	return pacer
}

// AddStream adds a new stream and its corresponding writer to the pacer.
func (p *LeakyBucketPacer) AddStream(ssrc uint32, writer interceptor.RTPWriter) {
	p.writerLock.Lock()
	defer p.writerLock.Unlock()
	p.ssrcToWriter[ssrc] = writer
}

// SetTargetBitrate updates the target bitrate at which the pacer is allowed to
// send packets. The pacer may exceed this limit by p.f.
func (p *LeakyBucketPacer) SetTargetBitrate(rate int) {
	p.targetBitrateLock.Lock()
	defer p.targetBitrateLock.Unlock()
	p.targetBitrate = int(p.f * float64(rate))
}

func (p *LeakyBucketPacer) getTargetBitrate() int {
	p.targetBitrateLock.Lock()
	defer p.targetBitrateLock.Unlock()

	return p.targetBitrate
}

// Write sends a packet with header and payload the a previously registered
// stream.
func (p *LeakyBucketPacer) Write(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
	p.qLock.Lock()
	defer p.qLock.Unlock()
	select {
	case <-p.done:
		return 0, io.ErrClosedPipe
	default:
	}
	resend, _ := attributes[rtpbuffer.ResendKey{}].(*rtpbuffer.Resend)
	buf, ok := p.pool.Get().(*[]byte)
	if !ok {
		return 0, errLeakyBucketPacerPoolCastFailed
	}

	*buf = append((*buf)[:0], payload...)
	hdr := header.Clone()

	resend.Retain()
	p.queue.PushBack(&item{
		header:     &hdr,
		payload:    buf,
		attributes: maps.Clone(attributes),
		resend:     resend,
	})

	return header.MarshalSize() + len(payload), nil
}

// Run starts the LeakyBucketPacer.
func (p *LeakyBucketPacer) Run() {
	ticker := time.NewTicker(p.pacingInterval)
	defer ticker.Stop()

	lastSent := time.Now()
	for {
		select {
		case <-p.done:
			return
		case now := <-ticker.C:
			budget := int(float64(now.Sub(lastSent).Milliseconds()) * float64(p.getTargetBitrate()) / 8000.0)
			p.qLock.Lock()
			for p.queue.Len() != 0 && budget > 0 {
				next, ok := p.queue.Front().Value.(*item)
				if !ok {
					p.log.Warnf("failed to access leaky bucket pacer queue, cast failed")
					p.queue.Remove(p.queue.Front())

					continue
				}
				p.queue.Remove(p.queue.Front())
				p.qLock.Unlock()
				budget -= p.write(next)
				lastSent = now
				p.pool.Put(next.payload)
				p.qLock.Lock()
			}
			p.qLock.Unlock()
		}
	}
}

func (p *LeakyBucketPacer) write(next *item) int {
	p.writerLock.RLock()
	writer, ok := p.ssrcToWriter[next.header.SSRC]
	p.writerLock.RUnlock()
	if !ok {
		p.log.Warnf("no writer found for ssrc: %v", next.header.SSRC)
		next.resend.Release(false)

		return 0
	}
	n, err := writer.Write(next.header, *next.payload, next.attributes)
	if err != nil {
		p.log.Errorf("failed to write packet: %v", err)
	}

	next.resend.Release(err == nil)

	return n
}

// Close closes the LeakyBucketPacer.
func (p *LeakyBucketPacer) Close() error {
	p.qLock.Lock()
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	queue := p.queue
	p.queue = list.New()
	p.qLock.Unlock()
	for elem := queue.Front(); elem != nil; elem = elem.Next() {
		if next, ok := elem.Value.(*item); ok {
			p.pool.Put(next.payload)
			next.resend.Release(false)
		}
	}

	return nil
}
