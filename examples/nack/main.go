package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pion/interceptor/v2"
	"github.com/pion/interceptor/v2/pkg/nack"
	"github.com/pion/rtp"
)

const (
	listenPort = 6420
	mtu        = 1500
	ssrc       = 5000
)

func main() {
	go sendRoutine()
	receiveRoutine()
}

func receiveRoutine() {
	serverAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("127.0.0.1:%d", listenPort))
	if err != nil {
		panic(err)
	}

	conn, err := net.ListenUDP("udp4", serverAddr)
	if err != nil {
		panic(err)
	}

	// Create NACK Generator
	generator, err := nack.NewGeneratorInterceptor()
	if err != nil {
		panic(err)
	}

	rtpOut := interceptor.TransformReceiver(conn, generator, mtu)

	for {
		p := &rtp.Packet{}
		_, err := rtpOut.ReadRTP(p)
		if err != nil {
			panic(err)
		}

		log.Println("Received RTP")
		log.Println(p)
	}
}

func sendRoutine() {
	// Dial our UDP listener that we create in receiveRoutine
	serverAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("127.0.0.1:%d", listenPort))
	if err != nil {
		panic(err)
	}

	conn, err := net.DialUDP("udp4", nil, serverAddr)
	if err != nil {
		panic(err)
	}

	// Create NACK Responder
	responder, err := nack.NewResponderInterceptor()
	if err != nil {
		panic(err)
	}

	rtpIn := interceptor.TransformSender(conn, responder, mtu)

	for sequenceNumber := uint16(0); ; sequenceNumber++ {
		// Send a RTP packet with a Payload of 0x0, 0x1, 0x2
		if _, err := rtpIn.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				SSRC:           ssrc,
				SequenceNumber: sequenceNumber,
			},
			Payload: []byte{0x0, 0x1, 0x2},
		}); err != nil {
			fmt.Println(err)
		}

		time.Sleep(time.Millisecond * 200)
	}
}
