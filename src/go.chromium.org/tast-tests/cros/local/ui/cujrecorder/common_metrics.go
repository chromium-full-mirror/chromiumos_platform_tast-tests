// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cujrecorder

import (
	"go.chromium.org/tast-tests/cros/common/perf"
)

// AshCommonMetricConfigs returns SPERA common metrics which are
// required to be collected by CUJ tests from the Ash process only.
func AshCommonMetricConfigs() []MetricConfig {
	return []MetricConfig{
		// Responsiveness.
		NewCustomMetricConfig("Ash.SplitViewResize.PresentationTime.ClamshellMode.SingleWindow", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.TabDrag.PresentationTime.ClamshellMode", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.TabDrag.PresentationTime.MaxLatency.ClamshellMode", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.SplitViewResize.PresentationTime.ClamshellMode.WithOverview", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.SplitViewResize.PresentationTime.MaxLatency.ClamshellMode.SingleWindow", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Ash.SplitViewResize.PresentationTime.MaxLatency.ClamshellMode.WithOverview", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Apps.PaginationTransition.DragScroll.PresentationTime.MaxLatency.TabletMode", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Apps.PaginationTransition.DragScroll.PresentationTime.TabletMode", "ms", perf.SmallerIsBetter),

		// Smoothness.
		NewDistributionMetricConfig("Ash.Smoothness.PercentDroppedFrames_1sWindow2", "percent", perf.SmallerIsBetter),
		NewSmoothnessMetricConfig("Apps.PaginationTransition.AnimationSmoothness.ClamshellMode"),
		NewSmoothnessMetricConfig("Apps.StateTransition.AnimationSmoothness"),
		NewSmoothnessMetricConfig("Apps.StateTransition.AnimationSmoothness.Close.ClamshellMode"),
		NewSmoothnessMetricConfig("Apps.StateTransition.AnimationSmoothness.FullscreenAllApps.ClamshellMode"),
		NewSmoothnessMetricConfig("Apps.StateTransition.AnimationSmoothness.Half.ClamshellMode"),
		NewSmoothnessMetricConfig("Apps.StateTransition.AnimationSmoothness.Peeking.ClamshellMode"),
		NewSmoothnessDistributionMetricConfig("Ash.Overview.AnimationSmoothness.Enter.ClamshellMode"),
		NewSmoothnessMetricConfig("Ash.Overview.AnimationSmoothness.Enter.SingleClamshellMode"),
		NewSmoothnessMetricConfig("Ash.Overview.AnimationSmoothness.Enter.SplitView"),
		NewSmoothnessMetricConfig("Ash.Overview.AnimationSmoothness.Exit.ClamshellMode"),
		NewSmoothnessMetricConfig("Ash.Overview.AnimationSmoothness.Exit.SingleClamshellMode"),
		NewSmoothnessMetricConfig("Ash.Overview.AnimationSmoothness.Exit.SplitView"),
		NewSmoothnessMetricConfig("Ash.Overview.AnimationSmoothness.Enter.TabletMode"),
		NewSmoothnessMetricConfig("Ash.Overview.AnimationSmoothness.Exit.MinimizedTabletMode"),
		NewSmoothnessMetricConfig("Ash.Overview.AnimationSmoothness.Enter.MinimizedTabletMode"),
		NewSmoothnessMetricConfig("Ash.Overview.AnimationSmoothness.Exit.TabletMode"),
		NewSmoothnessMetricConfig("Ash.Overview.AnimationSmoothness.Close.TabletMode"),
		NewSmoothnessMetricConfig("Ash.Rotation.AnimationSmoothness"),
		NewSmoothnessDistributionMetricConfig("Ash.WindowCycleView.AnimationSmoothness.Container"),
		NewSmoothnessDistributionMetricConfig("Ash.WindowCycleView.AnimationSmoothness.Show"),
		NewSmoothnessMetricConfig("Ash.Window.AnimationSmoothness.CrossFade"),
		NewSmoothnessMetricConfig("Ash.Window.AnimationSmoothness.Snap"),

		// Media Quality.
		NewCustomMetricConfig("Cras.FetchDelayMilliSeconds", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Cras.UnderrunsPerDevice", "count", perf.SmallerIsBetter),

		// Other metrics to monitor.
		NewCustomMetricConfig("Power.BatteryDischargeRate", "mW", perf.SmallerIsBetter),
	}
}

// LacrosCommonMetricConfigs returns SPERA common metrics which are
// required to be collected by CUJ tests from the Lacros process only.
func LacrosCommonMetricConfigs() []MetricConfig {
	return []MetricConfig{
		// Smoothness.
		NewCustomMetricConfig("Chrome.Lacros.Smoothness.PercentDroppedFrames_1sWindow2", "percent", perf.SmallerIsBetter),
		NewEnumCustomMetricConfig("Viz.DelegatedCompositing.Status",
			map[int64]string{
				0:  "FullDelegation",
				1:  "CompositedOther",
				2:  "CompositedNotAxisAligned",
				3:  "CompositedCheckOverlayFail",
				4:  "CompositedNotOverlay",
				5:  "CompositedTooManyQuads",
				6:  "CompositedBackdropFilter",
				7:  "CompositedCopyRequest",
				8:  "CompositedNotAxisAligned3dTransform",
				9:  "CompositedNotAxisAlignedShearTransform",
				10: "CompositedNotAxisAlignedRotatedTransform",
				11: "CompositedFeatureDisabled",
				12: "CompositedCandidateFailedOther",
				13: "CompositedCandidateBadBlendMode",
				14: "CompositedCandidateBadQuadMaterial",
				15: "CompositedCandidateBadBufferFormat",
				16: "CompositedCandidateHasNearFilter",
				17: "CompositedCandidateNotSharedImage",
				18: "CompositedCandidateBadMaskFilter",
				19: "CompositedCandidateHasTransformCantClip",
				20: "CompositedCandidateIsRenderpassWithTransform"}),
	}
}

// BrowserCommonMetricConfigs returns SPERA common metrics which are
// required to be collected by CUJ tests from the browser process only
// (Ash or Lacros).
func BrowserCommonMetricConfigs() []MetricConfig {
	return []MetricConfig{
		// Responsiveness.
		NewCustomMetricConfig("Browser.Tabs.TabSwitchResult3", "result", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.Tabs.TabSwitchResult3.NoSavedFrames_Loaded", "result", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.Tabs.TabSwitchResult3.NoSavedFrames_NotLoaded", "result", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.Tabs.TabSwitchResult3.WithSavedFrames", "result", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.Tabs.TotalIncompleteSwitchDuration3", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.Tabs.TotalIncompleteSwitchDuration3.NoSavedFrames_Loaded", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.Tabs.TotalIncompleteSwitchDuration3.NoSavedFrames_NotLoaded", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.Tabs.TotalIncompleteSwitchDuration3.WithSavedFrames", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.Tabs.TotalSwitchDuration3", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.Tabs.TotalSwitchDuration3.NoSavedFrames_Loaded", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.Tabs.TotalSwitchDuration3.NoSavedFrames_NotLoaded", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.Tabs.TotalSwitchDuration3.WithSavedFrames", "ms", perf.SmallerIsBetter),
		NewDistributionMetricConfig("EventLatency.GestureScrollUpdate.Touchscreen.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewDistributionMetricConfig("EventLatency.GestureScrollUpdate.Wheel.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("PageLoad.InteractiveTiming.InputDelay3", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("PageLoad.InteractiveTiming.TimeToNextPaint", "ms", perf.SmallerIsBetter),
		NewDistributionMetricConfig("EventLatency.KeyPressed.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewDistributionMetricConfig("EventLatency.MouseDragged.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewDistributionMetricConfig("EventLatency.MouseMoved.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewDistributionMetricConfig("EventLatency.MousePressed.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewDistributionMetricConfig("EventLatency.MouseReleased.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewDistributionMetricConfig("EventLatency.MouseWheel.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewDistributionMetricConfig("EventLatency.GesturePinchUpdate.Touchpad.TotalLatency", "microseconds", perf.SmallerIsBetter),

		// Smoothness.
		NewSmoothnessMetricConfig("Chrome.Tabs.AnimationSmoothness.TabLoading"),
		NewSmoothnessMetricConfig("Chrome.Tabs.AnimationSmoothness.HoverCard.FadeOut"),
		NewSmoothnessMetricConfig("Chrome.Tabs.AnimationSmoothness.HoverCard.FadeIn"),

		// Browser Render Latency.
		NewCustomMetricConfig("PageLoad.PaintTiming.NavigationToLargestContentfulPaint2", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("PageLoad.PaintTiming.NavigationToFirstContentfulPaint", "ms", perf.SmallerIsBetter),
		NewCustomMetricConfig("PageLoad.Experimental.NavigationTiming.NavigationStartToFirstResponseStart", "ms", perf.SmallerIsBetter),

		// Startup Latency.
		NewCustomMetricConfig("Startup.FirstWebContents.NonEmptyPaint3", "ms", perf.SmallerIsBetter),

		// Other metrics to monitor.
		NewDistributionMetricConfig("EventLatency.TotalLatency", "microseconds", perf.SmallerIsBetter),
		NewCustomMetricConfig("Media.Video.Roughness.60fps", "ms", perf.SmallerIsBetter),
		NewDistributionMetricConfig("Graphics.Smoothness.PercentDroppedFrames3.AllInteractions", "percent", perf.SmallerIsBetter),
		NewDistributionMetricConfig("Graphics.Smoothness.PercentDroppedFrames3.CompositorThread.Video", "percent", perf.SmallerIsBetter),
		NewDistributionMetricConfig("Graphics.Smoothness.PercentDroppedFrames3.AllSequences", "percent", perf.SmallerIsBetter),
	}
}

// AnyChromeCommonMetricConfigs returns SPERA common metrics which are
// required to be collected by CUJ tests from any Chrome binary running
// (could be from both Ash and Lacros in parallel).
func AnyChromeCommonMetricConfigs() []MetricConfig {
	return []MetricConfig{
		// Smoothness.
		NewSmoothnessMetricConfig("Ash.Window.AnimationSmoothness.Hide"),

		// Browser main thread responsiveness.
		NewCustomMetricConfig("Browser.MainThreadsCongestion", "janks", perf.SmallerIsBetter),
		NewCustomMetricConfig("Browser.MainThreadsCongestion.RunningOnly", "janks", perf.SmallerIsBetter),
	}
}
