// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DozeSuspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that automatic suspending while dozing works",
		Contacts: []string{
			"cros-vm-technology@google.com",
			"stevensd@google.com",
		},
		// ChromeOS > Platform > Virtualization > VM Technology
		BugComponent: "b:930563",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"android_vm", "chrome"},
		Fixture:      "arcBootedS2Idle",
		Timeout:      12 * time.Minute,
	})
}

func dumpsysSuspendControl(ctx context.Context, a *arc.ARC) ([]byte, error) {
	var serviceName string
	n, err := arc.SDKVersion()
	if err != nil {
		return nil, err
	}
	if n == arc.SDKR {
		serviceName = "suspend_control"
	} else {
		serviceName = "suspend_control_internal"
	}

	return a.Command(ctx, "dumpsys", serviceName).Output(testexec.DumpLogOnError)
}

func getSuspendCount(ctx context.Context, a *arc.ARC) (int, error) {
	out, err := dumpsysSuspendControl(ctx, a)
	if err != nil {
		return 0, errors.Wrap(err, "could not get dumpsys output")
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "success: ") {
			parts := strings.Split(line, " ")
			if len(parts) != 2 {
				return 0, errors.Errorf("Malformated line %q", line)
			}
			n, err := strconv.Atoi(parts[1])
			if err != nil {
				return 0, errors.Wrap(err, "failed to parse suspend count")
			}
			return n, nil
		}
	}

	return 0, errors.New("failed to find suspend count")
}

func DozeSuspend(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC

	testing.ContextLog(ctx, "Restarting adbd as root")
	if err := a.Root(ctx); err != nil {
		s.Fatal("Failed to start adb root: ", err)
	}

	s.Log("Waiting for suspend")
	err := testing.Poll(ctx, func(ctx context.Context) error {
		count, err := getSuspendCount(ctx, a)
		if err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		return errors.New("no successful suspend attempts")
	}, &testing.PollOptions{Timeout: 5 * time.Minute, Interval: 5 * time.Second})
	if err != nil {
		s.Error("Failed to wait for suspend: ", err)
		if out, err := dumpsysSuspendControl(ctx, a); err == nil {
			s.Log("Dumping suspend_control to out directory")
			if err := os.WriteFile(filepath.Join(s.OutDir(), "suspend_control.txt"), out, 0644); err != nil {
				s.Error("Failed to save suspend_control dump: ", err)
			}
		} else {
			s.Error("Failed to dump suspend_control: ", err)
		}
	} else {
		s.Log("Observed suspend")
	}
}
