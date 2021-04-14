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

func MinBitrate(rate float64) SenderOption {
	return func(s *SenderInterceptor) error {
		s.minBitrate = rate
		return nil
	}
}

func InitialBitrate(rate float64) SenderOption {
	return func(s *SenderInterceptor) error {
		s.initialBitrate = rate
		return nil
	}
}

func MaxBitrate(rate float64) SenderOption {
	return func(s *SenderInterceptor) error {
		s.maxBitrate = rate
		return nil
	}
}
