// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec

// FecOption can be used to set initial options on Fec encoder interceptors.
type FecOption func(d *FecInterceptor) error

// NumMediaPackets sets the number of media packets to accumulate before generating another FEC packets batch.
func NumMediaPackets(numMediaPackets uint32) FecOption {
	return func(f *FecInterceptor) error {
		f.numMediaPackets = numMediaPackets

		return nil
	}
}

// NumFECPackets sets the number of FEC packets to generate for each batch of media packets.
func NumFECPackets(numFecPackets uint32) FecOption {
	return func(f *FecInterceptor) error {
		f.numFecPackets = numFecPackets

		return nil
	}
}

// FECEncoderFactory sets the custom factory for constructing the FEC Encoders.
func FECEncoderFactory(factory EncoderFactory) FecOption {
	return func(f *FecInterceptor) error {
		f.encoderFactory = factory

		return nil
	}
}

// MultiFrame switches the interceptor from fixed-size packet batches to frame-aware
// protection groups that may span up to maxFrames video frames (λ0 in Holmer et al.,
// "Handling packet loss in WebRTC", ICIP 2013, section 3.2).
//
// A group is closed at a frame boundary once either the number of frames in the group
// reaches the current limit λ, or the overhead actually produced by protecting the group
// is no more than MaxExcessOverhead above the target ProtectionLevel. This lets low-bitrate
// streams, which have few packets per frame, hit the target overhead instead of
// overshooting it, at the cost of extra FEC recovery delay.
//
// Unless DisableRTTAdaptation is set, λ follows max(1, min(f·RTT, maxFrames)), with the frame
// rate f estimated from RTP timestamps and the RTT from incoming RTCP receiver reports. Until
// both are known, maxFrames is used.
func MultiFrame(maxFrames uint32) FecOption {
	return func(f *FecInterceptor) error {
		if maxFrames == 0 {
			return errInvalidMaxFrames
		}
		f.multiFrame = true
		f.maxFrames = maxFrames

		return nil
	}
}

// ProtectionLevel sets the target FEC overhead PL = m/(k+m) used in MultiFrame mode, where k is
// the number of media packets and m the number of FEC packets in a protection group. It must be
// in (0, 1). If unset, it is derived from NumMediaPackets and NumFECPackets.
//
// At most one FEC packet is sent per media packet, so the overhead never exceeds 0.5. With a
// level above 0.5, every frame is protected as its own group.
func ProtectionLevel(level float64) FecOption {
	return func(f *FecInterceptor) error {
		if !(level > 0 && level < 1) {
			return errInvalidProtectionLevel
		}
		f.protectionLevel = level

		return nil
	}
}

// MaxExcessOverhead sets how far above the target ProtectionLevel the actual overhead of a
// protection group may be before the group is closed early in MultiFrame mode. It must be
// non-negative. Default is 0.2 (roughly libwebrtc's kMaxExcessOverhead of 50/256).
func MaxExcessOverhead(excess float64) FecOption {
	return func(f *FecInterceptor) error {
		if !(excess >= 0) {
			return errInvalidMaxExcessOverhead
		}
		f.maxExcessOverhead = excess

		return nil
	}
}

// DisableRTTAdaptation makes MultiFrame mode always allow up to maxFrames per group, instead
// of scaling the limit with the measured RTT and frame rate.
func DisableRTTAdaptation() FecOption {
	return func(f *FecInterceptor) error {
		f.disableRTTAdaptation = true

		return nil
	}
}
