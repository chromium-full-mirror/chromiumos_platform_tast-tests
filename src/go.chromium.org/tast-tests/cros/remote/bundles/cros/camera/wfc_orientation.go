// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/common/camera/chart"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	pb "go.chromium.org/tast-tests/cros/services/cros/camerabox"

	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WFCOrientation,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifying the WFC orientation is correct or not",
		Data:         []string{"camerabox_align.js", "opencv.js", "pattern_checker.js", "wfc_orientation.html", "wfc_orientation.css", "wfc_orientation.png", "wfc_pattern.png"},
		Contacts:     []string{"chromeos-camera-eng@google.com", "beckerh@chromium.org", "xinggu@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:camerabox"},
		SoftwareDeps: []string{"chrome", caps.BuiltinCamera},
		ServiceDeps:  []string{"tast.cros.camerabox.OrientationService"},
		Vars:         []string{"chart", "facing"},
		Params: []testing.Param{
			{
				Name:      "back",
				ExtraAttr: []string{"camerabox_facing_back"},
				Val:       pb.Facing_FACING_BACK,
			},
			{
				Name:      "front",
				ExtraAttr: []string{"camerabox_facing_front"},
				Val:       pb.Facing_FACING_FRONT,
			},
		},
	})
}

func WFCOrientation(ctx context.Context, s *testing.State) {
	facing := s.Param().(pb.Facing)

	d := s.DUT()
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to WFC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Prepare data path on DUT.
	tempdir, err := d.Conn().CommandContext(ctx, "mktemp", "-d", "/tmp/camerabox_align_XXXXXX").Output()
	dataPath := strings.TrimSpace(string(tempdir))
	defer d.Conn().CommandContext(ctx, "rm", "-r", dataPath).Output()
	if _, err := linuxssh.PutFiles(
		ctx, d.Conn(), map[string]string{
			s.DataPath("wfc_orientation.html"): filepath.Join(dataPath, "wfc_orientation.html"),
			s.DataPath("wfc_orientation.css"):  filepath.Join(dataPath, "wfc_orientation.css"),
			s.DataPath("wfc_orientation.png"):  filepath.Join(dataPath, "wfc_orientation.png"),
			s.DataPath("wfc_pattern.png"):      filepath.Join(dataPath, "wfc_pattern.png"),
			s.DataPath("camerabox_align.js"):   filepath.Join(dataPath, "camerabox_align.js"),
			s.DataPath("pattern_checker.js"):   filepath.Join(dataPath, "pattern_checker.js"),
			s.DataPath("opencv.js"):            filepath.Join(dataPath, "opencv.js"),
		},
		linuxssh.DereferenceSymlinks); err != nil {
		s.Fatalf("Failed to send data to remote data path %v: %v", dataPath, err)
	}

	defer d.Conn().CommandContext(ctx, "rm", "-r", dataPath).Output()

	var chartAddr string
	if altAddr, ok := s.Var("chart"); ok {
		chartAddr = altAddr
	}
	c, namePaths, err := chart.New(ctx, s.DUT(), chartAddr, s.OutDir(), []string{s.DataPath("wfc_orientation.png")})
	if err != nil {
		s.Fatal("Failed to prepare chart tablet: ", err)
	}
	defer c.Close(ctx, s.OutDir())
	if err := c.Display(ctx, namePaths[0]); err != nil {
		s.Fatal("Failed to display chart on chart tablet: ", err)
	}

	acl := pb.NewOrientationServiceClient(cl.Conn)
	response, err := acl.CheckOrientation(ctx, &pb.CheckOrientationRequest{
		DataPath: dataPath,
		Facing:   facing,
	})
	if err != nil {
		s.Fatal("Remote call CheckOrientation() failed: ", err)
	}
	if response.Result != pb.TestResult_TEST_RESULT_PASSED {
		s.Error("Orientation check failed: ", response.Error)
	}

	s.Log("Passed all alignment checks")
}
