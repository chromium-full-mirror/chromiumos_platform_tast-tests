// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"chromiumos/tast/local/graphics"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: IgtChamelium,
		Desc: "Verifies IGT Chamelium test binaries run successfully",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"markyacoub@google.com",
		},
		// ChromeOS > Platform > Graphics > Display
		BugComponent: "b:188154",
		SoftwareDeps: []string{"drm_atomic", "igt", "no_qemu"},
		VarDeps:      []string{"graphics.chameleon_ip"},
		Attr:         []string{"group:graphics", "graphics_chameleon_igt"},
		Fixture:      "chromeGraphicsIgt",
		Params: []testing.Param{{
			Name: "kms_chamelium_color",
			Val: graphics.IgtTest{
				Exe: "kms_chamelium_color",
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"graphics_nightly"},
		}, {
			Name: "kms_chamelium_edid",
			Val: graphics.IgtTest{
				Exe: "kms_chamelium_edid",
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"graphics_nightly"},
		}, {
			Name: "kms_chamelium_frames",
			Val: graphics.IgtTest{
				Exe: "kms_chamelium_frames",
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"graphics_nightly"},
		}, {
			Name: "kms_chamelium_hpd",
			Val: graphics.IgtTest{
				Exe: "kms_chamelium_hpd",
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"graphics_nightly"},
		}},
	})
}

func setIgtrcFile(s *testing.State) {
	igtFilePath := "/tmp/.igtrc"
	igtFile, err := os.Create(igtFilePath)
	if err != nil {
		s.Fatal("Failed to create .igtrc: ", err)
	}
	defer igtFile.Close()

	// Get Chameleon IP
	// This is used for local dev env.
	url, err := graphics.ChameleonGetURL()
	if err != nil {
		s.Fatal("Failed to get the Chameleon URL: ", err)
	}

	content := `
[Common]
# The path to dump frames that fail comparison checks
FrameDumpPath=/tmp

[DUT]
SuspendResumeDelay=15

[Chamelium]
URL=` + url + `

`

	// Write config content to .igtrc
	_, err = igtFile.WriteString(content)
	if err != nil {
		s.Fatal("Failed to write to igtrc: ", err)
		return
	}

	// Set the file path as env variable for IGT to find it.
	os.Setenv("IGT_CONFIG_PATH", igtFilePath)

	s.Log("$IGT_CONFIG_PATH = ", igtFilePath)
	s.Log("Chameleon Device URL = ", url)
}

func IgtChamelium(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(graphics.IgtTest)
	f, err := os.Create(filepath.Join(s.OutDir(), filepath.Base(testOpt.Exe)+".txt"))
	if err != nil {
		s.Fatal("Failed to create a log file: ", err)
	}
	defer f.Close()

	setIgtrcFile(s)

	isExitErr, exitErr, err := graphics.IgtExecuteTests(ctx, testOpt.Exe, f)

	isError, outputLog := graphics.IgtProcessResults(testOpt.Exe, f, isExitErr, exitErr, err)

	if isError {
		s.Error(outputLog)
	} else {
		s.Log(outputLog)
	}
}
