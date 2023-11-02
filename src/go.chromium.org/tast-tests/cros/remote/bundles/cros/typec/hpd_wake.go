// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/typec/mcci"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// The index of the MCCI port to which the monitor is connected.
const portUsed = 1

func init() {
	testing.AddTest(&testing.Test{
		Func:     HpdWake,
		Desc:     "Checks that Display Port HPD (Hot Plug Detect) events can wake the system",
		Contacts: []string{"chromeos-usb@google.com", "pmalani@chromium.org"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		HardwareDeps: hwdep.D(hwdep.ECFeatureTypecCmd(), hwdep.ChromeEC()),
		Vars:         []string{"typec.McciSerial"},
	})
}

// HpdWake does the following:
// - Disconnect the monitor via MCCI switch.
// - Suspend the DUT.
// - Reconnect the monitor via MCCI switch.
// - Check that the DUT woke, count the EC wake events and confirm that the wake count increased.
//
// This test expects the following hardware topology:
//
//	Host -------- DUT -------- dock ----- MCCI (`portUsed`) ---- display
//	 |                                        |
//	 |________________________________________|
func HpdWake(ctx context.Context, s *testing.State) {
	d := s.DUT()

	sw, err := mcci.GetSwitch(s.RequiredVar("typec.McciSerial"))
	if err != nil {
		s.Fatal("Failed to get MCCI switch handle: ", err)
	}
	defer sw.Close()

	// Disconnect the monitor.
	sw.DisablePorts()

	// GoBigSleepLint: Give enough time for a new display modeset after hot unplug.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to sleep for display unplug modeset: ", err)
	}

	// Count wake sources before.
	wakesBefore, err := getWakeCount(ctx, d)
	if err != nil {
		s.Fatal("Failed to count wakeup sources: ", err)
	}

	// Suspend DUT. Run in a separate thread since this function blocks until resume.
	done := make(chan error, 1)
	go func(ctx context.Context) {
		defer close(done)
		out, err := d.Conn().CommandContext(ctx, "powerd_dbus_suspend", "--timeout=120").CombinedOutput()
		testing.ContextLog(ctx, "powerd_dbus_suspend output: ", string(out))
		done <- err
	}(ctx)

	if err := d.WaitUnreachable(ctx); err != nil {
		s.Fatal("Couldn't verify DUT become unreachable after suspend: ", err)
	}

	// Reconnect the monitor.
	sw.EnablePort(portUsed)

	// Verify DUT reconnected.
	if err := testing.Poll(ctx, d.Connect, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to re-connect to DUT after suspend: ", err)
	}

	// Count wake source.
	wakesAfter, err := getWakeCount(ctx, d)
	if err != nil {
		s.Fatal("Failed to count wakeup sources: ", err)
	}

	// Check the difference between wake sources.
	if wakesAfter <= wakesBefore {
		s.Fatal("Wakeup event not registered")
	}

	// We should make a note if there were other wakeup events for cros_ec.
	if wakesAfter != wakesBefore+1 {
		s.Logf("Registered more than 1 wakeup event, before:%d, after:%d", wakesBefore, wakesAfter)
	}

	// Verify that the suspend command didn't return an unexpected error.
	// TODO(b/197903975): Try to parse actual output of powerd_dbus_suspend.
	if err := <-done; err != nil {
		if !bytes.Contains([]byte(err.Error()), []byte("remote command exited without exit status")) {
			s.Fatal("Suspend command returned unexpected error: ", err)
		}
	}
}

// getWakeCount returns the number of wake ups from the ChromeOS EC device (through which HPD wakeups are passed).
// Uses the wakeup class object attached to the chromeos class cros_ec device in sysfs
func getWakeCount(ctx context.Context, d *dut.DUT) (int64, error) {
	base := "/sys/class/chromeos/cros_ec/"

	link, err := d.Conn().CommandContext(ctx, "readlink", base+"device").Output()
	if err != nil {
		return -1, errors.Wrap(err, "could not find cros_ec device in sysfs")
	}

	ec, err := d.Conn().CommandContext(ctx, "dirname", strings.TrimSpace(string(link))).Output()
	if err != nil {
		return -1, errors.Wrap(err, "could not get dirname of cros_ec device")
	}

	path := base + strings.TrimSpace(string(ec)) + "/wakeup/"
	wakeup, err := d.Conn().CommandContext(ctx, "ls", path).Output()
	if err != nil {
		return -1, errors.Wrap(err, "could not find wakeup class for EC device")
	}

	out, err := d.Conn().CommandContext(ctx, "cat", path+strings.TrimSpace((string(wakeup)))+"/wakeup_count").Output()
	if err != nil {
		return -1, errors.Wrap(err, "could not cat cros_ec wakeup_count on DUT")
	}

	count, err := strconv.ParseInt(strings.TrimSpace(string(out)), 0, 64)
	if err != nil {
		return -1, errors.Wrap(err, "couldn't parse wakeup count for EC device")
	}

	return count, nil
}
