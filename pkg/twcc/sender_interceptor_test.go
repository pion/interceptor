package twcc

import (
	"fmt"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestSenderInterceptor(t *testing.T) {
	t.Run("before any packets", func(t *testing.T) {
		f, err := NewSenderInterceptor()
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{SSRC: 1, RTPHeaderExtensions: []interceptor.RTPHeaderExtension{
			{
				URI: transportCCURI,
				ID:  1,
			},
		}}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, 1, len(pkts))
		tlcc, ok := pkts[0].(*rtcp.TransportLayerCC)
		assert.True(t, ok)
		assert.Equal(t, uint16(0), tlcc.PacketStatusCount)
		assert.Equal(t, uint8(0), tlcc.FbPktCount)
		assert.Equal(t, uint16(0), tlcc.BaseSequenceNumber)
		assert.Equal(t, uint32(0), tlcc.MediaSSRC)
		assert.Equal(t, uint32(0), tlcc.ReferenceTime)
		assert.Equal(t, 0, len(tlcc.RecvDeltas))
		assert.Equal(t, 0, len(tlcc.PacketChunks))
	})

	t.Run("after RTP packets", func(t *testing.T) {
		f, err := NewSenderInterceptor()
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{SSRC: 1, RTPHeaderExtensions: []interceptor.RTPHeaderExtension{
			{
				URI: transportCCURI,
				ID:  1,
			},
		}}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		for i := 0; i < 10; i++ {
			hdr := rtp.Header{}
			tcc, err := (&rtp.TransportCCExtension{TransportSequence: uint16(i)}).Marshal()
			assert.NoError(t, err)
			err = hdr.SetExtension(1, tcc)
			assert.NoError(t, err)
			stream.ReceiveRTP(&rtp.Packet{Header: hdr})
		}

		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, 1, len(pkts))
		cc, ok := pkts[0].(*rtcp.TransportLayerCC)
		assert.True(t, ok)
		assert.Equal(t, uint32(1), cc.MediaSSRC)
		assert.Equal(t, uint16(0), cc.BaseSequenceNumber)
		assert.Equal(t, []rtcp.PacketStatusChunk{
			&rtcp.RunLengthChunk{
				PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
				RunLength:          10,
			},
		}, cc.PacketChunks)
	})

	t.Run("different delays between RTP packets", func(t *testing.T) {
		f, err := NewSenderInterceptor(SendInterval(500 * time.Millisecond))
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{RTPHeaderExtensions: []interceptor.RTPHeaderExtension{
			{
				URI: transportCCURI,
				ID:  1,
			},
		}}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		delays := []int{0, 10, 100, 200}
		for i, d := range delays {
			time.Sleep(time.Duration(d) * time.Millisecond)

			hdr := rtp.Header{}
			tcc, err := (&rtp.TransportCCExtension{TransportSequence: uint16(i)}).Marshal()
			assert.NoError(t, err)
			err = hdr.SetExtension(1, tcc)
			assert.NoError(t, err)
			stream.ReceiveRTP(&rtp.Packet{Header: hdr})
		}

		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, 1, len(pkts))
		cc, ok := pkts[0].(*rtcp.TransportLayerCC)
		assert.True(t, ok)
		assert.Equal(t, uint16(0), cc.BaseSequenceNumber)
		assert.Equal(t, []rtcp.PacketStatusChunk{
			&rtcp.StatusVectorChunk{
				SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
				SymbolList: []uint16{
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketReceivedLargeDelta,
					rtcp.TypeTCCPacketReceivedLargeDelta,
				},
			},
		}, cc.PacketChunks)
	})

	t.Run("packet loss", func(t *testing.T) {
		f, err := NewSenderInterceptor(SendInterval(2 * time.Second))
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{RTPHeaderExtensions: []interceptor.RTPHeaderExtension{
			{
				URI: transportCCURI,
				ID:  1,
			},
		}}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		sequenceNumberToDelay := map[int]int{
			0:  0,
			1:  10,
			4:  100,
			8:  200,
			9:  20,
			10: 20,
			30: 300,
		}
		for _, i := range []int{0, 1, 4, 8, 9, 10, 30} {
			d := sequenceNumberToDelay[i]
			time.Sleep(time.Duration(d) * time.Millisecond)

			hdr := rtp.Header{}
			tcc, err := (&rtp.TransportCCExtension{TransportSequence: uint16(i)}).Marshal()
			assert.NoError(t, err)
			err = hdr.SetExtension(1, tcc)
			assert.NoError(t, err)
			stream.ReceiveRTP(&rtp.Packet{Header: hdr})
		}

		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, 1, len(pkts))
		cc, ok := pkts[0].(*rtcp.TransportLayerCC)
		assert.True(t, ok)
		assert.Equal(t, uint16(0), cc.BaseSequenceNumber)
		assert.Equal(t, []rtcp.PacketStatusChunk{
			&rtcp.StatusVectorChunk{
				SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
				SymbolList: []uint16{
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketReceivedLargeDelta,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
				},
			},
			&rtcp.StatusVectorChunk{
				SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
				SymbolList: []uint16{
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketReceivedLargeDelta,
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
				},
			},
			&rtcp.RunLengthChunk{
				PacketStatusSymbol: rtcp.TypeTCCPacketNotReceived,
				RunLength:          16,
			},
			&rtcp.RunLengthChunk{
				PacketStatusSymbol: rtcp.TypeTCCPacketReceivedLargeDelta,
				RunLength:          1,
			},
		}, cc.PacketChunks)
	})

	t.Run("overflow", func(t *testing.T) {
		f, err := NewSenderInterceptor(SendInterval(2 * time.Second))
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{RTPHeaderExtensions: []interceptor.RTPHeaderExtension{
			{
				URI: transportCCURI,
				ID:  1,
			},
		}}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		for _, i := range []int{65530, 65534, 65535, 1, 2, 10} {
			hdr := rtp.Header{}
			tcc, err := (&rtp.TransportCCExtension{TransportSequence: uint16(i)}).Marshal()
			assert.NoError(t, err)
			err = hdr.SetExtension(1, tcc)
			assert.NoError(t, err)
			stream.ReceiveRTP(&rtp.Packet{Header: hdr})
		}

		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, 1, len(pkts))
		cc, ok := pkts[0].(*rtcp.TransportLayerCC)
		assert.True(t, ok)
		assert.Equal(t, uint16(65530), cc.BaseSequenceNumber)
		assert.Equal(t, []rtcp.PacketStatusChunk{
			&rtcp.StatusVectorChunk{
				SymbolSize: rtcp.TypeTCCSymbolSizeOneBit,
				SymbolList: []uint16{
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
				},
			},
			&rtcp.StatusVectorChunk{
				SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
				SymbolList: []uint16{
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketReceivedSmallDelta,
				},
			},
		}, cc.PacketChunks)
	})

	t.Run("THING", func(t *testing.T) {
		f, err := NewSenderInterceptor(SendInterval(2 * time.Second))
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{RTPHeaderExtensions: []interceptor.RTPHeaderExtension{
			{
				URI: transportCCURI,
				ID:  1,
			},
		}}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		type packet struct {
			nr uint16
			ts time.Time
		}
		pkts := []packet{
			{nr: 1, ts: mustParse("13:21:32.66605293")},
			{nr: 3, ts: mustParse("13:21:32.68289038")},
			{nr: 2, ts: mustParse("13:21:32.6829856")},
			{nr: 4, ts: mustParse("13:21:32.68331201")},
			{nr: 5, ts: mustParse("13:21:32.69534352")},
			{nr: 6, ts: mustParse("13:21:32.69545351")},
			{nr: 7, ts: mustParse("13:21:32.70660835")},
			{nr: 9, ts: mustParse("13:21:32.71778934")},
			{nr: 10, ts: mustParse("13:21:32.73091492")},
		}
		go func() {
			for _, pkt := range pkts {
				hdr := rtp.Header{}
				tcc, err := (&rtp.TransportCCExtension{TransportSequence: pkt.nr}).Marshal()
				assert.NoError(t, err)
				err = hdr.SetExtension(1, tcc)
				assert.NoError(t, err)
				stream.ReceiveRTP(&rtp.Packet{Header: hdr})
			}
		}()

		rtcpPkts := <-stream.WrittenRTCP()
		assert.Equal(t, 1, len(rtcpPkts))
		cc := rtcpPkts[0].(*rtcp.TransportLayerCC)
		fmt.Println(cc)

		pkts = []packet{
			{nr: 13, ts: mustParse("13:21:32.7400967")},
			{nr: 14, ts: mustParse("13:21:32.74720876")},
			{nr: 15, ts: mustParse("13:21:32.75139925")},
			{nr: 18, ts: mustParse("13:21:32.75755856")},
			{nr: 19, ts: mustParse("13:21:32.76365941")},
			{nr: 20, ts: mustParse("13:21:32.76374313")},
			{nr: 21, ts: mustParse("13:21:32.76584219")},
			{nr: 22, ts: mustParse("13:21:32.76797761")},
			{nr: 23, ts: mustParse("13:21:32.77013109")},
			{nr: 24, ts: mustParse("13:21:32.77332659")},
			{nr: 25, ts: mustParse("13:21:32.7754075")},
			{nr: 26, ts: mustParse("13:21:32.77761419")},
			{nr: 27, ts: mustParse("13:21:32.78074493")},
			{nr: 28, ts: mustParse("13:21:32.78080871")},
			{nr: 29, ts: mustParse("13:21:32.78295785")},
			{nr: 30, ts: mustParse("13:21:32.78304688")},
			{nr: 32, ts: mustParse("13:21:32.78554982")},
			{nr: 33, ts: mustParse("13:21:32.78741282")},
			{nr: 34, ts: mustParse("13:21:32.78750512")},
			{nr: 35, ts: mustParse("13:21:32.79050869")},
			{nr: 36, ts: mustParse("13:21:32.79059667")},
			{nr: 37, ts: mustParse("13:21:32.79265193")},
			{nr: 38, ts: mustParse("13:21:32.79273576")},
			{nr: 39, ts: mustParse("13:21:32.79472212")},
			{nr: 40, ts: mustParse("13:21:32.79482135")},
		}

		go func() {
			for _, pkt := range pkts {
				hdr := rtp.Header{}
				tcc, err := (&rtp.TransportCCExtension{TransportSequence: pkt.nr}).Marshal()
				assert.NoError(t, err)
				err = hdr.SetExtension(1, tcc)
				assert.NoError(t, err)
				stream.ReceiveRTP(&rtp.Packet{Header: hdr})
			}
		}()

		rtcpPkts = <-stream.WrittenRTCP()
		assert.Equal(t, 1, len(rtcpPkts))
		cc = rtcpPkts[0].(*rtcp.TransportLayerCC)
		fmt.Println(cc)

		pkts = []packet{
			{nr: 46, ts: mustParse("13:21:32.87171882")},
			{nr: 46, ts: mustParse("13:21:32.87181345")},
			{nr: 47, ts: mustParse("13:21:32.87379482")},
			{nr: 48, ts: mustParse("13:21:32.87385716")},
			{nr: 49, ts: mustParse("13:21:32.87696351")},
			{nr: 50, ts: mustParse("13:21:32.87703434")},
			{nr: 51, ts: mustParse("13:21:32.88018133")},
			{nr: 52, ts: mustParse("13:21:32.88028212")},
			{nr: 54, ts: mustParse("13:21:32.8803838")},
			{nr: 54, ts: mustParse("13:21:32.88042859")},
			{nr: 56, ts: mustParse("13:21:32.88232208")},
			{nr: 56, ts: mustParse("13:21:32.88244116")},
			{nr: 58, ts: mustParse("13:21:32.88259426")},
			{nr: 58, ts: mustParse("13:21:32.88265957")},
			{nr: 60, ts: mustParse("13:21:32.88559268")},
			{nr: 60, ts: mustParse("13:21:32.88569149")},
			{nr: 61, ts: mustParse("13:21:32.88576656")},
			{nr: 62, ts: mustParse("13:21:32.88585761")},
			{nr: 65, ts: mustParse("13:21:32.91189473")},
		}

		go func() {
			for _, pkt := range pkts {
				hdr := rtp.Header{}
				tcc, err := (&rtp.TransportCCExtension{TransportSequence: pkt.nr}).Marshal()
				assert.NoError(t, err)
				err = hdr.SetExtension(1, tcc)
				assert.NoError(t, err)
				stream.ReceiveRTP(&rtp.Packet{Header: hdr})
			}
		}()

		rtcpPkts = <-stream.WrittenRTCP()
		assert.Equal(t, 1, len(rtcpPkts))
		cc = rtcpPkts[0].(*rtcp.TransportLayerCC)
		fmt.Println(cc)

		pkts = []packet{
			{nr: 68, ts: mustParse("13:21:32.94151858")},
			{nr: 69, ts: mustParse("13:21:32.94162367")},
			{nr: 70, ts: mustParse("13:21:32.94367602")},
			{nr: 71, ts: mustParse("13:21:32.94376743")},
			{nr: 72, ts: mustParse("13:21:32.94583518")},
			{nr: 73, ts: mustParse("13:21:32.94593532")},
			{nr: 75, ts: mustParse("13:21:32.94603652")},
			{nr: 75, ts: mustParse("13:21:32.94610846")},
			{nr: 76, ts: mustParse("13:21:32.94620313")},
			{nr: 77, ts: mustParse("13:21:32.94803155")},
			{nr: 78, ts: mustParse("13:21:32.94812239")},
			{nr: 80, ts: mustParse("13:21:32.94826421")},
			{nr: 80, ts: mustParse("13:21:32.94833984")},
			{nr: 81, ts: mustParse("13:21:32.94848815")},
			{nr: 82, ts: mustParse("13:21:32.95025247")},
			{nr: 83, ts: mustParse("13:21:32.95035025")},
			{nr: 85, ts: mustParse("13:21:32.95048062")},
			{nr: 85, ts: mustParse("13:21:32.95052717")},
			{nr: 86, ts: mustParse("13:21:32.95059531")},
			{nr: 87, ts: mustParse("13:21:32.95266087")},
			{nr: 88, ts: mustParse("13:21:32.95275829")},
			{nr: 89, ts: mustParse("13:21:32.95285479")},
			{nr: 90, ts: mustParse("13:21:32.95295125")},
			{nr: 91, ts: mustParse("13:21:32.95306884")},
			{nr: 92, ts: mustParse("13:21:32.9547105")},
			{nr: 93, ts: mustParse("13:21:32.95483123")},
			{nr: 94, ts: mustParse("13:21:32.95492333")},
			{nr: 96, ts: mustParse("13:21:32.95504746")},
			{nr: 96, ts: mustParse("13:21:32.95513238")},
			{nr: 97, ts: mustParse("13:21:32.95999092")},
			{nr: 100, ts: mustParse("13:21:33.00914531")},
			{nr: 101, ts: mustParse("13:21:33.01114703")},
		}

		go func() {
			for _, pkt := range pkts {
				hdr := rtp.Header{}
				tcc, err := (&rtp.TransportCCExtension{TransportSequence: pkt.nr}).Marshal()
				assert.NoError(t, err)
				err = hdr.SetExtension(1, tcc)
				assert.NoError(t, err)
				stream.ReceiveRTP(&rtp.Packet{Header: hdr})
			}
		}()

		rtcpPkts = <-stream.WrittenRTCP()
		assert.Equal(t, 1, len(rtcpPkts))
		cc = rtcpPkts[0].(*rtcp.TransportLayerCC)
		fmt.Println(cc)
	})
}
