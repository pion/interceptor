//+build scream

package scream

import "github.com/mengelbart/scream-go"

// SenderOption can be used to configure SenderInterceptor.
type SenderOption func(r *SenderInterceptor) error

// SenderQueue sets the factory function to create new RTP Queues for new streams.
func SenderQueue(queueFactory func() RTPQueue) SenderOption {
	return func(s *SenderInterceptor) error {
		s.newRTPQueue = queueFactory
		return nil
	}
}

func Tx(tx *scream.Tx) SenderOption {
	return func(s *SenderInterceptor) error {
		s.tx = tx
		return nil
	}
}
