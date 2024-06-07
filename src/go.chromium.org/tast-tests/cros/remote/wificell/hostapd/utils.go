// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hostapd

import "go.chromium.org/tast/core/errors"

// freqToChannelMap maps a frequency (MHz) to channel number in the 2.4/5GHz band.
var freqToChannelMap = map[int]int{
	2412: 1,
	2417: 2,
	2422: 3,
	2427: 4,
	2432: 5,
	2437: 6,
	2442: 7,
	2447: 8,
	2452: 9,
	2457: 10,
	2462: 11,
	// 12, 13 are only legitimate outside the US.
	2467: 12,
	2472: 13,
	// 14 is for Japan, DSSS and CCK only.
	2484: 14,
	// 32 valid in Europe.
	5160: 32,
	// 34 valid in Europe.
	5170: 34,
	// 36-116 valid in the US, except 38, 42, and 46, which have
	// mixed international support.
	5180: 36,
	5190: 38,
	5200: 40,
	5210: 42,
	5220: 44,
	5230: 46,
	5240: 48,
	5260: 52,
	5280: 56,
	5300: 60,
	5320: 64,
	5500: 100,
	5520: 104,
	5540: 108,
	5560: 112,
	5580: 116,
	// 120, 124, 128 valid in Europe/Japan.
	5600: 120,
	5620: 124,
	5640: 128,
	// 132+ valid in US.
	5660: 132,
	5680: 136,
	5700: 140,
	5710: 142,
	// 144 is supported by a subset of WiFi chips
	// (e.g. bcm4354, but not ath9k).
	5720: 144,
	5745: 149,
	5755: 151,
	5765: 153,
	5785: 157,
	5805: 161,
	5825: 165,
}

// OpClass6GHzEnum is the type for specifying the operating class in
// hostapd config on 6GHz.
type OpClass6GHzEnum int

const (
	opClass20MHz       OpClass6GHzEnum = 131
	opClass40MHz       OpClass6GHzEnum = 132
	opClass80MHz       OpClass6GHzEnum = 133
	opClass160MHz      OpClass6GHzEnum = 134
	opClass80Plus80MHz OpClass6GHzEnum = 135
	opClassCh2         OpClass6GHzEnum = 136
	opClass320MHz      OpClass6GHzEnum = 137
)

// Excludes channel 2 because its operating class is determined by channel
// number rather than channel width.
var chWidthToOpClass = map[ChWidthEnum]OpClass6GHzEnum{
	ChWidth20:       opClass20MHz,
	ChWidth40:       opClass40MHz,
	ChWidth80:       opClass80MHz,
	ChWidth160:      opClass160MHz,
	ChWidth80Plus80: opClass80Plus80MHz,
	ChWidth320:      opClass320MHz,
}

const (
	base6GHzFreq   int = 5950
	channel2       int = 2
	min6GHzFreq    int = 5935
	max6GHzFreq    int = 7115
	min6GHzChannel int = 1
	max6GHzChannel int = 233

	// For verifying that operating class matches the channel number.
	first6GHz20MHzChannel  int = 1
	first6GHz40MHzChannel  int = 3
	first6GHz80MHzChannel  int = 7
	first6GHz160MHzChannel int = 15
)

// OpClass6GHz maps channel width to operating class in the 6GHz band. Except
// for channel 2 which has a unique operating class.
func OpClass6GHz(channelWidth ChWidthEnum, channel int) (OpClass6GHzEnum, error) {
	if channel == 2 {
		return opClassCh2, nil
	}
	for cw, opClass := range chWidthToOpClass {
		if cw == channelWidth {
			return opClass, nil
		}
	}
	return 0, errors.Errorf("cannnot determine the op class for the given channel width=%s and channel=%d", channelWidth.String(), channel)
}

// FrequencyToChannel maps center frequency (in MHz) to the corresponding channel.
func FrequencyToChannel(freq int) (int, error) {
	var ch int
	var ok bool
	is6GHz := freq >= min6GHzFreq
	if !is6GHz {
		ch, ok = freqToChannelMap[freq]
		if !ok {
			return 0, errors.Errorf("cannot find channel with frequency=%d", freq)
		}
	} else if freq == min6GHzFreq {
		ch = channel2
	} else if (freq-min6GHzFreq)%10 != 0 || freq > max6GHzFreq || freq == 5945 {
		return 0, errors.New("invalid 6GHz frequency")
	} else {
		ch = (freq - base6GHzFreq) / 5
	}
	return ch, nil
}

// Validate6GHzOpClass checks that the correct operating class is used for a
// given center channel in the 6GHz band.
func Validate6GHzOpClass(ch int, opClass OpClass6GHzEnum) error {
	if ch < min6GHzChannel || ch > max6GHzChannel {
		return errors.New("channel is out of range")
	}
	// In 6GHz band, all channels of the same channel width (except channel 2)
	// are a constant multiple of two from each other.
	if opClass == opClass20MHz && (ch-first6GHz20MHzChannel)%4 != 0 {
		return errors.Errorf("channel %d does not match operating class which expects a 20MHz channel", ch)
	} else if opClass == opClass40MHz && (ch-first6GHz40MHzChannel)%8 != 0 {
		return errors.Errorf("channel %d does not match operating class which expects a 40MHZ channel", ch)
	} else if opClass == opClass80MHz && (ch-first6GHz80MHzChannel)%16 != 0 {
		return errors.Errorf("channel %d does not match operating class which expects an 80MHZ channel", ch)
	} else if opClass == opClass160MHz && (ch-first6GHz160MHzChannel)%32 != 0 {
		return errors.Errorf("channel %d does not match operating class which expects a 160 MHZ channel", ch)
	} else if opClass == opClass80Plus80MHz {
		// Non-contiguous channel widths are not yet supported by our routers.
		return errors.New("operating class corresponds to unsupported 80+80 channel width")
	} else if opClass == opClassCh2 && ch != 2 {
		return errors.Errorf("channel %d does not match operating class 136", ch)
	} else if opClass == opClass320MHz {
		// TODO(b/314396114) Add 320 MHz channel width as an option for WiFi7 tests
		return errors.New("operating class corresponds to unsupported 320MHz channel width for WiFi 6E")
	}
	return nil
}

// ChannelToFrequencyWithBand maps channel id to its center frequency (in MHz).
func ChannelToFrequencyWithBand(target int, is6GHz bool) (int, error) {
	if is6GHz {
		if target < min6GHzChannel || target > max6GHzChannel {
			return 0, errors.New("channel is out of range")
		}
		if target == channel2 {
			return min6GHzFreq, nil
		}
		return base6GHzFreq + target*5, nil
	}

	for f, ch := range freqToChannelMap {
		if ch == target {
			return f, nil
		}
	}
	return 0, errors.Errorf("cannnot find channel num=%d", target)
}

// ChannelToFrequency is kept for backwards compatibility with cases where
// the operating class is not specified.
func ChannelToFrequency(ch int) (int, error) {
	return ChannelToFrequencyWithBand(ch, false)
}
