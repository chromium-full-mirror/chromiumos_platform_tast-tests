// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/testing"
	"go.chromium.org/tast/core/shutil"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MinijailLandlock,
		Desc: "Verifies minijail0's Landlock enforcement",
		Contacts: []string{
			"minijail-dev@google.com",
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Minijail
		BugComponent: "b:1099158",
		SoftwareDeps: []string{"landlock_enabled"},
		Attr:         []string{"group:mainline"},
	})
}

func MinijailLandlock(ctx context.Context, s *testing.State) {
	const (
		minijailPath     = "/sbin/minijail0"
		minijailTastFlag = "--fs-path-rx=/usr/local/libexec/tast"
		exitSuccess      = 0
	)
	landlockArgs := []string{"--fs-default-paths", minijailTastFlag}
	profileArgs := []string{"--profile=minimalistic-mountns", minijailTastFlag}

	for _, tc := range []struct {
		name          string   // human-readable test case name
		cmd           []string // cmd and arguments for minijail to run
		args          []string // minijail0-specific args
		expectSuccess bool     // true if the cmd should return 0
	}{
		{
			"landlock-allow-nonzero-return",
			[]string{"/bin/false"},
			landlockArgs,
			false,
		},
		{
			"landlock-deny-disallowed-path",
			[]string{"/bin/ls", "/tmp"},
			landlockArgs,
			false,
		},
		{
			"landlock-allow-profile",
			[]string{"/bin/touch", "/tmp/foo"},
			append(profileArgs, "--enable-profile-fs-restrictions"),
			true,
		},
		{
			"landlock-deny-profile",
			[]string{"/bin/touch /tmp/bar"},
			profileArgs,
			false,
		},
	} {
		if ctx.Err() != nil {
			s.Error("Aborting testing: ", ctx.Err())
			break
		}
		var args []string
		args = append(args, tc.args...)
		args = append(args, tc.cmd...)
		cmd := testexec.CommandContext(ctx, minijailPath, args...)
		cmdStr := shutil.EscapeSlice(cmd.Args)
		s.Logf("Running %q: %v", tc.name, cmdStr)
		err := cmd.Run()

		if st, ok := testexec.GetWaitStatus(err); !ok {
			s.Errorf("Case %q (%v) failed (no exit status): %v", tc.name, cmdStr, err)
			cmd.DumpLog(ctx)
		} else if tc.expectSuccess && st.ExitStatus() != exitSuccess {
			s.Errorf("Case %q (%v) exited with %d; want zero", tc.name, cmdStr, st.ExitStatus())
			cmd.DumpLog(ctx)
		} else if !tc.expectSuccess && st.ExitStatus() == exitSuccess {
			s.Errorf("Case %q (%v) exited with %d; want nonzero", tc.name, cmdStr, st.ExitStatus())
			cmd.DumpLog(ctx)
		}
	}
}
