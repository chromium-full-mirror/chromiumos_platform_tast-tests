// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fwupd

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/godbus/dbus/v5"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
	"gopkg.in/ini.v1"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:         "ensureRemotes",
		Desc:         "Check if LVFS and LVFS Testing are available and do metadata refresh",
		Contacts:     []string{"chromeos-fwupd@google.com"},
		Impl:         &dfuFixture{},
		SetUpTimeout: 60 * time.Second,
	})
}

type dfuFixture struct {
}

func (dfu *dfuFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Don't close the shared connection
	conn, err := dbusutil.SystemBus()
	if err != nil {
		s.Fatal("Failed to connect to system bus: ", err)
	}
	fwupd := conn.Object(DbusName, DbusPath)

	var remotes []map[string]dbus.Variant
	if err := fwupd.Call(DbusInterface+".GetRemotes", 0).Store(&remotes); err != nil {
		s.Fatal("Failed to find remotes: ", err)
	}

	// Firmware update/downgrade tests requires LVFS and LVFS Testing remotes
	// to download needed firmware version.
	if err := ensureTestRemote(remotes, "lvfs"); err != nil {
		s.Fatal("LVFS remote is not enabled: ", err)
	}
	if err := ensureTestRemote(remotes, "lvfs-testing"); err != nil {
		// Let's try with LVFS only.
		s.Log("LVFS Testing remote is not enabled: ", err)
	}

	cmd := testexec.CommandContext(ctx, "/usr/bin/fwupdmgr", "refresh", "--json")
	if output, err := cmd.Output(testexec.DumpLogOnError); err != nil {
		// Refresh could fail -- just log the error.
		s.Logf("Metadata refresh failed: %q: %v", shutil.EscapeSlice(cmd.Args), err)
		if err := os.WriteFile(filepath.Join(s.OutDir(), "fwupd-refresh.txt"), output, 0644); err != nil {
			s.Logf("Failed dumping output for refresh command: %q, with output: %s", err, output)
		}
	}
	return nil
}

func (dfu *dfuFixture) Reset(ctx context.Context) error {
	return nil
}

func (dfu *dfuFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (dfu *dfuFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (dfu *dfuFixture) TearDown(ctx context.Context, s *testing.FixtState) {}

// ensureTestRemote checks remotes are enabled and enable them if needed.
// Not fwupd nor test are able to change the original remote since it is on R/O storage.
// Ensures the remote passed is enabled, otherwise check if test remote based on
// original one already exists. If not it tries to create the remote "test-<remote>"
// in /var/lib/fwupd/remotes.d based on original variant and enable it.
func ensureTestRemote(remotes []map[string]dbus.Variant, origRemote string) (err error) {

	const targetDir = "/var/lib/fwupd/remotes.d"
	testRemote := "test-" + origRemote

	if idx := findRemoteByID(remotes, testRemote); idx != -1 {
		return nil
	}

	idx := findRemoteByID(remotes, origRemote)
	if idx == -1 {
		return errors.New("Unable to find remote " + origRemote)
	}

	remoteDesc := struct {
		enabled     bool
		title       string
		metadataURI string
		reportURI   string
	}{}
	rawRemoteFields := []interface{}{
		remotes[idx]["Enabled"],
		remotes[idx]["Title"],
		remotes[idx]["Uri"],
		remotes[idx]["ReportUri"],
	}
	remoteFields := []interface{}{
		&remoteDesc.enabled,
		&remoteDesc.title,
		&remoteDesc.metadataURI,
		&remoteDesc.reportURI,
	}

	if err := dbus.Store(rawRemoteFields, remoteFields...); err != nil {
		return errors.Wrap(err, "failed to parse remote")
	}

	if remoteDesc.enabled == true {
		// Do not need the new one since original remote is enabled.
		return nil
	}

	config := ini.Empty()
	section, err := config.NewSection("fwupd Remote")
	if err != nil {
		return err
	}

	kvDesc := map[string]string{
		"Enabled":     "true",
		"Title":       remoteDesc.title,
		"MetadataURI": remoteDesc.metadataURI,
		"ReportURI":   remoteDesc.reportURI,
		"OrderBefore": origRemote,
	}
	for k, v := range kvDesc {
		if _, err := section.NewKey(k, v); err != nil {
			return err
		}
	}

	if err := config.SaveTo(path.Join(targetDir, testRemote+".conf")); err != nil {
		return err
	}

	return nil
}

func findRemoteByID(remotes []map[string]dbus.Variant, remoteID string) int {
	for i, remote := range remotes {
		var id string
		// Ignore failed.
		dbus.Store([]interface{}{remote["RemoteId"]}, &id)
		if id == remoteID {
			return i
		}
	}
	return -1
}
