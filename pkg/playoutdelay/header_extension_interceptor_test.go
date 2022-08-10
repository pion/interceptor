package playoutdelay

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestHeaderExtensionInterceptor(t *testing.T) {
	t.Run("add playout delay to each packet", func(t *testing.T) {
		factory, err := NewHeaderExtensionInterceptor()
		assert.NoError(t, err)

		inter, err := factory.NewInterceptor("", 10*time.Millisecond, 20*time.Millisecond)
		assert.NoError(t, err)

		pChan := make(chan *rtp.Packet, 10*5)
		go func() {
			// start some parallel streams using the same interceptor to test for race conditions
			var wg sync.WaitGroup
			num := 10
			wg.Add(num)
			for i := 0; i < num; i++ {
				go func(ch chan *rtp.Packet, id uint16) {
					stream := test.NewMockStream(&interceptor.StreamInfo{RTPHeaderExtensions: []interceptor.RTPHeaderExtension{
						{
							URI: playoutDelayURI,
							ID:  1,
						},
					}}, inter)
					defer func() {
						wg.Done()
						assert.NoError(t, stream.Close())
					}()

					for _, seqNum := range []uint16{id * 1, id * 2, id * 3, id * 4, id * 5} {
						assert.NoError(t, stream.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}}))
						select {
						case p := <-stream.WrittenRTP():
							assert.Equal(t, seqNum, p.SequenceNumber)
							ch <- p
						case <-time.After(10 * time.Millisecond):
							panic("written rtp packet not found")
						}
					}
				}(pChan, uint16(i+1))
			}
			wg.Wait()
			close(pChan)
		}()

		for p := range pChan {
			extensionHeader := p.GetExtension(1)
			ext := &rtp.PlayoutDelayExtension{}
			err = ext.Unmarshal(extensionHeader)
			assert.NoError(t, err)
			assert.Equal(t, uint16(1), ext.minDelay)
			assert.Equal(t, uint16(2), ext.maxDelay)
		}
	})
}
