// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/network/ip"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wep"
	"go.chromium.org/tast/core/errors"
)

const (
	// Autotest may be used on these routers too, and if it failed to clean up, we may be out of space in /tmp.

	// AutotestWorkdirGlob is the path that grabs all autotest outputs.
	AutotestWorkdirGlob = "/tmp/autotest-*"
	// WorkingDir is the tast-test's working directory.
	WorkingDir = "/tmp/tast-test/"
)

// RouterCloseContextDuration is a shorter context.Context duration is used for
// running things before Router.Close to reserve time for it to run.
const RouterCloseContextDuration = 5 * time.Second

// RouterCloseFrameSenderDuration is the length of time the context deadline
// should be shortened by to reserve time for r.CloseFrameSender() to run.
const RouterCloseFrameSenderDuration = 2 * time.Second

// TimestampFileFormat is the time format used for timestamps in generated
// file names and folder names.
const TimestampFileFormat = "20060102-150405"

// BuildWorkingDirPath creates a working directory path based on the current
// time and base WorkingDir. All temporary files shall be placed within this
// directory during the life of the router controller instance. The time-based
// subdirectory under WorkingDir separates different instances' temporary files.
func BuildWorkingDirPath() string {
	return fmt.Sprintf("%s/%s/", WorkingDir, time.Now().Format(TimestampFileFormat))
}

// RemoveDevicesWithPrefix removes the devices whose names start with the given prefix.
func RemoveDevicesWithPrefix(ctx context.Context, ipr *ip.Runner, prefix string) error {
	devs, err := ipr.LinkWithPrefix(ctx, prefix)
	if err != nil {
		return err
	}
	for _, dev := range devs {
		if err := ipr.DeleteLink(ctx, dev); err != nil {
			return err
		}
	}
	return nil
}

// HostapdSecurityConfigIsWEP returns true if the hostapd security configuration
// uses WEP.
func HostapdSecurityConfigIsWEP(secConf security.Config) (bool, error) {
	if secConf == nil {
		return false, nil
	}
	if secConf.Class() == shillconst.SecurityWEP {
		return true, nil
	}
	hostapdConfig, err := secConf.HostapdConfig()
	if err != nil {
		return false, errors.Wrap(err, "failed to build hostapd conf for security config")
	}
	if authAlgsStr, hasAuthAlgs := hostapdConfig["auth_algs"]; hasAuthAlgs {
		authAlgs, err := strconv.Atoi(authAlgsStr)
		if err == nil && (authAlgs == int(wep.AuthAlgoOpen) || authAlgs == int(wep.AuthAlgoShared)) {
			return true, nil
		}
	}
	for configOptionName := range hostapdConfig {
		if strings.Contains(configOptionName, "wep") {
			return true, nil
		}
	}
	return false, nil
}

// TearDownRedundantInterfaces tears down all the interfaces except those in linkList.
func TearDownRedundantInterfaces(ctx context.Context, ipr *ip.Runner, linkList []string) error {
	allLinks, err := ipr.ListUpLinks(ctx)
	if err != nil {
		return err
	}
re:
	for _, link := range allLinks {
		for _, upLink := range linkList {
			if link == upLink {
				continue re
			}
		}
		if err := ipr.SetLinkDown(ctx, link); err != nil {
			return err
		}
	}
	return nil
}
