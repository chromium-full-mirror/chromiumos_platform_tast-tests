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
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/testing"
)

const (
	cleanupTime = 10 * time.Second
	msPerFrame  = 1000.00
	// Time to wait for planets to run before taking metrics.
	sampleWaitTime       = 30 * time.Second
	webGLManyPlanetsDeep = "webgl_many_planets_deep_static.tar.zst"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: WebGLManyPlanetsDeep,
		Desc: "Runs WebGL many planets deep demo from a local build and reports metrics",
		Contacts: []string{
			"chromeos-gfx@google.com",
			"syedfaaiz@google.com",
		},
		// ChromeOS > Platform > Graphics > GPU
		BugComponent: "b:995569",
		Attr:         []string{"graphics_nightly", "group:graphics", "group:mainline", "informational"},
		Data:         []string{webGLManyPlanetsDeep},
		Fixture:      "chromeGraphics",
		Timeout:      2 * time.Minute,
	})
}

func savePerfValue(number float64, name, unit string, pv *perf.Values) {
	direction := perf.SmallerIsBetter
	if name == "avg_fps" {
		direction = perf.BiggerIsBetter
	}
	pv.Set(perf.Metric{
		Name:      name,
		Unit:      unit,
		Direction: direction,
	}, float64(number))
}

func findMean(data []float64) float64 {
	var sum float64
	n := len(data)
	for _, entry := range data {
		sum += entry
	}
	return sum / float64(n)
}

func WebGLManyPlanetsDeep(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	webGLManyPlanetsDeepSrc := s.DataPath(webGLManyPlanetsDeep)
	if err := testexec.CommandContext(ctx, "tar", "-xf", webGLManyPlanetsDeepSrc, "-C", os.TempDir()).Run(testexec.DumpLogOnError); err != nil {
		s.Fatalf("Failed to extract %s", webGLManyPlanetsDeep)
	}
	s.Logf("Extracted %s", webGLManyPlanetsDeep)

	server := httptest.NewServer(http.FileServer(http.Dir(os.TempDir() + "/src")))
	defer server.Close()

	cr := s.FixtValue().(*chrome.Chrome)
	url := path.Join(server.URL, "ManyPlanetsDeep.html")
	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		s.Fatalf("Failed to open %v: %v", url, err)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	if err = conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		s.Fatal("Page failed to load: ", err)
	}

	var bufferSize int
	if err = conn.Eval(ctx, "g_crosFpsCounter.buffer_size", &bufferSize); err != nil {
		s.Fatal("Could not get the frame data buffer size : ", err)
	}

	frameData := make([]map[string]float64, bufferSize)
	var frameTimeData, jsDataTime []float64
	if err = conn.Call(ctx, nil, "g_crosFpsCounter.reset"); err != nil {
		s.Fatal("Could not reset fps counter: ", err)
	}

	if err = testing.Sleep(ctx, sampleWaitTime); err != nil {
		s.Fatalf("Failed to sleep while running planets: %s", err)
	}

	if err = conn.Eval(ctx, "g_crosFpsCounter.getFrameData()", &frameData); err != nil {
		s.Fatal("Could not get the frame data : ", err)
	}

	for index := 0; index < bufferSize; index++ {
		frameTimeData = append(frameTimeData, frameData[index]["frameElapsedTime"])
		jsDataTime = append(jsDataTime, frameData[index]["jsElapsedTime"])
	}
	meanFT := findMean(frameTimeData)
	meanJT := findMean(jsDataTime)
	pv := perf.NewValues()

	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf data: ", err)
		}
	}()

	savePerfValue(msPerFrame/meanFT, "avg_fps", "fps", pv)
	savePerfValue(meanFT, "avg_frame_time", "ms", pv)
	savePerfValue(meanJT, "avg_js_time", "ms", pv)
}
