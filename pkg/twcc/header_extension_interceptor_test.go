package twcc

import (
	"sync"
	"testing"

	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestHeaderExtensionInterceptor(t *testing.T) {
	t.Run("add transport wide cc to each packet", func(t *testing.T) {
		hei, err := NewHeaderExtensionInterceptor(1)
		assert.NoError(t, err)

		pChan := make(chan *rtp.Packet, 10*5)
		go func() {
			// start some parallel streams using the same interceptor to test for race conditions
			var wg sync.WaitGroup
			num := 10
			wg.Add(num)
			for i := 0; i < num; i++ {
				go func(ch chan *rtp.Packet, id uint16) {
					rtpOut, rtpWriter := rtpio.RTPPipe()

					rtpIn := hei.Transform(rtpWriter, nil, nil)
					defer func() {
						wg.Done()
					}()

					for _, seqNum := range []uint16{id * 1, id * 2, id * 3, id * 4, id * 5} {
						go func() {
							_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}})
							assert.NoError(t, err2)
						}()
						p := &rtp.Packet{}
						_, err2 := rtpOut.ReadRTP(p)
						assert.NoError(t, err2)
						assert.Equal(t, seqNum, p.SequenceNumber)
						ch <- p
					}
				}(pChan, uint16(i+1))
			}
			wg.Wait()
			assert.NoError(t, hei.Close())
			close(pChan)
		}()

		for p := range pChan {
			// Can't check for increasing transport cc sequence number, since we can't ensure ordering between the streams
			// on pChan is same as in the interceptor, but at least make sure each packet has a seq nr.
			extensionHeader := p.GetExtension(1)
			twcc := &rtp.TransportCCExtension{}
			err = twcc.Unmarshal(extensionHeader)
			assert.NoError(t, err)
		}
	})
}
