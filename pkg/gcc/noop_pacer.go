package gcc

import (
	"errors"
	"fmt"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

var ErrUnknownStream = errors.New("unknown ssrc")

type NoOpPacer struct {
	lock         sync.Mutex
	ssrcToWriter map[uint32]interceptor.RTPWriter
}

func NewNoOpPacer() *NoOpPacer {
	return &NoOpPacer{
		lock:         sync.Mutex{},
		ssrcToWriter: map[uint32]interceptor.RTPWriter{},
	}
}

func (p *NoOpPacer) AddStream(ssrc uint32, writer interceptor.RTPWriter) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.ssrcToWriter[ssrc] = writer
}

func (p *NoOpPacer) Write(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if w, ok := p.ssrcToWriter[header.SSRC]; ok {
		return w.Write(header, payload, attributes)
	}

	return 0, fmt.Errorf("%w: %v", ErrUnknownStream, header.SSRC)
}

func (p *NoOpPacer) Close() error {
	return nil
}
