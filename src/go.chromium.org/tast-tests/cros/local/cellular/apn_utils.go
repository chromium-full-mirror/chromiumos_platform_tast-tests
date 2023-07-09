// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cellular provides functions for testing Cellular connectivity.
package cellular

import (
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast/core/errors"
)

// KnownAPN is an APN known to a carrier.
// |Optional| indicates that any of the optional APNs might work, but not all.
// At least one of the |Optional| APNs has to work
type KnownAPN struct {
	Optional bool
	APNInfo  map[string]interface{}
	APNTypes []string
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
	ipType      = shillconst.DevicePropertyCellularAPNInfoApnIPType
	ipv4        = shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv4
	ipv4v6      = shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv4v6
	ipv6        = shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv6
	typeDefault = shillconst.DevicePropertyCellularAPNInfoApnTypeDefault
	typeIA      = shillconst.DevicePropertyCellularAPNInfoApnTypeIA
	auth        = shillconst.DevicePropertyCellularAPNInfoApnAuthentication
	chap        = shillconst.DevicePropertyCellularAPNInfoApnAuthenticationChap
	pap         = shillconst.DevicePropertyCellularAPNInfoApnAuthenticationPap
	username    = shillconst.DevicePropertyCellularAPNInfoApnUsername
	password    = shillconst.DevicePropertyCellularAPNInfoApnPassword
)

var (
	// When updating this list, please also update the list in cellular/data/test_no_apns.prototxt
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
)

func initializeCarrierAPNs() map[carrier][]KnownAPN {
	return map[carrier][]KnownAPN{
		carrierAmarisoft: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv4", ipType: ipv4}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv4", ipType: ipv4}, APNTypes: []string{typeDefault}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv4-chap", ipType: ipv4, username: "username", password: "password"}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv4-pap", ipType: ipv4, username: "username", password: "password", auth: pap}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv6", ipType: ipv6}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv4v6", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
		},
		// US
		carrierTmobile: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "fast.t-mobile.com", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "fast.t-mobile.com", ipType: ipv4v6}, APNTypes: []string{typeDefault}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "fast.t-mobile.com", ipType: ipv4}, APNTypes: []string{typeDefault}},
		},
		carrierAtt: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "broadband", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "broadband"}, APNTypes: []string{typeDefault}},
		},
		carrierVerizon: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "vzwinternet", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "vzwinternet"}, APNTypes: []string{typeDefault}},
		},
		// Japan
		carrierKDDI: []KnownAPN{
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "au.au-net.ne.jp", ipType: ipv4v6, username: "user@au.au-net.ne.jp", password: "au", auth: chap}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "uno.au-net.ne.jp", ipType: ipv4v6, username: "685840734641020@uno.au-net.ne.jp", password: "KpyrR6BP", auth: chap}, APNTypes: []string{typeDefault, typeIA}},
		},
		carrierDocomo: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "spmode.ne.jp", ipType: ipv4v6, auth: chap}, APNTypes: []string{typeDefault, typeIA}},
		},
		carrierRakuten: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "rakuten.jp", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
		},
		carrierSoftbank: []KnownAPN{
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "plus.acs.jp.v6", ipType: ipv4v6, username: "ym", password: "ym", auth: chap}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "cmn.mgx", ipType: ipv4v6, username: "cmn@mgx", password: "mgx", auth: pap}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "plus.4g", ipType: ipv4v6, username: "plus", password: "4g", auth: chap}, APNTypes: []string{typeDefault, typeIA}},
		},
		// UK
		carrierEEUK: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "everywhere", ipType: ipv4v6, username: "eesecure", password: "secure", auth: pap}, APNTypes: []string{typeDefault}},
		},
		carrierVodafoneUK: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "wap.vodafone.co.uk", ipType: ipv4v6, username: "wap", password: "wap"}, APNTypes: []string{typeDefault}},
		},
	}
}

// IsAttachAPN returns true if the ApnTypes contains DevicePropertyCellularAPNInfoApnIA
func (knownAPN KnownAPN) IsAttachAPN() bool {
	for _, t := range knownAPN.APNTypes {
		if t == typeIA {
			return true
		}
	}
	return false
}

// GetKnownAPNsForOperator returns a modifiable list of known APNs for a carrier.
func GetKnownAPNsForOperator(operatorID string) ([]KnownAPN, error) {
	carrierAPNs := initializeCarrierAPNs()
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

// GetAPNForModemManager removes any keys not recognized by MM from ApnInfo and replaces
// any key/value by the MM equivalent.
func (knownAPN KnownAPN) GetAPNForModemManager() map[string]interface{} {
	keyMapping := map[string]string{apn: mmconst.BearerPropertyApn,
		ipType:   mmconst.BearerPropertyIPType,
		auth:     mmconst.BearerPropertyAllowedAuth,
		username: mmconst.BearerPropertyUser,
		password: mmconst.BearerPropertyPassword,
	}
	keyAuthMapping := map[interface{}]mmconst.BearerAllowedAuth{chap: mmconst.BearerAllowedAuthCHAP,
		pap: mmconst.BearerAllowedAuthPAP,
	}
	keyIPMapping := map[interface{}]mmconst.BearerIPFamily{ipv4: mmconst.BearerIPFamilyIPv4,
		ipv6:   mmconst.BearerIPFamilyIPv6,
		ipv4v6: mmconst.BearerIPFamilyIPv4v6,
	}
	ret := make(map[string]interface{})
	for shillKey, v := range knownAPN.APNInfo {
		if mmKey, ok := keyMapping[shillKey]; ok {
			if shillKey == auth {
				if mmVal, ok := keyAuthMapping[v]; ok {
					ret[mmKey] = mmVal
				} else {
					errorString := fmt.Sprintf("invalid Authentication type %q", v)
					panic(errorString)
				}
			} else if shillKey == ipType {
				if mmVal, ok := keyIPMapping[v]; ok {
					ret[mmKey] = mmVal
				} else {
					errorString := fmt.Sprintf("invalid IP type %q", v)
					panic(errorString)
				}
			} else {
				ret[mmKey] = v
			}
		}
	}
	return ret
}

// GetAPNForShill converts the ApnInfo into a map[string]string.
func (knownAPN KnownAPN) GetAPNForShill() map[string]string {
	ret := make(map[string]string, len(knownAPN.APNInfo))
	for k, v := range knownAPN.APNInfo {
		ret[k] = v.(string)
	}
	if knownAPN.IsAttachAPN() {
		ret[shillconst.DevicePropertyCellularAPNInfoApnAttach] = shillconst.DevicePropertyCellularAPNInfoApnAttachTrue
	}
	ret[shillconst.DevicePropertyCellularAPNInfoApnTypes] = strings.Join(knownAPN.APNTypes, ",")
	return ret
}
