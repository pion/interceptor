# dynamic_jitterbuffer
dynamic_jitterbuffer demonstrates a jitter buffer created with `WithDynamicLength` and `WithBandwidthEstimator`. A browser page sends RTP packets over an unreliable WebRTC data channel through a simulated network, and a Go client plays them out through the jitter buffer.

## Instructions
### run the example
```
cd examples/dynamic_jitterbuffer
go run .
```

Then open http://localhost:8080 and click **Connect**. Keep the tab visible, because browsers throttle timers in background tabs.

The `go.mod` in this directory replaces `github.com/pion/interceptor` with `../..`, so the example builds against your local copy of the jitter buffer.

### flags
- `-addr`: HTTP listen address (default `localhost:8080`)
- `-target-delay`: playout delay the buffer is sized to cover (default `100ms`)

### the page
Use the sliders to set bandwidth, latency, jitter and packet loss. You can also allow reordering and stall the network for one second. **Stop** closes the connection, and **Connect** starts a new session. Packets are 1000 bytes.

### the Go client
The Go client plays packets out in real time using their RTP timestamps. Every second it logs:
- the buffer state and how full the buffer is against its target length
- the bandwidth estimate
- packets received and played per second
- the jitter buffer's skipped, underflow, out-of-order and overflow counts

It also logs events: playback start, underflows, overflows and target length changes.

To read the log:
- `skipped` counts packets that weren't there when their turn came, because they were lost or arrived late. `late` is the part of those that arrived after being skipped. Late packets mean the playout delay is shorter than the network jitter. Raise `-target-delay` to cover it.
- `dropped N packets` means the client deliberately dropped backlog. It drops only when every packet played over the last 2 seconds waited longer than needed and none arrived late, as after the burst that follows a network stall. Without this, a stall would add its length to the latency for the rest of the session.

### Firefox
Without camera or microphone permission, Firefox offers only one network address for WebRTC. For a page loaded from `localhost` this can be a VPN tunnel address. Packets sent to that address go into the tunnel and never come back to this machine, so the connection fails. Chrome offers every interface and isn't affected.

To use Firefox, listen on your LAN address and open the page at that address:

```
go run . -addr 192.168.1.10:8080
```

Then open http://192.168.1.10:8080, replacing `192.168.1.10` with your machine's LAN IP. This makes the server reachable from other devices on your network.
