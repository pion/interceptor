package gcc

import (
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

type SendSideBandwidthEstimation struct {
	loss *lossBasedBandwidthEstimator
}

func (e *SendSideBandwidthEstimation) Write(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
	return 0, nil
}

func (e *SendSideBandwidthEstimation) Read([]byte, interceptor.Attributes) (int, interceptor.Attributes, error) {
	return 0, nil, nil
}
