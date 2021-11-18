package report

import (
	"testing"
	"time"

	"github.com/pion/interceptor/v2/internal/test"
	"github.com/pion/interceptor/v2/pkg/feature"
	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/stretchr/testify/assert"
)

func TestReceiverInterceptor(t *testing.T) {
	rtpTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	t.Run("after RTP packets", func(t *testing.T) {
		mt := test.MockTime{}
		md := feature.NewMediaDescriptionReceiver()
		assert.NoError(t, md.WriteMediaDescription(&sdp.MediaDescription{
			Attributes: []sdp.Attribute{
				{Key: "ssrc", Value: "123456"},
				{Key: "rtpmap", Value: "96 VP8/90000"},
			},
		}))
		i, err := NewReceiverInterceptor(
			md,
			ReceiverInterval(time.Millisecond*50),
			ReceiverLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			ReceiverNow(mt.Now),
		)
		assert.NoError(t, err)

		defer func() {
			assert.NoError(t, i.Close())
		}()

		rtcpOut, rtcpWriter := rtpio.RTCPPipe()
		rtpReader, rtpIn := rtpio.RTPPipe()

		rtpOut := i.Transform(rtcpWriter, rtpReader, nil)

		for i := 0; i < 10; i++ {
			go func() {
				_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
					SSRC:           123456,
					PayloadType:    96,
					SequenceNumber: uint16(i),
				}})
				assert.NoError(t, err2)
			}()

			p := &rtp.Packet{}
			_, err2 := rtpOut.ReadRTP(p)
			assert.NoError(t, err2)
			assert.Equal(t, uint16(i), p.Header.SequenceNumber)
		}

		pkts := make([]rtcp.Packet, 16)
		n, err := rtcpOut.ReadRTCP(pkts)
		assert.NoError(t, err)
		assert.Equal(t, n, 1)
		rr, ok := pkts[0].(*rtcp.ReceiverReport)
		assert.True(t, ok)
		assert.Equal(t, 1, len(rr.Reports))
		assert.Equal(t, rtcp.ReceptionReport{
			SSRC:               uint32(123456),
			LastSequenceNumber: 9,
			LastSenderReport:   0,
			FractionLost:       0,
			TotalLost:          0,
			Delay:              0,
			Jitter:             0,
		}, rr.Reports[0])
	})

	t.Run("after RTP and RTCP packets", func(t *testing.T) {
		mt := test.MockTime{}
		md := feature.NewMediaDescriptionReceiver()
		assert.NoError(t, md.WriteMediaDescription(&sdp.MediaDescription{
			Attributes: []sdp.Attribute{
				{Key: "ssrc", Value: "123456"},
				{Key: "rtpmap", Value: "96 VP8/90000"},
			},
		}))
		i, err := NewReceiverInterceptor(
			md,
			ReceiverInterval(time.Millisecond*50),
			ReceiverLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			ReceiverNow(mt.Now),
		)
		assert.NoError(t, err)

		defer func() {
			assert.NoError(t, i.Close())
		}()

		rtcpOut, rtcpWriter := rtpio.RTCPPipe()
		rtpReader, rtpIn := rtpio.RTPPipe()
		rtcpReader, rtcpIn := rtpio.RTCPPipe()

		rtpOut := i.Transform(rtcpWriter, rtpReader, rtcpReader)

		for i := 0; i < 10; i++ {
			go func() {
				_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
					SSRC:           123456,
					PayloadType:    96,
					SequenceNumber: uint16(i),
				}})
				assert.NoError(t, err2)
			}()

			p := &rtp.Packet{}
			_, err2 := rtpOut.ReadRTP(p)
			assert.NoError(t, err2)
			assert.Equal(t, uint16(i), p.Header.SequenceNumber)
		}

		now := time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC)
		go func() {
			_, err2 := rtcpIn.WriteRTCP([]rtcp.Packet{
				&rtcp.SenderReport{
					SSRC:        123456,
					NTPTime:     ntpTime(now),
					RTPTime:     987654321 + uint32(now.Sub(rtpTime).Seconds()*90000),
					PacketCount: 10,
					OctetCount:  0,
				},
			})
			assert.NoError(t, err2)
		}()

		pkts := make([]rtcp.Packet, 16)
		n, err := rtcpOut.ReadRTCP(pkts)
		assert.NoError(t, err)
		assert.Equal(t, n, 1)
		rr, ok := pkts[0].(*rtcp.ReceiverReport)
		assert.True(t, ok)
		assert.Equal(t, 1, len(rr.Reports))
		assert.Equal(t, rtcp.ReceptionReport{
			SSRC:               uint32(123456),
			LastSequenceNumber: 9,
			LastSenderReport:   1861287936,
			FractionLost:       0,
			TotalLost:          0,
			Delay:              rr.Reports[0].Delay,
			Jitter:             0,
		}, rr.Reports[0])
	})

	t.Run("overflow", func(t *testing.T) {
		mt := test.MockTime{}
		md := feature.NewMediaDescriptionReceiver()
		assert.NoError(t, md.WriteMediaDescription(&sdp.MediaDescription{
			Attributes: []sdp.Attribute{
				{Key: "ssrc", Value: "123456"},
				{Key: "rtpmap", Value: "96 VP8/90000"},
			},
		}))
		i, err := NewReceiverInterceptor(
			md,
			ReceiverInterval(time.Millisecond*50),
			ReceiverLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			ReceiverNow(mt.Now),
		)
		assert.NoError(t, err)

		defer func() {
			assert.NoError(t, i.Close())
		}()

		rtcpOut, rtcpWriter := rtpio.RTCPPipe()
		rtpReader, rtpIn := rtpio.RTPPipe()

		rtpOut := i.Transform(rtcpWriter, rtpReader, nil)

		go func() {
			_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
				SSRC:           123456,
				PayloadType:    96,
				SequenceNumber: 0xffff,
			}})
			assert.NoError(t, err2)
		}()

		p := &rtp.Packet{}
		_, err = rtpOut.ReadRTP(p)
		assert.NoError(t, err)
		assert.Equal(t, uint16(0xffff), p.Header.SequenceNumber)

		go func() {
			_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
				SSRC:           123456,
				PayloadType:    96,
				SequenceNumber: 0x00,
			}})
			assert.NoError(t, err2)
		}()

		_, err = rtpOut.ReadRTP(p)
		assert.NoError(t, err)
		assert.Equal(t, uint16(0x00), p.Header.SequenceNumber)

		pkts := make([]rtcp.Packet, 16)
		n, err := rtcpOut.ReadRTCP(pkts)
		assert.NoError(t, err)
		assert.Equal(t, n, 1)
		rr, ok := pkts[0].(*rtcp.ReceiverReport)
		assert.True(t, ok)
		assert.Equal(t, 1, len(rr.Reports))
		assert.Equal(t, rtcp.ReceptionReport{
			SSRC:               uint32(123456),
			LastSequenceNumber: 1<<16 | 0x0000,
			LastSenderReport:   0,
			FractionLost:       0,
			TotalLost:          0,
			Delay:              0,
			Jitter:             0,
		}, rr.Reports[0])
	})

	t.Run("packet loss", func(t *testing.T) {
		mt := test.MockTime{}
		md := feature.NewMediaDescriptionReceiver()
		assert.NoError(t, md.WriteMediaDescription(&sdp.MediaDescription{
			Attributes: []sdp.Attribute{
				{Key: "ssrc", Value: "123456"},
				{Key: "rtpmap", Value: "96 VP8/90000"},
			},
		}))
		i, err := NewReceiverInterceptor(
			md,
			ReceiverInterval(time.Millisecond*50),
			ReceiverLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			ReceiverNow(mt.Now),
		)
		assert.NoError(t, err)

		defer func() {
			assert.NoError(t, i.Close())
		}()

		rtcpOut, rtcpWriter := rtpio.RTCPPipe()
		rtpReader, rtpIn := rtpio.RTPPipe()
		rtcpReader, rtcpIn := rtpio.RTCPPipe()

		rtpOut := i.Transform(rtcpWriter, rtpReader, rtcpReader)

		go func() {
			_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
				SSRC:           123456,
				PayloadType:    96,
				SequenceNumber: 0x01,
			}})
			assert.NoError(t, err2)
		}()

		p := &rtp.Packet{}
		_, err = rtpOut.ReadRTP(p)
		assert.NoError(t, err)
		assert.Equal(t, uint16(0x01), p.Header.SequenceNumber)

		go func() {
			_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
				SSRC:           123456,
				PayloadType:    96,
				SequenceNumber: 0x03,
			}})
			assert.NoError(t, err2)
		}()

		_, err = rtpOut.ReadRTP(p)
		assert.NoError(t, err)
		assert.Equal(t, uint16(0x03), p.Header.SequenceNumber)

		pkts := make([]rtcp.Packet, 16)
		n, err := rtcpOut.ReadRTCP(pkts)
		assert.NoError(t, err)
		assert.Equal(t, n, 1)
		rr, ok := pkts[0].(*rtcp.ReceiverReport)
		assert.True(t, ok)
		assert.Equal(t, 1, len(rr.Reports))
		assert.Equal(t, rtcp.ReceptionReport{
			SSRC:               uint32(123456),
			LastSequenceNumber: 0x03,
			LastSenderReport:   0,
			FractionLost:       256 * 1 / 3,
			TotalLost:          1,
			Delay:              0,
			Jitter:             0,
		}, rr.Reports[0])

		now := time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC)
		_, err = rtcpIn.WriteRTCP([]rtcp.Packet{
			&rtcp.SenderReport{
				SSRC:        123456,
				NTPTime:     ntpTime(now),
				RTPTime:     987654321 + uint32(now.Sub(rtpTime).Seconds()*90000),
				PacketCount: 10,
				OctetCount:  0,
			},
		})
		assert.NoError(t, err)

		n, err = rtcpOut.ReadRTCP(pkts)
		assert.NoError(t, err)
		assert.Equal(t, n, 1)
		rr, ok = pkts[0].(*rtcp.ReceiverReport)
		assert.True(t, ok)
		assert.Equal(t, 1, len(rr.Reports))
		assert.Equal(t, rtcp.ReceptionReport{
			SSRC:               uint32(123456),
			LastSequenceNumber: 0x03,
			LastSenderReport:   1861287936,
			FractionLost:       0,
			TotalLost:          1,
			Delay:              rr.Reports[0].Delay,
			Jitter:             0,
		}, rr.Reports[0])
	})

	t.Run("overflow and packet loss", func(t *testing.T) {
		mt := test.MockTime{}
		md := feature.NewMediaDescriptionReceiver()
		assert.NoError(t, md.WriteMediaDescription(&sdp.MediaDescription{
			Attributes: []sdp.Attribute{
				{Key: "ssrc", Value: "123456"},
				{Key: "rtpmap", Value: "96 VP8/90000"},
			},
		}))
		i, err := NewReceiverInterceptor(
			md,
			ReceiverInterval(time.Millisecond*50),
			ReceiverLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			ReceiverNow(mt.Now),
		)
		assert.NoError(t, err)

		defer func() {
			assert.NoError(t, i.Close())
		}()

		rtcpOut, rtcpWriter := rtpio.RTCPPipe()
		rtpReader, rtpIn := rtpio.RTPPipe()

		rtpOut := i.Transform(rtcpWriter, rtpReader, nil)

		go func() {
			_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
				SSRC:           123456,
				PayloadType:    96,
				SequenceNumber: 0xffff,
			}})
			assert.NoError(t, err2)
		}()

		p := &rtp.Packet{}
		_, err = rtpOut.ReadRTP(p)
		assert.NoError(t, err)
		assert.Equal(t, uint16(0xffff), p.Header.SequenceNumber)

		go func() {
			_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
				SSRC:           123456,
				PayloadType:    96,
				SequenceNumber: 0x01,
			}})
			assert.NoError(t, err2)
		}()

		_, err = rtpOut.ReadRTP(p)
		assert.NoError(t, err)
		assert.Equal(t, uint16(0x01), p.Header.SequenceNumber)

		pkts := make([]rtcp.Packet, 16)
		n, err := rtcpOut.ReadRTCP(pkts)
		assert.NoError(t, err)
		assert.Equal(t, n, 1)
		rr, ok := pkts[0].(*rtcp.ReceiverReport)
		assert.True(t, ok)
		assert.Equal(t, 1, len(rr.Reports))
		assert.Equal(t, rtcp.ReceptionReport{
			SSRC:               uint32(123456),
			LastSequenceNumber: 1<<16 | 0x01,
			LastSenderReport:   0,
			FractionLost:       256 * 1 / 3,
			TotalLost:          1,
			Delay:              0,
			Jitter:             0,
		}, rr.Reports[0])
	})

	t.Run("reordered packets", func(t *testing.T) {
		mt := test.MockTime{}
		md := feature.NewMediaDescriptionReceiver()
		assert.NoError(t, md.WriteMediaDescription(&sdp.MediaDescription{
			Attributes: []sdp.Attribute{
				{Key: "ssrc", Value: "123456"},
				{Key: "rtpmap", Value: "96 VP8/90000"},
			},
		}))
		i, err := NewReceiverInterceptor(
			md,
			ReceiverInterval(time.Millisecond*50),
			ReceiverLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			ReceiverNow(mt.Now),
		)
		assert.NoError(t, err)

		defer func() {
			assert.NoError(t, i.Close())
		}()

		rtcpOut, rtcpWriter := rtpio.RTCPPipe()
		rtpReader, rtpIn := rtpio.RTPPipe()

		rtpOut := i.Transform(rtcpWriter, rtpReader, nil)

		for _, seqNum := range []uint16{0x01, 0x03, 0x02, 0x04} {
			go func() {
				_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
					SSRC:           123456,
					PayloadType:    96,
					SequenceNumber: seqNum,
				}})
				assert.NoError(t, err2)
			}()

			p := &rtp.Packet{}
			_, err = rtpOut.ReadRTP(p)
			assert.NoError(t, err)
			assert.Equal(t, seqNum, p.Header.SequenceNumber)
		}

		pkts := make([]rtcp.Packet, 16)
		n, err := rtcpOut.ReadRTCP(pkts)
		assert.NoError(t, err)
		assert.Equal(t, n, 1)
		rr, ok := pkts[0].(*rtcp.ReceiverReport)
		assert.True(t, ok)
		assert.Equal(t, 1, len(rr.Reports))
		assert.Equal(t, rtcp.ReceptionReport{
			SSRC:               uint32(123456),
			LastSequenceNumber: 0x04,
			LastSenderReport:   0,
			FractionLost:       0,
			TotalLost:          0,
			Delay:              0,
			Jitter:             0,
		}, rr.Reports[0])
	})

	t.Run("jitter", func(t *testing.T) {
		mt := test.MockTime{}
		md := feature.NewMediaDescriptionReceiver()
		assert.NoError(t, md.WriteMediaDescription(&sdp.MediaDescription{
			Attributes: []sdp.Attribute{
				{Key: "ssrc", Value: "123456"},
				{Key: "rtpmap", Value: "96 VP8/90000"},
			},
		}))
		i, err := NewReceiverInterceptor(
			md,
			ReceiverInterval(time.Millisecond*50),
			ReceiverLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			ReceiverNow(mt.Now),
		)
		assert.NoError(t, err)

		defer func() {
			assert.NoError(t, i.Close())
		}()

		rtcpOut, rtcpWriter := rtpio.RTCPPipe()
		rtpReader, rtpIn := rtpio.RTPPipe()

		rtpOut := i.Transform(rtcpWriter, rtpReader, nil)

		mt.SetNow(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
		go func() {
			_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
				SSRC:           123456,
				PayloadType:    96,
				SequenceNumber: 0x01,
				Timestamp:      42378934,
			}})
			assert.NoError(t, err2)
		}()

		p := &rtp.Packet{}
		_, err = rtpOut.ReadRTP(p)
		assert.NoError(t, err)
		assert.Equal(t, uint16(0x01), p.Header.SequenceNumber)

		mt.SetNow(time.Date(2009, time.November, 10, 23, 0, 1, 0, time.UTC))
		go func() {
			_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
				SSRC:           123456,
				PayloadType:    96,
				SequenceNumber: 0x02,
				Timestamp:      42378934 + 60000,
			}})
			assert.NoError(t, err2)
		}()

		_, err = rtpOut.ReadRTP(p)
		assert.NoError(t, err)
		assert.Equal(t, uint16(0x02), p.Header.SequenceNumber)

		pkts := make([]rtcp.Packet, 16)
		n, err := rtcpOut.ReadRTCP(pkts)
		assert.NoError(t, err)
		assert.Equal(t, n, 1)
		rr, ok := pkts[0].(*rtcp.ReceiverReport)
		assert.True(t, ok)
		assert.Equal(t, 1, len(rr.Reports))
		assert.Equal(t, rtcp.ReceptionReport{
			SSRC:               uint32(123456),
			LastSequenceNumber: 0x02,
			LastSenderReport:   0,
			FractionLost:       0,
			TotalLost:          0,
			Delay:              0,
			Jitter:             30000 / 16,
		}, rr.Reports[0])
	})
}
