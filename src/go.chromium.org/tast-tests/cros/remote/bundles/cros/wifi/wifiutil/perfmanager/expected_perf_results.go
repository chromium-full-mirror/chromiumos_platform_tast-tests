// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perfmanager

import (
	"strings"

	ap "go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	routerSupport "go.chromium.org/tast-tests/cros/remote/wificell/router/common/support"
	"go.chromium.org/tast/core/errors"
)

/*
This file defines the expected throughput values that should be used with the networkWiFiPerf.* tests.

The expected throughput values depend on the following parameters:
1- The router type:
	a) LegacyT
	b) OpenWrtT
2- The test type:
	a) TCP_TX
	b) TCP_RX
	c) TCP_BIDIRECTIONAL
	d) UDP_TX
	e) UDP_RX
	f) UDP_BIDIRECTIONAL
	Note: The thoughput is viewed from the DUT perspective:
		streaming to DUT = RX
		streaming from DUT = TX
		simultaneous TX + RX = BIDIERECTIONAL
2- The Connection mode:
	a) 80211n
	b) 80211ac
	c) 80211ax
    Note: gale(legacy) doesn't have expected values for 80211ax
3- The channel width:
	a) 20 MHz
	b) 40 MHz
	c) 80 MHz
*/

// ExpectedTput holds the must and should throughpt.
type ExpectedTput struct {
	Must   float64
	Should float64
}

// expectedThroughputWiFi map contains the must and should throughput based on the router type,
// test type, connection mode, and channel width.
var expectedThroughputWiFi = map[routerSupport.RouterType]map[TestType]map[ap.ModeEnum]map[ap.ChWidthEnum]ExpectedTput{
	routerSupport.LegacyT: {
		TestTypeTCPTx: {
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{61, 86},
				ap.ChWidth40Plus:  ExpectedTput{115, 166},
				ap.ChWidth40Minus: ExpectedTput{115, 166},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20:      ExpectedTput{74, 103},
				ap.ChWidth40:      ExpectedTput{153, 221},
				ap.ChWidth40Plus:  ExpectedTput{153, 221},
				ap.ChWidth40Minus: ExpectedTput{153, 221},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{200, 400},
			},
		},
		TestTypeTCPRx: {
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{61, 86},
				ap.ChWidth40Plus:  ExpectedTput{115, 166},
				ap.ChWidth40Minus: ExpectedTput{115, 166},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20:      ExpectedTput{74, 103},
				ap.ChWidth40:      ExpectedTput{153, 221},
				ap.ChWidth40Plus:  ExpectedTput{153, 221},
				ap.ChWidth40Minus: ExpectedTput{153, 221},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{200, 400},
			},
		},
		TestTypeTCPBidirectional: {
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{0, 0},
				ap.ChWidth40:      ExpectedTput{0, 0},
				ap.ChWidth40Plus:  ExpectedTput{0, 0},
				ap.ChWidth40Minus: ExpectedTput{0, 0},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20:      ExpectedTput{0, 0},
				ap.ChWidth40:      ExpectedTput{0, 0},
				ap.ChWidth40Plus:  ExpectedTput{0, 0},
				ap.ChWidth40Minus: ExpectedTput{0, 0},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{0, 0},
			},
		},
		TestTypeUDPTx: {
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{72, 101},
				ap.ChWidth40:      ExpectedTput{135, 195},
				ap.ChWidth40Plus:  ExpectedTput{135, 195},
				ap.ChWidth40Minus: ExpectedTput{135, 195},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20:      ExpectedTput{87, 121},
				ap.ChWidth40:      ExpectedTput{180, 260},
				ap.ChWidth40Plus:  ExpectedTput{180, 260},
				ap.ChWidth40Minus: ExpectedTput{180, 260},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{347, 500},
			},
		},
		TestTypeUDPRx: {
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{72, 101},
				ap.ChWidth40:      ExpectedTput{135, 195},
				ap.ChWidth40Plus:  ExpectedTput{135, 195},
				ap.ChWidth40Minus: ExpectedTput{135, 195},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20:      ExpectedTput{87, 121},
				ap.ChWidth40:      ExpectedTput{180, 260},
				ap.ChWidth40Plus:  ExpectedTput{180, 260},
				ap.ChWidth40Minus: ExpectedTput{180, 260},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{347, 500},
			},
		},
		TestTypeUDPBidirectional: {
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{0, 0},
				ap.ChWidth40:      ExpectedTput{0, 0},
				ap.ChWidth40Plus:  ExpectedTput{0, 0},
				ap.ChWidth40Minus: ExpectedTput{0, 0},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20:      ExpectedTput{0, 0},
				ap.ChWidth40:      ExpectedTput{0, 0},
				ap.ChWidth40Plus:  ExpectedTput{0, 0},
				ap.ChWidth40Minus: ExpectedTput{0, 0},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{0, 0},
			},
		},
	},
	routerSupport.OpenWrtT: {
		TestTypeTCPTx: {
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{61, 86},
				ap.ChWidth40:      ExpectedTput{115, 166},
				ap.ChWidth40Plus:  ExpectedTput{115, 166},
				ap.ChWidth40Minus: ExpectedTput{115, 166},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20: ExpectedTput{74, 103},
				ap.ChWidth40: ExpectedTput{153, 221},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{200, 400},
			},
			ap.Mode80211axPure: {
				ap.ChWidth20: ExpectedTput{74, 103},
				ap.ChWidth40: ExpectedTput{153, 221},
			},
			ap.Mode80211axMixed: {
				ap.ChWidth80: ExpectedTput{200, 400},
			},
		},
		TestTypeTCPRx: {
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{61, 86},
				ap.ChWidth40:      ExpectedTput{115, 166},
				ap.ChWidth40Plus:  ExpectedTput{115, 166},
				ap.ChWidth40Minus: ExpectedTput{115, 166},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20: ExpectedTput{74, 103},
				ap.ChWidth40: ExpectedTput{153, 221},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{200, 400},
			},
			ap.Mode80211axPure: {
				ap.ChWidth20: ExpectedTput{74, 103},
				ap.ChWidth40: ExpectedTput{153, 221},
			},
			ap.Mode80211axMixed: {
				ap.ChWidth80: ExpectedTput{200, 400},
			},
		},
		TestTypeTCPBidirectional: {
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{0, 0},
				ap.ChWidth40:      ExpectedTput{0, 0},
				ap.ChWidth40Plus:  ExpectedTput{0, 0},
				ap.ChWidth40Minus: ExpectedTput{0, 0},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20: ExpectedTput{0, 0},
				ap.ChWidth40: ExpectedTput{0, 0},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{0, 0},
			},
			ap.Mode80211axPure: {
				ap.ChWidth20: ExpectedTput{0, 0},
				ap.ChWidth40: ExpectedTput{0, 0},
			},
			ap.Mode80211axMixed: {
				ap.ChWidth80: ExpectedTput{0, 0},
			},
		},
		TestTypeUDPTx: {
			// TODO(b/271490937): wifi_perf_openwrt: UDP TX perf numbers are low.
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{72, 101},
				ap.ChWidth40:      ExpectedTput{135, 195},
				ap.ChWidth40Plus:  ExpectedTput{135, 195},
				ap.ChWidth40Minus: ExpectedTput{135, 195},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20: ExpectedTput{87, 121},
				ap.ChWidth40: ExpectedTput{180, 260},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{347, 500},
			},
			ap.Mode80211axPure: {
				ap.ChWidth20: ExpectedTput{87, 121},
				ap.ChWidth40: ExpectedTput{180, 260},
			},
			ap.Mode80211axMixed: {
				ap.ChWidth80: ExpectedTput{347, 500},
			},
		},
		TestTypeUDPRx: {
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{72, 101},
				ap.ChWidth40:      ExpectedTput{135, 195},
				ap.ChWidth40Plus:  ExpectedTput{135, 195},
				ap.ChWidth40Minus: ExpectedTput{135, 195},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20: ExpectedTput{87, 121},
				ap.ChWidth40: ExpectedTput{180, 260},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{347, 500},
			},
			ap.Mode80211axPure: {
				ap.ChWidth20: ExpectedTput{87, 121},
				ap.ChWidth40: ExpectedTput{180, 260},
			},
			ap.Mode80211axMixed: {
				ap.ChWidth80: ExpectedTput{347, 500},
			},
		},
		TestTypeUDPBidirectional: {
			ap.Mode80211nPure: {
				ap.ChWidth20:      ExpectedTput{0, 0},
				ap.ChWidth40:      ExpectedTput{0, 0},
				ap.ChWidth40Plus:  ExpectedTput{0, 0},
				ap.ChWidth40Minus: ExpectedTput{0, 0},
			},
			ap.Mode80211acPure: {
				ap.ChWidth20: ExpectedTput{0, 0},
				ap.ChWidth40: ExpectedTput{0, 0},
			},
			ap.Mode80211acMixed: {
				ap.ChWidth80: ExpectedTput{0, 0},
			},
			ap.Mode80211axPure: {
				ap.ChWidth20: ExpectedTput{0, 0},
				ap.ChWidth40: ExpectedTput{0, 0},
			},
			ap.Mode80211axMixed: {
				ap.ChWidth80: ExpectedTput{0, 0},
			},
		},
	},
}

