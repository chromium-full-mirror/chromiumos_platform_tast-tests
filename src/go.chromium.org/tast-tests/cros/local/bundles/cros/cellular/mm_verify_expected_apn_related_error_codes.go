// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
)

type verifyExpectedApnRelatedErrorCodesTest int

const (
	ipErrors verifyExpectedApnRelatedErrorCodesTest = iota
	authenticationErrors
	wrongApnErrors
)

type verifyExpectedApnRelatedErrorCodesTestParam struct {
	errorType verifyExpectedApnRelatedErrorCodesTest
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           MMVerifyExpectedApnRelatedErrorCodes,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that Modem Manager reports the correct error codes for multiple APN related issues",
		Contacts:       []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		Fixture:        "cellularModemManager",
		Params: []testing.Param{{
			Name: "ip",
			Val:  verifyExpectedApnRelatedErrorCodesTestParam{ipErrors},
		}, {
			Name: "authentication",
			Val:  verifyExpectedApnRelatedErrorCodesTestParam{authenticationErrors},
		}, {
			Name: "wrong_apn",
			Val:  verifyExpectedApnRelatedErrorCodesTestParam{wrongApnErrors},
		}},
	})
}

// MMVerifyExpectedApnRelatedErrorCodes Test
func MMVerifyExpectedApnRelatedErrorCodes(ctx context.Context, s *testing.State) {
	// (1) Note: The golang dbus error only contains the error message and doesn't contain the error code.
	//     To get the last error code, we need to find the last bearer used and retrieve the error from the bearer.
	params := s.Param().(verifyExpectedApnRelatedErrorCodesTestParam)

	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		s.Fatal("Failed to create Modem: ", err)
	}
	modem3gpp, err := modem.GetModem3gpp(ctx)
	if err != nil {
		s.Fatal("Could not get modem3gpp object: ", err)
	}
	simpleModem, err := modem.GetSimpleModem(ctx)
	if err != nil {
		s.Fatal("Could not get simplemodem object: ", err)
	}

	// Create variables with short names to make the dict declarations legible.
	const apn = mmconst.BearerPropertyApn
	const auth = mmconst.BearerPropertyAllowedAuth
	const user = mmconst.BearerPropertyUser
	const pass = mmconst.BearerPropertyPassword
	const ipType = mmconst.BearerPropertyIPType
	const apnType = mmconst.BearerPropertyApnType

	// Clear the attach APN to use the default attach APN
	if err := modemmanager.SetInitialEpsBearerSettings(ctx, modem3gpp, map[string]interface{}{apn: "", apnType: mmconst.BearerAPNTypeInitial}); err != nil {
		s.Fatal("Failed to clear the initial EPS bearer settings: ", err)
	}
	if err := modemmanager.EnsureRegistered(ctx, modem, simpleModem); err != nil {
		s.Fatal("Modem not registered: ", err)
	}

	// Shorten deadline to leave time for cleanup.
	clearCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Second)
	defer func(ctx context.Context) {
		if err := simpleModem.Call(ctx, mmconst.ModemDisconnect, dbus.ObjectPath("/")).Err; err != nil {
			testing.ContextLog(ctx, "Modem disconnect failed with: ", err)
		}
		modem.DeleteAllBearers(ctx, modem)
	}(clearCtx)
	defer cancel()

	switch params.errorType {
	case wrongApnErrors:
		errorCode := connectAndGetBearerErrorCode(ctx, s, modem, simpleModem, map[string]interface{}{apnType: mmconst.BearerAPNTypeDefault})
		if errorCode != "org.freedesktop.ModemManager1.Error.Core.InvalidArgs" {
			s.Fatalf("Expected error for empty apn properties doesn't match : %s", errorCode)
		}

		serviceOptionNotSubscribed := "org.freedesktop.ModemManager1.Error.MobileEquipment.ServiceOptionNotSubscribed"
		missingOrUnknownApn := "org.freedesktop.ModemManager1.Error.MobileEquipment.MissingOrUnknownApn"
		errorCode = connectAndGetBearerErrorCode(ctx, s, modem, simpleModem, map[string]interface{}{apn: "invalid-apn", apnType: mmconst.BearerAPNTypeDefault})
		if errorCode != serviceOptionNotSubscribed && errorCode != missingOrUnknownApn {
			s.Fatalf("Expected error for invalid apn doesn't match : %s", errorCode)
		}

	case authenticationErrors:
		userAuthenticationFailed := "org.freedesktop.ModemManager1.Error.MobileEquipment.UserAuthenticationFailed"
		errorCode := connectAndGetBearerErrorCode(ctx, s, modem, simpleModem, map[string]interface{}{apn: "callbox-ipv4-chap", user: "wrong-username", apnType: mmconst.BearerAPNTypeDefault})
		if errorCode != userAuthenticationFailed {
			s.Fatalf("Expected error for authentication failed doesn't match:%s", errorCode)
		}

		errorCode = connectAndGetBearerErrorCode(ctx, s, modem, simpleModem, map[string]interface{}{apn: "callbox-ipv4-chap", user: "username", pass: "wrong-password", apnType: mmconst.BearerAPNTypeDefault})
		if errorCode != userAuthenticationFailed {
			s.Fatalf("Expected error for authentication failed doesn't match:%s", errorCode)
		}

		isL850, _ := cellular.IsModemType(ctx, cellularconst.ModemTypeL850)
		errorCode = connectAndGetBearerErrorCode(ctx, s, modem, simpleModem, map[string]interface{}{apn: "callbox-ipv4-chap", user: "username", pass: "password", auth: mmconst.BearerAllowedAuthPAP, apnType: mmconst.BearerAPNTypeDefault})
		if isL850 && errorCode == "" {
			testing.ContextLog(ctx, "INFO: b/264796148 L850 connects with any authentication")
		} else if errorCode != userAuthenticationFailed {
			s.Fatalf("Expected error for authentication failed doesn't match:%s", errorCode)
		}

		errorCode = connectAndGetBearerErrorCode(ctx, s, modem, simpleModem, map[string]interface{}{apn: "callbox-ipv4-pap", user: "username", pass: "password", apnType: mmconst.BearerAPNTypeDefault})
		if isL850 && errorCode == "" {
			testing.ContextLog(ctx, "INFO: b/264796148 L850 connects with any authentication")
		} else if errorCode != userAuthenticationFailed {
			s.Fatalf("Expected error for authentication failed doesn't match:%s", errorCode)
		}

		errorCode = connectAndGetBearerErrorCode(ctx, s, modem, simpleModem, map[string]interface{}{apn: "callbox-ipv4-pap", user: "username", pass: "password", auth: mmconst.BearerAllowedAuthCHAP, apnType: mmconst.BearerAPNTypeDefault})
		if isL850 && errorCode == "" {
			testing.ContextLog(ctx, "INFO: b/264796148 L850 connects with any authentication")
		} else if errorCode != userAuthenticationFailed {
			s.Fatalf("Expected error for authentication failed doesn't match:%s", errorCode)
		}

	case ipErrors:
		// Fail quickly on FM101 variants with old FW, since running this test puts the modem into a bad state(b/302697538#comment5)
		errOldFw := cellular.TagKnownBugOnModem(ctx, nil, "b/302697538", cellular.ModemFwFilterFM101MR1)
		if errOldFw != nil {
			s.Fatalf("Fail early to avoid putting the DUT in a bad state: %s", errOldFw)
		}
		errorCode := connectAndGetBearerErrorCode(ctx, s, modem, simpleModem, map[string]interface{}{apn: "callbox-ipv4", ipType: mmconst.BearerIPFamilyIPv6, apnType: mmconst.BearerAPNTypeDefault})
		if errorCode != "org.freedesktop.ModemManager1.Error.MobileEquipment.Ipv4OnlyAllowed" {
			err := cellular.TagKnownBugOnModem(ctx, nil, "b/263815534", cellular.ModemFwFilterL850MR7AndLower)
			err = cellular.TagKnownBugOnModem(ctx, err, "b/290110554", cellular.ModemFwFilterFM350MR3AndLower)
			s.Fatalf("Expected error for Ipv4OnlyAllowed doesn't match:%s: %s", errorCode, err)
		}

		errorCode = connectAndGetBearerErrorCode(ctx, s, modem, simpleModem, map[string]interface{}{apn: "callbox-ipv6", ipType: mmconst.BearerIPFamilyIPv4, apnType: mmconst.BearerAPNTypeDefault})
		if errorCode != "org.freedesktop.ModemManager1.Error.MobileEquipment.Ipv6OnlyAllowed" {
			err := cellular.TagKnownBugOnModem(ctx, nil, "b/263815534", cellular.ModemFwFilterL850MR7AndLower)
			err = cellular.TagKnownBugOnModem(ctx, err, "b/290110554", cellular.ModemFwFilterFM350MR3AndLower)
			s.Fatalf("Expected error for Ipv6OnlyAllowed doesn't match:%s: %s", errorCode, err)
		}

	default:
		s.Fatal("Unknown parameterized test")
	}

}

func connectAndGetBearerErrorCode(ctx context.Context, s *testing.State, modem, simpleModem *modemmanager.Modem, props map[string]interface{}) string {
	testing.ContextLog(ctx, "Connecting with APN: ", props)
	// Clear all bearer objects
	if err := simpleModem.Call(ctx, mmconst.ModemDisconnect, dbus.ObjectPath("/")).Err; err != nil {
		s.Fatal("Modem disconnect failed with: ", err)
	}
	modem.DeleteAllBearers(ctx, modem)

	if _, err := modemmanager.Connect(ctx, simpleModem, props); err != nil {
		testing.ContextLogf(ctx, "Expected error: fail to connect: %q", err)
	} else {
		return ""
	}
	bearer, err := modem.GetFirstDataBearer(ctx, mmconst.BearerAPNTypeDefault)
	if err != nil {
		s.Fatalf("Failed to get bearer : %s", err)
	}
	connectionError, err := bearer.ConnectionError().ToString()
	if err != nil {
		s.Fatalf("Failed to get bearer error: %s", err)
	}
	return connectionError

}
