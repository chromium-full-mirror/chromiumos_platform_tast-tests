// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbutils

import (
	"context"
	"io/ioutil"
	"regexp"
	"time"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// DisplaySpec contains details of the display connected to the DUT.
type DisplaySpec struct {
	NumberOfDisplays *int   // Number of displays connected.
	DisplayType      string // Display Type as HDMI or DP.
	DisplayRes       string // Display resolution.
	HDCPVer          string // HDCP Version.
}

// List of Display Type constants.
const (
	NativeHDMI = "NativeHDMI"
	TypeCHDMI  = "TypeCHDMI"
	NativeDP   = "NativeDP"
	TBTDisplay = "TBTDisplay"
	TypeCDP    = "TypeCDP"
)

// externalDisplayDetection verifies connected extended display is detected or not.
func externalDisplayDetection(ctx context.Context, dut *dut.DUT, remoteTest bool, spec DisplaySpec) error {
	const displayInfoFile = "/sys/kernel/debug/dri/0/i915_display_info"

	/*displayInfo is used to find display detection by mathcing pattern like below in pipe B/C/D
				[CRTC:131:pipe B]:
			        uapi: enable=yes, active=yes, mode="3840x2160": 144 1278720 3840 3952 3984 4000 2160 2210 2215 2220 0x48 0x9
			        hw: enable=yes, active=yes
	 disModes is used to find disply disply modes by matching patterns like
				 modes:
			                "3840x2160": 60
	 in displyCon:
				NativeHDMI will match : [CONNECTOR:245:HDMI-A-1]: status: connected
				TypeCHDMI will match:	[CONNECTOR:206:DP-2]: status: connected
		        						physical dimensions: 600x340mm
		        						subpixel order: Unknown
		       							CEA rev: 3
		        						DPCD rev: 12
		        						audio support: yes
		        						DP branch device present: yes
				TypeCDP will match:		[CONNECTOR:264:DP-2]: status: connected
	        							physical dimensions: 620x340mm
	       								subpixel order: Unknown
	       								CEA rev: 3
	        							DPCD rev: 14
	        							audio support: yes
	        							DP branch device present: no
				TBTDisplay will match:  [CONNECTOR:264:DP-2]: status: connected
	        							physical dimensions: 620x340mm
	       								subpixel order: Unknown
	       								CEA rev: 3
	        							DPCD rev: 14
	        							audio support: yes
	        							DP branch device present: no
				NativeDP will match: [CONNECTOR:264:DP-2]: status: connected
	 in hdcp:
	  			HDCP1.4 will match HDCP1.4
	  			HDCP2.2 will match HDCP2.2
	 in displayResolution:
	 			FullHD: 1920x1080
				2K:     2048x1080|2560x1440
				4K:     3840x2160|4096x2160
				5K:     5120x2880
				6K:     6144x3456
				8K:     7680x4320

	*/
	displayInfo := regexp.MustCompile(`.*pipe\s+[BCD]\]:\n.*active=yes, mode=.[0-9]+x[0-9]+.: [0-9]+.*\s+[hw: active=yes]+`)
	disModes := regexp.MustCompile(`modes:\n.*"\d+x\d+":.60`)
	displayCon := map[string]*regexp.Regexp{
		NativeHDMI: regexp.MustCompile(`(?i)\[CONNECTOR:[0-9]+:HDMI-A-[0-9]+\]: status: connected`),
		TypeCHDMI:  regexp.MustCompile(`(?i)\[CONNECTOR:[0-9]+:DP-[0-9]+\]: status: connected[\s\S]+DP branch device present: yes[\s\S]+Type: HDMI`),
		TypeCDP:    regexp.MustCompile(`(?i)\[CONNECTOR:[0-9]+:DP-[0-9]+\]: status: connected[\s\S]+DP branch device present: no`),
		TBTDisplay: regexp.MustCompile(`(?i)\[CONNECTOR:[0-9]+:DP-[0-9]+\]: status: connected[\s\S]+DP branch device present: no`),
		NativeDP:   regexp.MustCompile(`(?i)\[CONNECTOR:[0-9]+:DP-[0-9]+\]: status: connected`),
	}
	hdcp := map[string]*regexp.Regexp{
		"HDCP1.4": regexp.MustCompile("HDCP1.4"),
		"HDCP2.2": regexp.MustCompile("HDCP2.2"),
	}

	displayResolution := map[string]*regexp.Regexp{
		"FullHD": regexp.MustCompile("1920x1080"),
		"2K":     regexp.MustCompile("2048x1080|2560x1440"),
		"4K":     regexp.MustCompile("3840x2160|4096x2160"),
		"5K":     regexp.MustCompile("5120x2880"),
		"6K":     regexp.MustCompile("6144x3456"),
		"8K":     regexp.MustCompile("7680x4320"),
	}

	var out []byte
	var err error
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if remoteTest {
			out, err = linuxssh.ReadFile(ctx, dut.Conn(), displayInfoFile)
		} else {
			out, err = ioutil.ReadFile(displayInfoFile)
		}
		if err != nil {
			return errors.Wrap(err, "failed to run display info command")
		}
		if !(disModes).MatchString(string(out)) {
			return errors.Errorf("failed %q error message", displayCon["Modes"])
		}
		matchedString := displayInfo.FindAllString(string(out), -1)

		if !(len(matchedString) >= *spec.NumberOfDisplays) {
			return errors.New("connected external display info not found")
		}
		if spec.DisplayType != "" {
			if reg, ok := displayCon[spec.DisplayType]; ok && !reg.MatchString(string(out)) {
				return errors.Errorf("failed %q error message", displayCon[spec.DisplayType])
			}
		}
		if spec.DisplayRes != "" {
			if reg, ok := displayResolution[spec.DisplayRes]; ok && !reg.MatchString(string(out)) {
				return errors.Errorf("failed %q error message", displayResolution[spec.DisplayRes])
			}
		}
		if spec.HDCPVer != "" {
			if reg, ok := hdcp[spec.HDCPVer]; ok && !reg.MatchString(string(out)) {
				return errors.Errorf("failed %q error message", hdcp[spec.HDCPVer])
			}
		}

		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		return errors.Wrap(err, "please connect external display as required")
	}
	return nil
}

// ExternalDisplayDetectionForLocal verifies connected extended display is detected or not for local tests.
func ExternalDisplayDetectionForLocal(ctx context.Context, spec DisplaySpec) error {
	return externalDisplayDetection(ctx, nil, false, spec)
}

// ExternalDisplayDetectionForRemote verifies connected extended display is detected or not for remote tests.
func ExternalDisplayDetectionForRemote(ctx context.Context, dut *dut.DUT, spec DisplaySpec) error {
	return externalDisplayDetection(ctx, dut, true, spec)
}
