package gcc

import (
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// mockTWCCResponder is a RTPWriter that writes
// TWCC feedback to a embedded SendSideBWE instantly
type mockTWCCResponder struct {
	bwe     *SendSideBWE
	rtpChan chan []byte
}

func (m *mockTWCCResponder) Read(out []byte, attributes interceptor.Attributes) (int, interceptor.Attributes, error) {
	pkt := <-m.rtpChan
	copy(out, pkt)
	return len(pkt), nil, nil
}

func (m *mockTWCCResponder) Write(pkts []rtcp.Packet, attributes interceptor.Attributes) (int, error) {
	return 0, m.bwe.WriteRTCP(pkts, attributes)
}

// mockGCCWriteStream receives RTP packets that have been paced by
// the congestion controller
type mockGCCWriteStream struct {
	twccResponder *mockTWCCResponder
}

func (m *mockGCCWriteStream) Write(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
	pkt, err := (&rtp.Packet{Header: *header, Payload: payload}).Marshal()
	if err != nil {
		panic(err)
	}

	m.twccResponder.rtpChan <- pkt
	return 0, err
}

func TestSendSideBWE(t *testing.T) {
	buffer := make([]byte, 1500)
	rtpPayload := make([]byte, 1460)
	streamInfo := &interceptor.StreamInfo{
		SSRC:                1,
		RTPHeaderExtensions: []interceptor.RTPHeaderExtension{{URI: transportCCURI, ID: 1}},
	}

	bwe, err := NewSendSideBWE()
	require.NoError(t, err)
	require.NotNil(t, bwe)

	m := &mockGCCWriteStream{
		&mockTWCCResponder{
			bwe,
			make(chan []byte, 500),
		},
	}

	twccSender, err := (&twcc.SenderInterceptorFactory{}).NewInterceptor("")
	require.NoError(t, err)
	require.NotNil(t, twccSender)

	twccInboundRTP := twccSender.BindRemoteStream(streamInfo, m.twccResponder)
	twccSender.BindRTCPWriter(m.twccResponder)

	require.Equal(t, latestBitrate, bwe.GetTargetBitrate())
	require.NotEqual(t, 0, len(bwe.GetStats()))

	rtpWriter := bwe.AddStream(streamInfo, m)
	require.NotNil(t, rtpWriter)

	twccWriter := twcc.HeaderExtensionInterceptor{}
	rtpWriter = twccWriter.BindLocalStream(streamInfo, rtpWriter)

	for i := 0; i <= 100; i++ {
		if _, err = rtpWriter.Write(&rtp.Header{SSRC: 1, Extensions: []rtp.Extension{}}, rtpPayload, nil); err != nil {
			panic(err)
		}
		if _, _, err = twccInboundRTP.Read(buffer, nil); err != nil {
			panic(err)
		}
	}

	// Sending a stream with zero loss and no RTT should increase estimate
	require.Less(t, latestBitrate, bwe.GetTargetBitrate())
}
