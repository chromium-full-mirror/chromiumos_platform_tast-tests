// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/testing"
)

const (
	waitTimeForBrowser = 5 * time.Second
	// Time to allow fishes to run before recording metrics.
	runFishesFor  = 30 * time.Second
	webGlAquarium = "webgl_aquarium_static_20221212.tar.zst"
)

var (
	commandsMap = map[string]string{
		"avg_fps":             "g_crosFpsCounter.getAvgFps()",
		"avg_interframe_time": "g_crosFpsCounter.getAvgInterFrameTime()",
		"avg_render_time_":    "g_crosFpsCounter.getAvgRenderTime()",
		"std_interframe_time": "g_crosFpsCounter.getStdInterFrameTime()",
	}
	fishSettings = map[int][]string{
		50:   {"'setSetting2'", "2"},
		1000: {"'setSetting6'", "6"},
	}
)

func init() {
	testing.AddTest(&testing.Test{
		Func: WebGLAquarium,
		Desc: "Runs WebGL aquarium demo from a local build and reports metrics",
		Contacts: []string{
			"chromeos-gfx@google.com",
			"syedfaaiz@google.com",
		},
		// ChromeOS > Platform > Graphics > GPU
		BugComponent: "b:995569",
		Attr:         []string{"graphics_perbuild", "group:graphics", "group:mainline", "informational"},
		Timeout:      2 * time.Minute,
		Params: []testing.Param{{
			Name:      "50_fishes",
			Fixture:   "chromeGraphics",
			ExtraData: []string{webGlAquarium},
			Val:       50,
		}, {
			Name:      "1000_fishes",
			Fixture:   "chromeGraphics",
			ExtraData: []string{webGlAquarium},
			Val:       1000,
		}, {
			Name:      "50_fishes_lacros",
			Fixture:   "chromeGraphicsLacros",
			ExtraData: []string{webGlAquarium},
			Val:       50,
		}, {
			Name:      "1000_fishes_lacros",
			Fixture:   "chromeGraphicsLacros",
			ExtraData: []string{webGlAquarium},
			Val:       1000,
		}},
	})
}

func savePerfVal(number float64, name, unit string, pv *perf.Values) {
	direction := perf.SmallerIsBetter
	if name == "avg_fps" {
		direction = perf.BiggerIsBetter
		unit = "fps"
	}
	pv.Set(perf.Metric{
		Name:      name,
		Unit:      unit,
		Direction: direction,
	}, float64(number))
}

func WebGLAquarium(ctx context.Context, s *testing.State) {
	numFish := s.Param().(int)
	webGlAquariumSrc := s.DataPath(webGlAquarium)
	webglLocalDir, err := os.MkdirTemp("", "")
	if err != nil {
		s.Fatal("Failed to created temp dir: ", err)
	}
	defer os.RemoveAll(webglLocalDir)
	if err := testexec.CommandContext(ctx, "tar", "-xf", webGlAquariumSrc, "-C", webglLocalDir).Run(testexec.DumpLogOnError); err != nil {
		s.Logf("Failed to extract %s", webGlAquarium)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(webglLocalDir + "/webgl_aquarium_static")))
	defer server.Close()
	s.Logf("Extracted %s", webGlAquarium)
	cr := s.FixtValue().(*chrome.Chrome)

	url := path.Join(server.URL, "aquarium.html")
	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		s.Fatalf("Failed to open %v: %v", url, err)
	}
	defer conn.Close()
	ctconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	if err = conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		s.Fatal("Page failed to load: ", err)
	}
	elemID := strings.Replace("document.getElementsById(*)", "*", fishSettings[numFish][0], 1)
	if err = conn.Call(ctx, nil, "setSetting", elemID, fishSettings[numFish][1]); err != nil {
		s.Fatal("Could not get the intrinsic fish set id: ", err)
	}
	defer ash.CloseAllWindows(ctx, ctconn)
	if err = conn.Call(ctx, nil, "g_crosFpsCounter.reset"); err != nil {
		s.Fatal("Could not reset the FPS counter: ", err)
	}
	testing.Sleep(ctx, runFishesFor)

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf data: ", err)
		}
	}()
	for metricName, evalCommand := range commandsMap {
		var output float64
		if err = conn.Eval(ctx, evalCommand, &output); err != nil {
			s.Fatalf("Failed while fetching values for metric %s : %s", evalCommand, err)
		}
		savePerfVal(output, metricName, "ms", pv)
	}
}
