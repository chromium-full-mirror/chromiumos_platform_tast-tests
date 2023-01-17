// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/local/audio"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AloopLoadedFixture,
		Desc:         "Test the AloopLoaded fixture",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:875484",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      1 * time.Minute,
		Fixture:      fixture.AloopLoaded,
	})
}

func AloopLoadedFixture(ctx context.Context, s *testing.State) {
	const (
		aloopModulePath = "/sys/module/snd_aloop/"
		crasAloopType   = "ALSA_LOOPBACK"
	)

	fileInfo, err := os.Stat(aloopModulePath)
	if err != nil {
		s.Fatalf("Failed to stat %s: %v", aloopModulePath, err)
	}
	if !fileInfo.IsDir() {
		s.Fatalf("%s is not a directory", aloopModulePath)
	}

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Cannot connect to CRAS: ", err)
	}
	if _, err := cras.GetNodeByType(ctx, crasAloopType); err != nil {
		s.Error("CRAS alsa loopback device not found: ", err)
	}
}
