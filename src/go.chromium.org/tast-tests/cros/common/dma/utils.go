// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dma

import (
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/accountmanager"
	"go.chromium.org/tast-tests/cros/common/arc"
	"go.chromium.org/tast-tests/cros/common/assistant"
	"go.chromium.org/tast-tests/cros/common/calendar"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/connectivityfwdumps"
	"go.chromium.org/tast-tests/cros/common/drivefs"
	"go.chromium.org/tast-tests/cros/common/filemanager"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/common/wallpaper"
	"go.chromium.org/tast/core/testing"
)

var dmaEnableVar = testing.RegisterVarString(
	"dma.Enable",
	"false",
	"It indicates whether dma is enabled or not",
)

func pools() (map[string]string, map[string]string) {
	var dmaPools = map[string]string{

		accountmanager.AccountPoolVarName:           ui.GaiaDMAPoolDefaultValue(),
		arc.ChildAccountVarName:                     arc.ChildDMAAccountValue(),
		arc.DrivefsPoolVarName:                      ui.GaiaDMAPoolDefaultValue(),
		arc.Managed3pEmmAccountVarName:              arc.ManagedDMAAccountPoolValue(),
		arc.ManagedAccountPoolVarName:               arc.ManagedDMAAccountPoolValue(),
		arc.ParentAccountVarName:                    arc.ParentDMAAccountValue(),
		arc.SharesheetPoolVarName:                   ui.GaiaDMAPoolDefaultValue(),
		assistant.AccountPoolVarName:                ui.GaiaDMAPoolDefaultValue(),
		calendar.GoogleCalendarAccountPoolVarName:   calendar.GoogleCalendarDMAAccountPoolValue(),
		calendar.UpcomingEventsAccountVarName:       calendar.UpcomingEventsDMAAccountValue(),
		connectivityfwdumps.GaiaLoginAccountVarName: arc.ManagedDMAAccountPoolValue(),
		drivefs.AccountPoolVarName:                  ui.GaiaDMAPoolDefaultValue(),
		filemanager.FullAccountPoolVarName:          filemanager.FullDMAAccountPoolValue(),
		filemanager.OrgFullAccountPoolVarName:       filemanager.OrgFullDMAAccountPoolValue(),
		filemanager.WarnAccountPoolVarName:          filemanager.WarnDMAAccountPoolValue(),
		policy.ManagedUserAccountPoolVarName:        arc.ManagedDMAAccountPoolValue(),
		ui.GaiaPoolDefaultVarName:                   ui.GaiaDMAPoolDefaultValue(),
		ui.CUJAccountPoolVarName:                    ui.GaiaDMAPoolDefaultValue(),
		wallpaper.GooglePhotosAccountPoolVarName:    wallpaper.GooglePhotosDMAAccountPoolValue(),
	}

	var regularPools = map[string]string{
		accountmanager.AccountPoolVarName:           accountmanager.AccountPoolValue(),
		arc.ChildAccountVarName:                     arc.ChildAccountValue(),
		arc.DrivefsPoolVarName:                      arc.DrivefsPoolValue(),
		arc.Managed3pEmmAccountVarName:              arc.Managed3pEmmAccountValue(),
		arc.ManagedAccountPoolVarName:               arc.ManagedAccountPoolValue(),
		arc.ParentAccountVarName:                    arc.ParentAccountValue(),
		arc.SharesheetPoolVarName:                   arc.SharesheetPoolValue(),
		assistant.AccountPoolVarName:                assistant.AccountPoolValue(),
		calendar.GoogleCalendarAccountPoolVarName:   calendar.GoogleCalendarAccountPoolValue(),
		calendar.UpcomingEventsAccountVarName:       calendar.UpcomingEventsAccountValue(),
		connectivityfwdumps.GaiaLoginAccountVarName: connectivityfwdumps.GaiaLoginAccountValue(),
		drivefs.AccountPoolVarName:                  drivefs.AccountPoolValue(),
		filemanager.FullAccountPoolVarName:          filemanager.FullAccountPoolValue(),
		filemanager.OrgFullAccountPoolVarName:       filemanager.OrgFullAccountPoolValue(),
		filemanager.WarnAccountPoolVarName:          filemanager.WarnAccountPoolValue(),
		policy.ManagedUserAccountPoolVarName:        policy.ManagedUserAccountPoolValue(),
		ui.GaiaPoolDefaultVarName:                   ui.GaiaPoolDefaultValue(),
		ui.CUJAccountPoolVarName:                    ui.CUJAccountPoolValue(),
		wallpaper.GooglePhotosAccountPoolVarName:    wallpaper.GooglePhotosAccountPoolValue(),
	}

	return dmaPools, regularPools
}

func enabled() bool {
	return strings.ToLower(dmaEnableVar.Value()) == "true"
}

// CredsFromPool returns proper credentials based on DMA status.
func CredsFromPool(pool string) string {
	dmaPools, regularPools := pools()

	dmaEnabled := enabled()
	if dmaEnabled {
		creds, ok := dmaPools[pool]
		if !ok {
			panic(fmt.Sprintf("Pool %q not onboard DMA yet", pool))
		}

		return creds
	}
	creds, ok := regularPools[pool]
	if !ok {
		panic(fmt.Sprintf("Pool %q not convert to global runtime variable yet", pool))
	}

	return creds
}

// UserPassFromPool returns a random username, password, error (if present) from pool.
func UserPassFromPool(pool string) (user, pass string, err error) {
	creds := CredsFromPool(pool)
	cred, err := credconfig.PickRandomCreds(creds)
	if err != nil {
		return "", "", err
	}

	return cred.User, cred.Pass, nil
}
