// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"chromiumos/tast/common/hermesconst"
	"chromiumos/tast/common/shillconst"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/cellular"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/dbusutil"
	"chromiumos/tast/local/hermes"
	"chromiumos/tast/local/modemmanager"
	"chromiumos/tast/local/network/netconfig"
	"chromiumos/tast/local/power"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularSuspendResumeConnect,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that cellular reconnects after a suspend and resume only when autoconnect is enabled",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_prod_esim"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellular",
		Timeout:      5 * time.Minute,
	})
}

func CellularSuspendResumeConnect(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	if _, err := modemmanager.NewModemWithSim(ctx); err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}

	helper, err := cellular.NewHelperWithConnectedCellular(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
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

	profileName, err := getConnectedProfileNickname(ctx)
	if err != nil {
		s.Fatal("Could not get connected profile Nickname: ", err)
	}

	if profileName == "" {
		profileName = networkName
	}

	mdp, err := ossettings.OpenNetworkDetailPage(ctx, tconn, cr, profileName, netconfig.Cellular)
	defer mdp.Close(ctx)
	if err != nil {
		s.Fatal("Failed to open cellular details subpage: ", err)
	}

	if err := mdp.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.ConnectedStatus)(ctx); err != nil {
		s.Fatal("Failed to verify network is connected: ", err)
	}

	if err := power.SuspendAndResume(ctx, cr, 15*time.Second); err != nil {
		s.Fatal("Failed to suspend and resume: ", err)
	}

	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to re-establish the Test API connection: ", err)
	}

	mdp, err = ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data page: ", err)
	}

	mdp, err = ossettings.OpenNetworkDetailPage(ctx, tconn, cr, profileName, netconfig.Cellular)
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

	if err := power.SuspendAndResume(ctx, cr, 15*time.Second); err != nil {
		s.Fatal("Failed to suspend and resume: ", err)
	}

	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to re-establish the Test API connection: ", err)
	}

	mdp, err = ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data page: ", err)
	}

	mdp, err = ossettings.OpenNetworkDetailPage(ctx, tconn, cr, profileName, netconfig.Cellular)
	if err != nil {
		s.Fatal("Failed to open cellular details subpage: ", err)
	}

	if err := mdp.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.ConnectedStatus)(ctx); err == nil {
		s.Fatal("Failed to verify network remains disconnected: ", err)
	}
}

func getConnectedProfileNickname(ctx context.Context) (string, error) {
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

	helper, err := cellular.NewHelperWithConnectedCellular(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to create cellular.Helper")
	}

	connectedIccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		return "", errors.Wrap(err, "could not get ICCID")
	}

	for _, profile := range profiles {
		props, err := dbusutil.NewDBusProperties(ctx, profile.DBusObject)

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
