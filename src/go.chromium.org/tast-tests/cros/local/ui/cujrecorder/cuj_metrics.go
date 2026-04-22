// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cujrecorder

import (
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/power/metrics"
)

// CUJAshCommonMetricConfigs returns metrics which are required to be
// collected by all CUJ tests from the Ash process only. This function
// automatically retrieves all SPERA metrics defined in
// AshCommonMetricConfigs and adds more.
func CUJAshCommonMetricConfigs() []MetricConfig {
	return append(AshCommonMetricConfigs(),
		// Memory Metrics.
		NewCustomMetricConfig("ChromeOS.CWP.PSIMemPressure.Full", "percentx100", perf.SmallerIsBetter),
		NewCustomMetricConfig("ChromeOS.CWP.PSIMemPressure.Some", "percentx100", perf.SmallerIsBetter),
		NewCustomMetricConfig("ChromeOS.Zram.ComprDataSizeMB", "MB", perf.SmallerIsBetter),
		NewCustomMetricConfig("ChromeOS.Zram.CompressedSizePct", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Platform.SwapInDaily", "pages", perf.SmallerIsBetter),
		NewCustomMetricConfig("Platform.SwapOutDaily", "pages", perf.SmallerIsBetter),
		NewCustomMetricConfig("Memory.Browser.PrivateMemoryFootprint", "MB", perf.SmallerIsBetter),
		NewCustomMetricConfig("Memory.OOMKills.Daily", "kills", perf.SmallerIsBetter),
		NewCustomMetricConfig("Memory.PressureLevel2", "level", perf.SmallerIsBetter),

		// Smoothness.
		NewCustomMetricConfig("Ash.Smoothness.PercentDroppedFrames_1sWindow.InSession2", "percent", perf.SmallerIsBetter),
		NewDistributionMetricConfig("Graphics.Smoothness.MaxPercentDroppedFrames_1sWindow", "percent", perf.SmallerIsBetter),
		NewDistributionMetricConfig("Graphics.Smoothness.Jank3.AllSequences", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.CheckerboardingNeedRaster4.AllSequences", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.CheckerboardingNeedRecord4.AllSequences", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.AllSequences", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.CanvasAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.CompositorAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.JSAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.MainThreadAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.PinchZoom", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.RAF", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.ScrollbarScroll", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.SETCompositorAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.SETMainThreadAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.TouchScroll", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.Video", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.CompositorThread.WheelScroll", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.CanvasAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.CompositorAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.JSAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.MainThreadAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.PinchZoom", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.RAF", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.ScrollbarScroll", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.SETCompositorAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.SETMainThreadAnimation", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.TouchScroll", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.Video", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Smoothness.Checkerboarding4.MainThread.WheelScroll", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Exo.Smoothness.DidNotProduceToFrameArrival", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Exo.Smoothness.PercentDidNotProduceFrame", "percent", perf.SmallerIsBetter),
		NewCustomMetricConfig("Graphics.Exo.Smoothness.PercentFrameDiscarded", "percent", perf.SmallerIsBetter),
		NewSmoothnessMetricConfig("Apps.HomeLauncherTransition.AnimationSmoothness.EnterFullscreenAllApps"),
		NewSmoothnessMetricConfig("Apps.HomeLauncherTransition.AnimationSmoothness.EnterFullscreenSearch"),
		NewSmoothnessMetricConfig("Apps.HomeLauncherTransition.AnimationSmoothness.FadeInOverview"),
		NewSmoothnessMetricConfig("Apps.HomeLauncherTransition.AnimationSmoothness.FadeOutOverview"),
		NewSmoothnessMetricConfig("Apps.HomeLauncherTransition.AnimationSmoothness.HideLauncherForWindow"),
		NewSmoothnessMetricConfig("Apps.StateTransition.AnimationSmoothness.FullscreenSearch.ClamshellMode"),

		// ARC App Kill Metrics.
		NewCustomMetricConfig("Arc.App.LowMemoryKills.LMKD.CachedCount10Minutes", "apps", perf.SmallerIsBetter),
		NewCustomMetricConfig("Arc.App.LowMemoryKills.LMKD.ForegroundCount10Minutes", "apps", perf.SmallerIsBetter),
		NewCustomMetricConfig("Arc.App.LowMemoryKills.LMKD.PerceptibleCount10Minutes", "apps", perf.SmallerIsBetter),
		NewCustomMetricConfig("Arc.App.LowMemoryKills.LinuxOOMCount10Minutes", "apps", perf.SmallerIsBetter),
		NewCustomMetricConfig("Arc.App.LowMemoryKills.Pressure.CachedCount10Minutes", "apps", perf.SmallerIsBetter),
		NewCustomMetricConfig("Arc.App.LowMemoryKills.Pressure.ForegroundCount10Minutes", "apps", perf.SmallerIsBetter),
		NewCustomMetricConfig("Arc.App.LowMemoryKills.Pressure.PerceptibleCount10Minutes", "apps", perf.SmallerIsBetter),

		// Tab Discard Metrics.
		NewCustomMetricConfig("Discarding.DailyDiscards.Urgent", "tabs", perf.SmallerIsBetter),
		NewCustomMetricConfig("Discarding.DailyReloads.Urgent", "tabs", perf.SmallerIsBetter),
		NewCumulativeMetricConfig("TabManager.Discarding.DiscardCount", "tabs", perf.SmallerIsBetter),

		// Desk Metrics.
		NewDistributionMetricConfig("Ash.Desks.AnimationLatency.DeskActivation", "ms", perf.SmallerIsBetter),
		NewSmoothnessMetricConfig("Ash.Desks.AnimationSmoothness.DeskActivation"),
		NewEnumCustomMetricConfig("Ash.Desks.DesksSwitchScreenshotResult",
			map[int64]string{
				0: "Failure",
				1: "Success"},
			nil),

		// Other metrics to monitor.
		NewLatencyMetricConfig("Ash.DragWindowFromShelf.PresentationTime"),
		NewLatencyMetricConfig("Ash.HotseatTransition.Drag.PresentationTime"),
		NewSmoothnessMetricConfig("Ash.Homescreen.AnimationSmoothness"),
		NewSmoothnessMetricConfig("Ash.HotseatTransition.AnimationSmoothness.TransitionToExtendedHotseat"),
		NewSmoothnessMetricConfig("Ash.HotseatTransition.AnimationSmoothness.TransitionToHiddenHotseat"),
		NewSmoothnessMetricConfig("Ash.HotseatTransition.AnimationSmoothness.TransitionToShownHotseat"),
		NewSmoothnessMetricConfig("Ash.SwipeHomeToOverviewGesture"),
		NewCustomMetricConfig("Graphics.Paint.UI.NormalizedInvalidatedArea", "area", perf.SmallerIsBetter),
		NewCustomMetricConfig("Compositing.DirectRenderer.PartialSwap.TotalDamage", "area", perf.SmallerIsBetter),
		NewCustomMetricConfig("Compositing.DirectRenderer.PartialSwap.ExtraDamage", "area", perf.SmallerIsBetter),

		// Note that BootTime.* metrics have special handling in cujRecorder.Record().
		NewBootAndShutdownCustomMetricConfig("BootTime.Authenticate", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("BootTime.Chrome", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("BootTime.Firmware", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("BootTime.Kernel", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("BootTime.Login2", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("BootTime.LoginNewUser", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("BootTime.System", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("BootTime.Total2", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("ShutdownTime.BrowserDeleted", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("ShutdownTime.Logout", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("ShutdownTime.Restart", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("ShutdownTime.UIMessageLoopEnded", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("Ash.Tast.ArcUiAvailableAfterLogin.Duration", "ms", perf.SmallerIsBetter),
		NewBootAndShutdownCustomMetricConfig("Arc.Tast.UiAvailable.TimeDelta", "ms", perf.SmallerIsBetter),

		// Event Latency Metrics.
		NewDistributionMetricConfig("Ash.EventLatency.TotalLatency", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.Core.TotalLatency", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.FirstGestureScrollUpdate.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureDoubleTap.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureLongPress.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureLongTap.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GesturePinchBegin.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GesturePinchEnd.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GesturePinchUpdate.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureScrollBegin.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureScrollEnd.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureScrollUpdate.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureShowPress.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureTap.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureTapCancel.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureTapDown.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureTapUnconfirmed.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.GestureTwoFingerTap.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.InertialGestureScrollUpdate.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewDistributionMetricConfig("Ash.EventLatency.KeyPressed.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.KeyReleased.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.MouseDragged.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.MouseMoved.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewDistributionMetricConfig("Ash.EventLatency.MousePressed.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.MouseReleased.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.MouseWheel.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.TouchMoved.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.TouchPressed.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.EventLatency.TouchReleased.TotalLatency", "microseconds", perf.SmallerIsBetter),

		// GPU metrics.
		NewCustomMetricConfig("Compositing.Display.PendingSwaps", "swaps", perf.SmallerIsBetter),
		NewCustomMetricConfig("Compositing.Display.OverlayProcessorUsingStrategy.NumOverlaysPromoted", "overlay_candidates", perf.BiggerIsBetter),
		NewCustomMetricConfig("Compositing.Display.OverlayProcessorUsingStrategy.NumOverlaysFailed", "overlay_candidates", perf.SmallerIsBetter),
		NewCustomMetricConfig("Compositing.Display.OverlayProcessorUsingStrategy.NumOverlaysAttempted", "overlay_candidates", perf.BiggerIsBetter),
		NewCustomMetricConfig("Viz.FrameSink.GpuBusyDuration", "microseconds", perf.SmallerIsBetter),
		NewEnumCustomMetricConfig("Compositing.Display.HardwareDisplayController.SchedulePageFlipResult",
			map[int64]string{
				0: "Success",
				1: "FailedPlaneAssignment",
				2: "FailedCommit"},
			map[int64]perf.Direction{
				0: perf.BiggerIsBetter,
			},
		),
		NewEnumCustomMetricConfig(metrics.OverlayStrategyMetric,
			metrics.OverlayStrategyVariants,
			nil),
	)
}

// CUJLacrosCommonMetricConfigs returns metrics which are required to
// be collected by all CUJ tests from the Lacros process only. This
// function automatically retrieves all SPERA metrics defined in
// LacrosCommonMetricConfigs and adds more.
func CUJLacrosCommonMetricConfigs() []MetricConfig {
	return LacrosCommonMetricConfigs()
}

// CUJBrowserCommonMetricConfigs returns metrics which are required to
// be collected by all CUJ tests from the browser process only (Ash or
// Lacros). This function automatically retrieves all SPERA metrics
// defined in BrowserCommonMetricConfigs and adds more.
func CUJBrowserCommonMetricConfigs() []MetricConfig {
	return append(BrowserCommonMetricConfigs(),
		// Other metrics to monitor.
		NewCustomMetricConfig("Media.DroppedFrameCount", "count", perf.SmallerIsBetter),
		NewCustomMetricConfig("PageLoad.InteractiveTiming.FirstInputDelay4", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("SessionRestore.ForegroundTabFirstPaint4", "ms", perf.SmallerIsBetter),
	)
}

// CUJAnyChromeCommonMetricConfigs returns metrics which are required to
// be collected by CUJ tests from any Chrome binary running (could be
// from both Ash and Lacros in parallel). This function automatically
// retrieves all SPERA metrics defined in AnyChromeCommonMetricConfigs
// and adds more. The metrics would be reported as combined in the original
// metrics names, and separately for each browser type under the original
// name + browser type suffix.
func CUJAnyChromeCommonMetricConfigs() []MetricConfig {
	return append(AnyChromeCommonMetricConfigs(),
		NewMemoryMetricConfig("Memory.Gpu.PrivateMemoryFootprint", "MB"),
		NewMemoryMetricConfig("Memory.Total.TileMemory", "MB"),
		NewMemoryMetricConfig("Memory.Experimental.Gpu2.SharedImages", "MB"),
		NewMemoryMetricConfig("Memory.Experimental.Gpu2.SharedImages.NonExo", "MB"),
		NewMemoryMetricConfig("Memory.Experimental.Gpu2.GpuMemory", "MB"),
		NewMemoryMetricConfig("Memory.Experimental.Gpu2.GpuMemoryNonExo", "MB"),
	)
}

// GetShortenedPerformanceMetrics returns a list of Ash metrics and a list
// of browser metrics that are highly important in tracking the performance
// of a device.
func GetShortenedPerformanceMetrics() ([]string, []string) {
	return []string{
			// Ash Metrics.

			// Smoothness.
			"Ash.Smoothness.PercentDroppedFrames_1sWindow2",
		}, []string{
			// Browser metrics.

			// Blink input latency.
			"EventLatency.KeyPressed.TotalLatency",
			"EventLatency.TotalLatency",
			"EventLatency.MousePressed.TotalLatency",

			// Graphics smoothness.
			"Graphics.Smoothness.PercentDroppedFrames3.CompositorThread.Video",

			// Other metrics.
			"Graphics.Paint.UI.NormalizedInvalidatedArea",
		}
}
