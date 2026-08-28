// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hermesconst"
	"go.chromium.org/tast-tests/cros/common/networkui/netconfigtypes"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast-tests/cros/local/power/suspend"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           SuspendResumeConnect,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Tests that cellular reconnects after a suspend and resume only when autoconnect is enabled",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:cellular", "cellular_sim_prod_esim"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularEnforceConnectionLocal",
		Timeout:      6 * time.Minute,
		Params: []testing.Param{{
			Name: "connected_before_suspend",
			Val:  false, // disconnect
		}, {
			Name: "disconnected_before_suspend",
			Val:  true, //disconnect
		}},
	})
}

func SuspendResumeConnect(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	if _, err := modemmanager.NewModemWithSim(ctx); err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	// Set up the device to autoconnect.
	cleanup1, err := helper.InitServiceProperty(ctx, shillconst.ServicePropertyAutoConnect, true)
	if err != nil {
		s.Fatal("Could not initialize autoconnect to true: ", err)
	}
	defer cleanup1(cleanupCtx)

	networkName, err := helper.GetCurrentNetworkName(ctx)
	if err != nil {
		s.Fatal("Could not get iccid: ", err)
	}

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	profileName, err := getConnectedProfileNickname(ctx, helper)
	if err != nil {
		s.Fatal("Could not get connected profile Nickname: ", err)
	}

	if profileName == "" {
		profileName = networkName
	}

	mdp, err := ossettings.OpenNetworkDetailPage(ctx, tconn, cr, profileName, netconfigtypes.Cellular)
	if err != nil {
		s.Fatal("Failed to open cellular details subpage: ", err)
	}
	defer mdp.Close(ctx)

	if err := mdp.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.ConnectedStatus)(ctx); err != nil {
		s.Fatal("Failed to verify network is connected: ", err)
	}

	if s.Param().(bool) {
		if err := uiauto.Combine("Disconnect the network",
			mdp.LeftClick(ossettings.DisconnectButton),
			mdp.WaitUntilExists(ossettings.DisconnectedStatus),
		)(ctx); err != nil {
			s.Fatal("Failed to disconnect the network: ", err)
		}
		s.Log("Disconnect successfully")
	}

	// Use wake up timeout to ensure device recovery.
	if _, err := suspend.Request(ctx, suspend.Delay(0), suspend.For(15*time.Second), suspend.WakeUpTimeout(20*time.Second)); err != nil {
		s.Fatal("Failed to suspend and resume: ", err)
	}

	if err = cr.Reconnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to Chrome: ", err)
	}

	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to re-establish the Test API connection: ", err)
	}

	_, err = ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data page: ", err)
	}

	mdp, err = ossettings.OpenNetworkDetailPage(ctx, tconn, cr, profileName, netconfigtypes.Cellular)
	if err != nil {
		s.Fatal("Failed to open cellular details subpage: ", err)
	}

	if err := mdp.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.ConnectedStatus)(ctx); err != nil {
		s.Fatal("Failed to verify network is connected: ", err)
	}

	// Set up the device to disable autoconnect.
	cleanup1, err = helper.InitServiceProperty(ctx, shillconst.ServicePropertyAutoConnect, false)
	if err != nil {
		s.Fatal("Could not initialize autoconnect to true: ", err)
	}
	defer cleanup1(cleanupCtx)

	// Use wake up timeout to ensure device recovery.
	if _, err := suspend.Request(ctx, suspend.Delay(0), suspend.For(15*time.Second), suspend.WakeUpTimeout(20*time.Second)); err != nil {
		s.Fatal("Failed to suspend and resume: ", err)
	}

	if err = cr.Reconnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to Chrome: ", err)
	}

	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to re-establish the Test API connection: ", err)
	}

	_, err = ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data page: ", err)
	}

	mdp, err = ossettings.OpenNetworkDetailPage(ctx, tconn, cr, profileName, netconfigtypes.Cellular)
	if err != nil {
		s.Fatal("Failed to open cellular details subpage: ", err)
	}

	if err := mdp.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.ConnectedStatus)(ctx); err == nil {
		s.Fatal("Failed to verify network remains disconnected: ", err)
	}
}

func getConnectedProfileNickname(ctx context.Context, helper *cellular.Helper) (string, error) {
	euicc, _, err := hermes.GetEUICC(ctx, false)
	if err != nil {
		return "", errors.Wrap(err, "could not get Hermes euicc")
	}

	testing.ContextLog(ctx, "Looking for installed profile")
	profiles, err := euicc.InstalledProfiles(ctx, false)
	if err != nil {
		return "", errors.Wrap(err, "could not get Hermes installed profiles")
	}

	if len(profiles) == 0 {
		return "", errors.Wrap(err, "there are no installed profiles")
	}

	if err := euicc.EnableAnyProfile(ctx); err != nil {
		return "", errors.Wrap(err, "could not enable any profiles")
	}

	if _, err := modemmanager.NewModemWithSim(ctx); err != nil {
		return "", errors.Wrap(err, "could not find MM dbus object with a valid sim")
	}

	if _, err := helper.Connect(ctx); err != nil {
		return "", errors.Wrap(err, "failed to connect to cellular service")
	}

	connectedIccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		return "", errors.Wrap(err, "could not get ICCID")
	}

	for _, profile := range profiles {
		props, _ := dbusutil.NewDBusProperties(ctx, profile.DBusObject)

		iccid, err := props.GetString(hermesconst.ProfilePropertyIccid)
		if err != nil {
			return "", errors.Wrap(err, "failed to read profile ICCID")
		}

		nickname, err := props.GetString(hermesconst.ProfilePropertyNickname)
		if err != nil {
			return "", errors.Wrap(err, "failed to read a profile's Nickname")
		}

		if iccid == connectedIccid {
			return nickname, nil
		}
	}
	return "", errors.Wrap(err, "no connected eSIM profile")
}
