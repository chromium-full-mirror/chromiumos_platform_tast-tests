// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cellular provides functions for testing Cellular connectivity.
package cellular

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
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

// Carrier represents a cellular carrier.
type Carrier int

// List of all cellular carriers in the lab.
const (
	CarrierUnknown Carrier = iota
	CarrierAmarisoft
	CarrierSysmocom // aleksandermj test network
	CarrierVerizon
	CarrierTmobile
	CarrierAtt
	CarrierSoftbank
	CarrierKDDI
	CarrierDocomo
	CarrierRakuten
	CarrierEEUK
	CarrierVodafoneUK
	CarrierRoger
	CarrierTelus
	CarrierCBRS
)

const (
	// Create variables with short names to use them in |CarrierAPNs| and make the dict declaration legible.
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
	carrierMapping = map[string]Carrier{
		"00101":  CarrierAmarisoft,
		"001010": CarrierAmarisoft,
		"90170":  CarrierSysmocom,
		"99970":  CarrierSysmocom,
		"23415":  CarrierVodafoneUK,
		"23430":  CarrierEEUK,
		"302220": CarrierTelus,
		"302720": CarrierRoger,
		"310260": CarrierTmobile,
		"311882": CarrierTmobile,
		"310280": CarrierAtt,
		"310410": CarrierAtt,
		"311480": CarrierVerizon,
		"44010":  CarrierDocomo,
		"44011":  CarrierRakuten,
		"44020":  CarrierSoftbank,
		"44051":  CarrierKDDI,
		"99940":  CarrierCBRS,
	}
)

func initializeCarrierAPNs() map[Carrier][]KnownAPN {
	return map[Carrier][]KnownAPN{
		CarrierAmarisoft: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv4", ipType: ipv4}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv4", ipType: ipv4}, APNTypes: []string{typeDefault}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv4-chap", ipType: ipv4, username: "username", password: "password"}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv4-pap", ipType: ipv4, username: "username", password: "password", auth: pap}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv6", ipType: ipv6}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "callbox-ipv4v6", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
		},
		CarrierSysmocom: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "internet", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
		},
		// US
		CarrierTmobile: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "fast.t-mobile.com", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "fast.t-mobile.com", ipType: ipv4v6}, APNTypes: []string{typeDefault}},
		},
		CarrierAtt: []KnownAPN{
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "broadband", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "broadband"}, APNTypes: []string{typeDefault}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "nrbroadband", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "nrbroadband"}, APNTypes: []string{typeDefault}},
		},
		CarrierVerizon: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "vzwinternet", ipType: ipv4v6}, APNTypes: []string{typeDefault}},
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "vzwinternet"}, APNTypes: []string{typeDefault}},
		},
		// Japan
		CarrierKDDI: []KnownAPN{
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "au.au-net.ne.jp", ipType: ipv4v6, username: "user@au.au-net.ne.jp", password: "au", auth: chap}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "uno.au-net.ne.jp", ipType: ipv4v6, username: "685840734641020@uno.au-net.ne.jp", password: "KpyrR6BP", auth: chap}, APNTypes: []string{typeDefault, typeIA}},
		},
		CarrierDocomo: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "spmode.ne.jp", ipType: ipv4v6, auth: chap}, APNTypes: []string{typeDefault, typeIA}},
		},
		CarrierRakuten: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "rakuten.jp", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
		},
		CarrierSoftbank: []KnownAPN{
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "plus.acs.jp.v6", ipType: ipv4v6, username: "ym", password: "ym", auth: chap}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "cmn.mgx", ipType: ipv4v6, username: "cmn@mgx", password: "mgx", auth: pap}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "plus.4g", ipType: ipv4v6, username: "plus", password: "4g", auth: chap}, APNTypes: []string{typeDefault, typeIA}},
		},
		// UK
		CarrierEEUK: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "everywhere", ipType: ipv4v6, username: "eesecure", password: "secure", auth: pap}, APNTypes: []string{typeDefault}},
		},
		CarrierVodafoneUK: []KnownAPN{
			KnownAPN{Optional: false, APNInfo: map[string]interface{}{apn: "wap.vodafone.co.uk", ipType: ipv4v6, username: "wap", password: "wap"}, APNTypes: []string{typeDefault}},
		},
		// Canada
		CarrierRoger: []KnownAPN{
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "mobile.apn", ipType: ipv4v6}, APNTypes: []string{typeDefault}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "ltemobile.apn", ipType: ipv4v6}, APNTypes: []string{typeDefault, typeIA}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "ltemobile.apn", ipType: ipv4v6}, APNTypes: []string{typeDefault}},
		},
		CarrierTelus: []KnownAPN{
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "isp.telus.com", ipType: ipv4v6}, APNTypes: []string{typeDefault}},
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "sp.telus.com", ipType: ipv4v6}, APNTypes: []string{typeDefault}},
		},
		CarrierCBRS: []KnownAPN{
			KnownAPN{Optional: true, APNInfo: map[string]interface{}{apn: "", ipType: ipv4v6}, APNTypes: []string{typeDefault}},
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
	carrier, err := GetCarrier(operatorID)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get KnownAPNs")
	}
	apns, ok := carrierAPNs[carrier]
	if !ok {
		return nil, errors.Errorf("there are no APNs for operator %q", operatorID)
	}
	return apns, nil
}

// GetCarrier returns the carrier that matches the operatorID.
func GetCarrier(operatorID string) (Carrier, error) {
	if len(operatorID) < 5 || len(operatorID) > 6 {
		return CarrierUnknown, errors.Errorf("operator ID %q is malformed", operatorID)
	}

	carrier, ok := carrierMapping[operatorID]
	if !ok {
		operatorID1 := operatorID[0:5]
		carrier, ok = carrierMapping[operatorID1]
		if !ok {
			return CarrierUnknown, errors.Errorf("cannot find carrier for operators %q or %q", operatorID, operatorID1)
		}
	}
	return carrier, nil
}

// CheckThatApnListIsEmpty is used to navigate to the APN details page and check that the APN list is empty.
// This method expects to be called from the details page of a cellular network and is safe to call regardless of login state.
func CheckThatApnListIsEmpty(ctx context.Context, tconn *chrome.TestConn, apnSubpageButtonNode *nodewith.Finder) error {
	ui := uiauto.New(tconn)

	apnNotConnectedText := nodewith.NameContaining("You are not connected yet").Role(role.Link)
	if err := uiauto.Combine("Navigate to the APN subpage and verify no APN information is shown",
		ui.WaitUntilExists(apnSubpageButtonNode),
		ui.DoDefault(apnSubpageButtonNode),
		ui.WaitUntilExists(apnNotConnectedText),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to navigate to the APN subpage and verify no APN information is shown")
	}
	return nil
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
