// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cellular provides functions for testing Cellular connectivity.
package cellular

import (
	"strings"

	"chromiumos/tast/common/shillconst"
	"chromiumos/tast/errors"
)

// KnownAPN is an APN known to a carrier.
// |Optional| indicates that any of the optional APNs might work, but not all.
// At least one of the |Optional| APNs has to work
type KnownAPN struct {
	Optional bool
	APNInfo  map[string]string
}

type carrier int

const (
	carrierAmarisoft carrier = iota
	carrierVerizon
	carrierTmobile
	carrierAtt
	carrierSoftbank
	carrierKDDI
	carrierDocomo
	carrierRakuten
	carrierEEUK
	carrierVodafoneUK
)

const (
	// Create variables with short names to use them in |carrierAPNs| and make the dict declaration legible.
	apn         = shillconst.DevicePropertyCellularAPNInfoApnName
	attach      = shillconst.DevicePropertyCellularAPNInfoApnAttach
	attachTrue  = shillconst.DevicePropertyCellularAPNInfoApnAttachTrue
	ipType      = shillconst.DevicePropertyCellularAPNInfoApnIPType
	ipv4        = shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv4
	ipv4v6      = shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv4v6
	ipv6        = shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv6
	apnTypes    = shillconst.DevicePropertyCellularAPNInfoApnTypes
	typeDefault = shillconst.DevicePropertyCellularAPNInfoApnTypeDefault
	typeIA      = shillconst.DevicePropertyCellularAPNInfoApnTypeIA
	auth        = shillconst.DevicePropertyCellularAPNInfoApnAuthentication
	chap        = shillconst.DevicePropertyCellularAPNInfoApnAuthenticationChap
	pap         = shillconst.DevicePropertyCellularAPNInfoApnAuthenticationPap
	username    = shillconst.DevicePropertyCellularAPNInfoApnUsername
	password    = shillconst.DevicePropertyCellularAPNInfoApnPassword
)

var (
	// When updating this list, please also update the list in cellular/data/callbox_no_apns.prototxt
	// and regenerate the *.pbf files by following the directions in cellular/data/README.md.
	carrierMapping = map[string]carrier{
		"00101":  carrierAmarisoft,
		"001010": carrierAmarisoft,
		"23415":  carrierVodafoneUK,
		"23430":  carrierEEUK,
		"310260": carrierTmobile,
		"311882": carrierTmobile,
		"310280": carrierAtt,
		"310410": carrierAtt,
		"311480": carrierVerizon,
		"44010":  carrierDocomo,
		"44011":  carrierRakuten,
		"44020":  carrierSoftbank,
		"44051":  carrierKDDI,
	}

	carrierAPNs = map[carrier][]KnownAPN{
		carrierAmarisoft: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "callbox-ipv4", attach: attachTrue, ipType: ipv4, apnTypes: typeDefault + "," + typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "callbox-ipv4", ipType: ipv4, apnTypes: typeDefault}},
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "callbox-ipv4-chap", attach: attachTrue, ipType: ipv4, username: "username", password: "password", apnTypes: typeDefault + "," + typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "callbox-ipv4-pap", attach: attachTrue, ipType: ipv4, username: "username", password: "password", auth: pap, apnTypes: typeDefault + "," + typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "callbox-ipv6", attach: attachTrue, ipType: ipv6, apnTypes: typeDefault + "," + typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "callbox-ipv4v6", attach: attachTrue, ipType: ipv4v6, apnTypes: typeDefault + "," + typeIA}},
		},
		// US
		carrierTmobile: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "fast.t-mobile.com", attach: attachTrue, ipType: ipv4v6, apnTypes: typeDefault + "," + typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "fast.t-mobile.com", ipType: ipv4v6, apnTypes: typeDefault}},
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "fast.t-mobile.com", ipType: ipv4, apnTypes: typeDefault}},
		},
		carrierAtt: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "broadband", attach: attachTrue, ipType: ipv4v6, apnTypes: typeDefault + "," + typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "broadband"}},
		},
		carrierVerizon: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "vzwinternet", attach: attachTrue, ipType: ipv4v6, apnTypes: typeDefault + "," + typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "vzwinternet"}},
		},
		// Japan
		carrierKDDI: []KnownAPN{
			KnownAPN{Optional: true, APNInfo: map[string]string{apn: "au.au-net.ne.jp", attach: attachTrue, ipType: ipv4v6, username: "user@au.au-net.ne.jp", password: "au", auth: chap, apnTypes: typeDefault + "," + typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]string{apn: "uno.au-net.ne.jp", attach: attachTrue, ipType: ipv4v6, username: "685840734641020@uno.au-net.ne.jp", password: "KpyrR6BP", auth: chap, apnTypes: typeDefault + "," + typeIA}},
		},
		carrierDocomo: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "spmode.ne.jp", attach: attachTrue, ipType: ipv4v6, auth: chap, apnTypes: typeDefault + "," + typeIA}},
		},
		carrierRakuten: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "rakuten.jp", attach: attachTrue, ipType: ipv4v6, apnTypes: typeDefault + "," + typeIA}},
		},
		carrierSoftbank: []KnownAPN{
			KnownAPN{Optional: true, APNInfo: map[string]string{apn: "plus.acs.jp.v6", attach: attachTrue, ipType: ipv4v6, username: "ym", password: "ym", auth: chap, apnTypes: typeDefault + "," + typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]string{apn: "cmn.mgx", attach: attachTrue, ipType: ipv4v6, username: "cmn@mgx", password: "mgx", auth: pap, apnTypes: typeDefault + "," + typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]string{apn: "plus.4g", attach: attachTrue, ipType: ipv4v6, username: "plus", password: "4g", auth: chap, apnTypes: typeDefault + "," + typeIA}},
		},
		// UK
		carrierEEUK: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]string{apn: "everywhere", ipType: ipv4v6, username: "eesecure", password: "secure", auth: pap, apnTypes: typeDefault}},
		},
		carrierVodafoneUK: []KnownAPN{
			KnownAPN{Optional: true, APNInfo: map[string]string{apn: "wap.vodafone.co.uk", ipType: ipv4v6, username: "wap", password: "wap", apnTypes: typeDefault}},
			KnownAPN{Optional: true, APNInfo: map[string]string{apn: "pp.vodafone.co.uk", username: "web", password: "web", apnTypes: typeDefault}},
		},
	}
)

// IsAttach returns true if the ApnTypes contains DevicePropertyCellularAPNInfoApnIA
func IsAttach(apnInfo map[string]string) (bool, error) {
	apnTypes, okApnTypes := apnInfo[shillconst.DevicePropertyCellularAPNInfoApnTypes]
	if !okApnTypes {
		return false, errors.Errorf("missing key %q", shillconst.DevicePropertyCellularAPNInfoApnTypes)
	}
	types := strings.Split(apnTypes, ",")
	for _, t := range types {
		if t == typeIA {
			return true, nil
		}
	}
	return false, nil
}

// GetKnownAPNsForOperator returns a list of known APNs for a carrier.
func GetKnownAPNsForOperator(operatorID string) ([]KnownAPN, error) {
	carrier, ok := carrierMapping[operatorID]
	if !ok {
		operatorID1 := operatorID[0:5]
		carrier, ok = carrierMapping[operatorID1]
		if !ok {
			return nil, errors.Errorf("cannot find carrier for operators %q or %q", operatorID, operatorID1)
		}
		operatorID = operatorID1
	}
	apns, ok := carrierAPNs[carrier]
	if !ok {
		return nil, errors.Errorf("there are no APNs for operator %q", operatorID)
	}
	return apns, nil
}