// ExpectedThroughputWiFi return the expected must and should throughput.
func ExpectedThroughputWiFi(routerType routerSupport.RouterType, testType TestType, netMode ap.ModeEnum, netChanWidth ap.ChWidthEnum) (*ExpectedTput, error) {
	if tput, ok := expectedThroughputWiFi[routerType][testType][netMode][netChanWidth]; ok {
		return &tput, nil
	}
	return nil, errors.Errorf("failed to find expected throughput for router type =%s, test type=%s, network Mode =%s, channel width=%s", routerType.String(), testType, netMode, netChanWidth.String())
}

var maxThputExpectationForBoard = map[string]map[TestType]float64{
	// caroline throughput results tracked in b:188454947.
	"caroline": {
		TestTypeUDPRx: 200,
	},
	// elm throughput results tracked in b:201806809.
	"elm": {
		TestTypeUDPTx: 200,
		TestTypeUDPRx: 300,
		// The regression on TCP_RX is tracked in b:238853149
		TestTypeTCPRx: 125,
	},
	// eve throughput results tracked in b:188454947.
	"eve": {
		TestTypeUDPRx: 200,
	},
	// kukui throughput results tracked in b:201807413.
	"kukui": {
		TestTypeUDPRx: 300,
	},
	// nami throughput results tracked in b:188454947.
	"nami": {
		TestTypeUDPRx: 200,
	},
	// trogdor throughput results tracked in b:201807655.
	"trogdor": {
		TestTypeUDPRx: 250,
	},
}

// MaxExpectedThroughputForBoard returns the maximum throughput expectation for a given board in a given test type.
func MaxExpectedThroughputForBoard(boardName string, testType TestType) (float64, bool) {
	// Remove the suffix (-kernelnext, -connectivitynext, etc) from the board name.
	if strings.Contains(boardName, "-") {
		boardName = strings.Split(boardName, "-")[0]
	}
	if maxTput, ok := maxThputExpectationForBoard[boardName][testType]; ok {
		return maxTput, true
	}
	return 0, false
}
