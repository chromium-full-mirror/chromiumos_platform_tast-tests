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
	opClass20MHz           int = 131
	opClass40MHz           int = 132
	opClass80MHz           int = 133
	opClass160MHz          int = 134
	opClass80Plus80MHz     int = 135
	opClassCh2             int = 136
	opClass320MHz          int = 137
)

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

// Is6GHzOpClass returns whether a given operating class is in the 6GHz band.
func Is6GHzOpClass(opClass int) bool {
	return opClass >= 131 && opClass <= 137
}

// Validate6GHzOpClass checks that the correct operating class is used for a
// given channel in the 6GHz band.
func Validate6GHzOpClass(ch, opClass int) error {
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

// ChannelToFrequencyWithOpClass maps channel id and operating class to its
// center frequency (in MHz). For the 2.4/5GHz band, the operating class will be
// ignored. If the operating class is in the range [131, 137], the channel will
// be mapped to a frequency in the 6GHz band.
func ChannelToFrequencyWithOpClass(target, opClass int) (int, error) {
	// If the operating class is specified and is in the range [131, 137], the
	// channel id is mapped to a frequency in the 6GHz band.
	if Is6GHzOpClass(opClass) {
		if err := Validate6GHzOpClass(target, opClass); err != nil {
			return 0, err
		}
		if opClass == opClassCh2 && target == channel2 {
			return min6GHzFreq, nil
		}
		f := base6GHzFreq + target*5
		return f, nil
	}

	// If the operating class is not specified or out of the range
	// [131, 137], map to a frequency in the 2.4GHz/5GHz band.
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
	return ChannelToFrequencyWithOpClass(ch, 0)
}
