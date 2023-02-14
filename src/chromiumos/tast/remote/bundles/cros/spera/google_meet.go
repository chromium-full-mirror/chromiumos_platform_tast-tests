// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"strconv"
	"time"

	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/remote/bundles/cros/spera/conference"
	"chromiumos/tast/rpc"
	pb "chromiumos/tast/services/cros/spera"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GoogleMeet,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Host a Google Meet video conference and do presentation to participants",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "chicheny@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome", caps.BuiltinOrVividCamera},
		ServiceDeps: []string{
			"tast.cros.spera.ConferenceService2",
		},
		Data: []string{conference.CameraVideo, conference.TraceConfigFile},
		Vars: []string{
			"spera.use_real_camera",
			"spera.collectTrace", // Optional. Expecting "enable" or "disable", default is "disable".
		},
		Params: []testing.Param{
			{
				Name:    "grid_essential",
				Timeout: 50*time.Minute + conference.CPUIdleTimeout,
				Val: conference.TestParameters{
					Tier:     conference.Essential,
					RoomType: conference.LargeRoomSize,
				},
			},
			{
				Name:    "grid_advanced",
				Timeout: 50*time.Minute + conference.CPUIdleTimeout,
				Val: conference.TestParameters{
					Tier:     conference.Advanced,
					RoomType: conference.LargeRoomSize,
				},
			},
			{
				Name:    "class_essential",
				Timeout: 50*time.Minute + conference.CPUIdleTimeout,
				Val: conference.TestParameters{
					Tier:     conference.Essential,
					RoomType: conference.ClassRoomSize,
				},
			},
			{
				Name:    "class_advanced",
				Timeout: 50*time.Minute + conference.CPUIdleTimeout,
				Val: conference.TestParameters{
					Tier:     conference.Advanced,
					RoomType: conference.ClassRoomSize,
				},
			},
			{
				Name:              "grid_essential_lacros",
				Timeout:           50*time.Minute + conference.CPUIdleTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:     conference.Essential,
					RoomType: conference.LargeRoomSize,
					IsLacros: true,
				},
			},
			{
				Name:              "grid_advanced_lacros",
				Timeout:           50*time.Minute + conference.CPUIdleTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:     conference.Advanced,
					RoomType: conference.LargeRoomSize,
					IsLacros: true,
				},
			},
			{
				Name:              "class_essential_lacros",
				Timeout:           50*time.Minute + conference.CPUIdleTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:     conference.Essential,
					RoomType: conference.ClassRoomSize,
					IsLacros: true,
				},
			},
			{
				Name:              "class_advanced_lacros",
				Timeout:           50*time.Minute + conference.CPUIdleTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:     conference.Advanced,
					RoomType: conference.ClassRoomSize,
					IsLacros: true,
				},
			},
		},
	})
}

func GoogleMeet(ctx context.Context, s *testing.State) {
	param := s.Param().(conference.TestParameters)

	dut := s.DUT()
	c, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to dial to remote dut: ", err)
	}
	defer c.Close(ctx)
	var remoteCameraVideoPath string
	var useRealCamera bool // Default is false.
	if val, ok := s.Var("spera.use_real_camera"); ok {
		useRealCamera, err = strconv.ParseBool(val)
		if err != nil {
			s.Fatal("Unable to convert spera.use_real_camera var to bool: ", err)
		}
	}
	// Use fake camera by default.
	if !useRealCamera {
		remoteCameraVideoPath, err = conference.PushFileToTmpDir(ctx, s, dut, conference.CameraVideo)
		if err != nil {
			s.Fatal("Failed to push file to DUT's tmp directory: ", err)
		}
		defer dut.Conn().CommandContext(ctx, "rm", remoteCameraVideoPath).Run()
	}

	if collect, ok := s.Var("spera.collectTrace"); ok && collect == "enable" {
		remoteTraceConfigFilePath, err := conference.PushFileToTmpDir(ctx, s, dut, conference.TraceConfigFile)
		if err != nil {
			s.Fatal("Failed to push file to DUT's tmp directory: ", err)
		}
		defer dut.Conn().CommandContext(ctx, "rm", remoteTraceConfigFilePath).Run()
	}
	client := pb.NewConferenceService2Client(c.Conn)
	if _, err := client.RunGoogleMeetScenario(ctx, &pb.MeetScenarioRequest{
		Tier:            int64(param.Tier),
		RoomType:        int64(param.RoomType),
		ExtendedDisplay: false,
		CameraVideoPath: remoteCameraVideoPath,
		IsLacros:        param.IsLacros,
	}); err != nil {
		s.Fatal("Failed to run Google Meet testing: ", err)
	}
}
