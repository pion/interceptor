package interceptor

import (
	"errors"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

type rtpHeaderKeyType int

const (
	rtpHeaderKey rtpHeaderKeyType = iota
	rtcpHeaderKey
)

var errInvalidHeaderType = errors.New("found invalid header type in attributes map")

// Attributes are a generic key/value store used by interceptors
type Attributes map[interface{}]interface{}

// Get returns the attribute associated with key.
func (a Attributes) Get(key interface{}) interface{} {
	return a[key]
}

// Set sets the attribute associated with key to the given value.
func (a Attributes) Set(key interface{}, val interface{}) {
	a[key] = val
}

// GetRTPHeader gets the RTP header if present. If the header is not present, it
// will be unmarshaled from the raw byte slice and stored in the attributes.
func (a Attributes) GetRTPHeader(raw []byte) (*rtp.Header, error) {
	if val, ok := a[rtpHeaderKey]; ok {
		if header, ok := val.(*rtp.Header); ok {
			return header, nil
		}
		return nil, errInvalidHeaderType
	}
	header := &rtp.Header{}
	_, err := header.Unmarshal(raw)
	if err != nil {
		return nil, err
	}
	a[rtpHeaderKey] = header
	return header, nil
}

// GetRTCPHeader gets the RTCP header if present. If the header is not present,
// it will be unmarshaled from the raw byte slice and stored in the attributes.
func (a Attributes) GetRTCPHeader(raw []byte) (*rtcp.Header, error) {
	if val, ok := a[rtcpHeaderKey]; ok {
		if header, ok := val.(*rtcp.Header); ok {
			return header, nil
		}
		return nil, errInvalidHeaderType
	}
	header := &rtcp.Header{}
	err := header.Unmarshal(raw)
	if err != nil {
		return nil, err
	}
	a[rtcpHeaderKey] = header
	return header, nil
}
