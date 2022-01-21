package rfc8888

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/muxable/rtptools/pkg/x_range"
	"github.com/muxable/rtptools/pkg/x_time"
	"github.com/pion/webrtc/v3"
)

const FormatCCFB = uint8(11)

type RFC8888MetricBlock struct {
	SequenceNumber uint16
	ECN            uint8 // actually only two bits
	ArrivalTime    time.Time
}

type RFC8888ReportBlock struct {
	ssrc         webrtc.SSRC
	MetricBlocks []*RFC8888MetricBlock
}

type RFC8888Report struct {
	Blocks    map[webrtc.SSRC]*RFC8888ReportBlock
	Timestamp time.Time
}

func (b *RFC8888ReportBlock) getSeqRange() (uint16, uint16) {
	seqs := make([]uint16, len(b.MetricBlocks))
	for i, metric := range b.MetricBlocks {
		seqs[i] = metric.SequenceNumber
	}
	return x_range.GetSeqRange(seqs)
}

func (r *RFC8888Report) Marshal(ts time.Time) []byte {
	size := uint16(8) // timestamp
	for _, block := range r.Blocks {
		// find the sequence with the largest distnace to block.beginSeq.
		beginSeq, endSeq := block.getSeqRange()
		length := endSeq - beginSeq + 1
		if length%2 == 1 {
			length++
		}
		size += 2*length + 8
	}
	buf := make([]byte, size)
	offset := uint16(0)
	for _, block := range r.Blocks {
		binary.BigEndian.PutUint32(buf[offset:(offset+4)], uint32(block.ssrc))
		beginSeq, endSeq := block.getSeqRange()
		binary.BigEndian.PutUint16(buf[(offset+4):(offset+6)], beginSeq)
		length := endSeq - beginSeq + 1
		if length%2 == 1 {
			length++
		}
		for i := 0; i < len(block.MetricBlocks); i++ {
			metric := block.MetricBlocks[i]
			index := offset + 8 + 2*(metric.SequenceNumber-beginSeq)
			buf[index] |= 0x80
			buf[index] |= metric.ECN << 5
			delta := ts.Sub(metric.ArrivalTime)
			if delta < 0 {
				buf[index] |= 0x1F
				buf[index+1] = 0xFF
				continue
			}
			ato := uint64(delta.Seconds() / 1024)
			if ato > 0x1FFD {
				buf[index] |= 0x1F
				buf[index+1] = 0xFE
			} else {
				buf[index] |= uint8(0x1F & (uint16(ato) >> 8))
				buf[index+1] = uint8(0xFF & uint16(ato))
			}
		}
		binary.BigEndian.PutUint16(buf[(offset+6):(offset+8)], length)
		offset += 2*length + 8
	}
	binary.BigEndian.PutUint64(buf[offset:(offset+8)], x_time.GoTimeToNTP(ts))
	return buf
}

func (r *RFC8888Report) Unmarshal(ts time.Time, buf []byte) error {
	if len(buf) < 8 {
		return errors.New("invalid packet")
	}
	r.Blocks = make(map[webrtc.SSRC]*RFC8888ReportBlock)
	rtpTs := binary.BigEndian.Uint64(buf[(len(buf) - 8):])
	r.Timestamp = x_time.NTPToGoTime(rtpTs)
	offset := uint16(0)
	for offset < uint16(len(buf)-8) {
		block := &RFC8888ReportBlock{}
		block.ssrc = webrtc.SSRC(binary.BigEndian.Uint32(buf[offset:(offset + 4)]))
		beginSeq := binary.BigEndian.Uint16(buf[(offset + 4):(offset + 6)])
		length := binary.BigEndian.Uint16(buf[(offset + 6):(offset + 8)])
		for i := uint16(0); i < length; i++ {
			received := (buf[offset+8+2*i] & 0x80) != 0
			if !received {
				continue
			}
			ato := time.Duration(uint64(binary.BigEndian.Uint16(buf[(offset+8+2*i):(offset+10+2*i)])&0x1FFF)*1024) * time.Second
			metric := &RFC8888MetricBlock{
				SequenceNumber: beginSeq + i,
				ECN:            (buf[offset+8+2*i] >> 5) & 0x03,
				ArrivalTime:    r.Timestamp.Add(-ato),
			}
			block.MetricBlocks = append(block.MetricBlocks, metric)
		}
		r.Blocks[block.ssrc] = block
		offset += length*2 + 8
	}
	return nil
}

type PacketStream struct {
	activeReport *RFC8888Report
}

func NewPacketStream() *PacketStream {
	return &PacketStream{activeReport: &RFC8888Report{
		Blocks: make(map[webrtc.SSRC]*RFC8888ReportBlock),
	}}
}

// AddPacket writes a packet to the underlying stream.
func (ps *PacketStream) AddPacket(ts time.Time, ssrc webrtc.SSRC, seq uint16, ecn uint8) error {
	block := ps.activeReport.Blocks[ssrc]
	if block == nil {
		block = &RFC8888ReportBlock{
			ssrc: ssrc,
		}
		ps.activeReport.Blocks[ssrc] = block
	}
	block.MetricBlocks = append(block.MetricBlocks, &RFC8888MetricBlock{
		SequenceNumber: seq,
		ECN:            ecn,
		ArrivalTime:    ts,
	})
	return nil
}

// BuildReport removes packets that are older than the window and returns the loss and marking rate.
func (ps *PacketStream) BuildReport(now time.Time) *RFC8888Report {
	report := ps.activeReport
	ps.activeReport = &RFC8888Report{
		Blocks: make(map[webrtc.SSRC]*RFC8888ReportBlock),
	}
	report.Timestamp = now
	return report
}
