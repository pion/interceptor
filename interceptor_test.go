// +build !js

package interceptor

import (
	"context"
	"errors"
	"testing"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

func TestRTPReader_ContextCancel(t *testing.T) {
	r := RTPReaderFunc(func(ctx context.Context) (*rtp.Packet, Attributes, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := r.Read(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected '%v', got '%v'", context.Canceled, err)
	}
}

func TestRTCPReader_ContextCancel(t *testing.T) {
	r := RTCPReaderFunc(func(ctx context.Context) ([]rtcp.Packet, Attributes, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := r.Read(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected '%v', got '%v'", context.Canceled, err)
	}
}

func TestRTPWriter_ContextCancel(t *testing.T) {
	r := RTPWriterFunc(func(ctx context.Context, p *rtp.Packet, attr Attributes) (int, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Write(ctx, &rtp.Packet{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected '%v', got '%v'", context.Canceled, err)
	}
}

func TestRTCPWriter_ContextCancel(t *testing.T) {
	r := RTCPWriterFunc(func(ctx context.Context, p []rtcp.Packet, attr Attributes) (int, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Write(ctx, []rtcp.Packet{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected '%v', got '%v'", context.Canceled, err)
	}
}
