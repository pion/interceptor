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

func TestSenderInterceptor(t *testing.T) {
	t.Run("after RTP packets", func(t *testing.T) {
		mt := &test.MockTime{}
		md := feature.NewMediaDescriptionReceiver()
		assert.NoError(t, md.WriteMediaDescription(&sdp.MediaDescription{
			Attributes: []sdp.Attribute{
				{Key: "ssrc", Value: "123456"},
				{Key: "rtpmap", Value: "96 VP8/90000"},
			},
		}))
		i, err := NewSenderInterceptor(
			md,
			SenderInterval(time.Millisecond*50),
			SenderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			SenderNow(mt.Now),
		)
		assert.NoError(t, err)

		defer func() {
			assert.NoError(t, i.Close())
		}()

		rtcpOut, rtcpWriter := rtpio.RTCPPipe()
		rtpOut, rtpWriter := rtpio.RTPPipe()

		rtpIn := i.Transform(rtpWriter, rtcpWriter, nil)

		for i := 0; i < 10; i++ {
			go func() {
				_, err2 := rtpIn.WriteRTP(&rtp.Packet{
					Header:  rtp.Header{SequenceNumber: uint16(i), SSRC: 123456, PayloadType: 96},
					Payload: []byte("\x00\x00"),
				})
				assert.NoError(t, err2)
			}()

			p := &rtp.Packet{}
			_, err2 := rtpOut.ReadRTP(p)
			assert.NoError(t, err2)
			assert.Equal(t, uint16(i), p.Header.SequenceNumber)
		}

		mt.SetNow(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
		pkts := make([]rtcp.Packet, 16)
		n, err := rtcpOut.ReadRTCP(pkts)
		assert.NoError(t, err)
		assert.Equal(t, n, 1)
		sr, ok := pkts[0].(*rtcp.SenderReport)
		assert.True(t, ok)
		assert.Equal(t, &rtcp.SenderReport{
			SSRC:        123456,
			NTPTime:     ntpTime(mt.Now()),
			RTPTime:     2269117121,
			PacketCount: 10,
			OctetCount:  20,
		}, sr)
	})
}
