package x_range

func GetSeqRange(seqs []uint16) (uint16, uint16) {
	minDelta := 0
	maxDelta := 0
	seq0 := seqs[0]
	for _, seq := range seqs {
		delta := int(seq-seq0)
		if seq-seq0 >= 16384 {
			delta -= (1<<16)
			if delta < minDelta {
				minDelta = delta
			}
		} else {
			if delta > maxDelta {
				maxDelta = delta
			}
		}
	}
	return seq0 + uint16(minDelta), seq0 + uint16(maxDelta)
}
