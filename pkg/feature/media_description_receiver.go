package feature

import (
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/pion/sdp/v3"
)

// MediaDescriptionReceiver is a store for media descriptions to find them by SSRC.
type MediaDescriptionReceiver struct {
	sync.RWMutex

	bySSRC map[uint32]*sdp.MediaDescription
}

// NewMediaDescriptionReceiver creates a new MediaDescriptionReceiver
func NewMediaDescriptionReceiver() *MediaDescriptionReceiver {
	return &MediaDescriptionReceiver{
		bySSRC: make(map[uint32]*sdp.MediaDescription),
	}
}

// GetMediaDescriptionForSSRC finds the MediaDescription for the given SSRC.
func (s *MediaDescriptionReceiver) GetMediaDescriptionForSSRC(ssrc uint32) (*sdp.MediaDescription, bool) {
	s.RLock()
	defer s.RUnlock()

	md, found := s.bySSRC[ssrc]
	return md, found
}

// GetClockRate returns the clock rate for the given SSRC and payload type.
func (s *MediaDescriptionReceiver) GetClockRate(ssrc uint32, pt uint8) (uint32, bool) {
	md, ok := s.GetMediaDescriptionForSSRC(ssrc)
	if !ok {
		return 0, false
	}

	for _, attr := range md.Attributes {
		if attr.Key == "rtpmap" {
			tokens := strings.Split(attr.Value, " ")
			attrPt, err := strconv.ParseUint(tokens[0], 10, 8)
			if err != nil {
				continue
			}
			if uint8(attrPt) == pt {
				props := strings.Split(tokens[1], "/")
				rate, err := strconv.ParseUint(props[1], 10, 32)
				if err != nil {
					continue
				}
				return uint32(rate), true
			}
		}
	}
	return 0, false
}

// WriteSDP writes a SessionDescription to the receiver.
func (s *MediaDescriptionReceiver) WriteSDP(sdp *sdp.SessionDescription) error {
	// write all the individual MediaDescriptions.
	for _, md := range sdp.MediaDescriptions {
		if err := s.WriteMediaDescription(md); err != nil {
			return err
		}
	}

	return nil
}

var errMissingSSRC = errors.New("missing ssrc")

// WriteMediaDescription writes a media description to the receiver.
func (s *MediaDescriptionReceiver) WriteMediaDescription(md *sdp.MediaDescription) error {
	// find the ssrc.
	ssrc, found := uint32(0), false
	for _, attr := range md.Attributes {
		if attr.Key == "ssrc" {
			s, err := strconv.ParseUint(attr.Value, 10, 32)
			if err != nil {
				return err
			}
			ssrc = uint32(s)
			found = true
			break
		}
	}
	if !found {
		return errMissingSSRC
	}

	// write the media description.
	s.Lock()
	defer s.Unlock()
	s.bySSRC[ssrc] = md

	return nil
}
