package ccfb

// DuplicateAckFilter is a helper to remove duplicate acks from a Report.
type DuplicateAckFilter struct {
	highestAckedBySSRC map[uint32]int64
}

// NewDuplicateAckFilter creates a new DuplicateAckFilter.
func NewDuplicateAckFilter() *DuplicateAckFilter {
	return &DuplicateAckFilter{
		highestAckedBySSRC: make(map[uint32]int64),
	}
}

// Filter filters duplicate acks. It filters out all acks for packets with a
// sequence number smaller than the highest seen sequence number for each SSRC.
func (f *DuplicateAckFilter) Filter(reports Report) {
	for ssrc, prs := range reports.SSRCToPacketReports {
		n := 0
		for _, report := range prs {
			if highest, ok := f.highestAckedBySSRC[ssrc]; !ok || report.SeqNr > highest {
				f.highestAckedBySSRC[ssrc] = report.SeqNr
				prs[n] = report
				n++
			}
		}
		reports.SSRCToPacketReports[ssrc] = prs[:n]
	}
}
