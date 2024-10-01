// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"go.chromium.org/tast/core/testing"
)

// feature is the display name of the feature that will be used on the
// TPS Dashboard.
type feature string

const (
	batterySaver      feature = "BatterySaver"
	arcDisabled       feature = "ArcDisabled"
	arcEnabled        feature = "ArcEnabled"
	pvSched           feature = "Pvsched"
	fieldTrials       feature = "FieldTrials"
	roundedWindows    feature = "RoundedWindows"
	vulkan            feature = "Vulkan"
	wprFeature        feature = "WPR"
	chromevox         feature = "ChromeVox"
	imageIndexing     feature = "ImageIndexing"
	focusMode         feature = "FocusMode"
	passthrough       feature = "Passthrough"
	oak               feature = "Oak"
	noibat            feature = "Noibat"
	twoWindows        feature = "TwoWindows"
	tablet            feature = "Tablet"
	deferTabLoad      feature = "DeferTabLoad"
	deferConcierge    feature = "DeferConcierge"
	docs              feature = "Docs"
	presentation      feature = "MeetPresentation"
	echo              feature = "MeetEcho"
	noiseCancellation feature = "MeetNoiseCancellation"
	noMeetEffects     feature = "NoMeetEffects"
	npu               feature = "NpuInference"
	audioEffects      feature = "MeetAudioEffects"
	studioMic         feature = "MeetStudioMic"
	liveCaptions      feature = "MeetLiveCaptions"
	backgroundBlur    feature = "MeetBackgroundBlur"
	adjustLighting    feature = "MeetAdjustLighting"
	retouch           feature = "MeetRetouch"
	videoEffects      feature = "MeetVideoEffects"
	platformEffects   feature = "MeetPlatformEffects"
	enterprise        feature = "MeetEnterprise"
	muteCamera        feature = "MuteCamera"
	vsyncDecoding     feature = "MeetVsyncDecoding"
	meetEffects       feature = "MeetEffects"
	schedRt           feature = "RealtimeScheduler"
)

// Metadata represents metadata for a performance CUJ or a performance test.
type Metadata struct {
	// Test name associated with the given metadata. This field will be set
	// automatically when using WriteMetadataFile.
	TestName string

	DisplayName   string    // Optional display name to be shown on the dashboard.
	BaseTestNames []string  // Optional base tests that this test could be compared to.
	Metrics       []string  // Recommended metrics for this test.
	Features      []feature // Features that this test is testing.
}

var defaultMetrics = []string{
	"TPS.Power.Timeline",
	"Ash.Smoothness.PercentDroppedFrames_1sWindow2",
	"Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
	"EventLatency.MousePressed.TotalLatency",
	"EventLatency.KeyPressed.TotalLatency",
	"EventLatency.TotalLatency",
	"Ash.EventLatency.TotalLatency",
	"PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
	"PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
}

// VideoCUJ metrics.
var videoMetrics = append(defaultMetrics, []string{
	"CrosVideo.DroppedFrames",
	"CrosVideo.PercentDroppedFrames",
}...)

// Benchmark metrics.
const (
	speedometerMetric  = "Benchmark.Speedometer.Score"
	speedometer3Metric = "Benchmark.Speedometer3.Score"
	motionmarkMetric   = "Benchmark.Motionmark.Score"
	krakenMetric       = "Benchmark.Kraken.Score"
	octaneMetric       = "Benchmark.Octane.Score"
	jetstreamMetric    = "Benchmark.Jetstream.Score"
	webxprt4Metric     = "Benchmark.WebXPRT4.Score"
)

var idlePerfMetrics = []string{
	"TPS.Power.Timeline",
}

var overviewPerfMetrics = []string{
	"Ash.Overview.AnimationSmoothness.Enter.ClamshellMode.8windows",
	"Ash.Overview.AnimationSmoothness.Enter.MinimizedTabletMode.8windows",
	"Ash.Overview.AnimationSmoothness.Enter.SingleClamshellMode.8windows",
	"Ash.Overview.AnimationSmoothness.Enter.SplitView.8windowsincludingmaximizedoverviewwindows",
	"Ash.Overview.AnimationSmoothness.Enter.SplitView.8windowsincludingminimizedoverviewwindows",
	"Ash.Overview.AnimationSmoothness.Enter.TabletMode.8windows",
	"Ash.Overview.AnimationSmoothness.Exit.ClamshellMode.8windows",
	"Ash.Overview.AnimationSmoothness.Exit.MinimizedTabletMode.8windows",
	"Ash.Overview.AnimationSmoothness.Exit.SingleClamshellMode.8windows",
	"Ash.Overview.AnimationSmoothness.Exit.SplitView.8windowsincludingmaximizedoverviewwindows",
	"Ash.Overview.AnimationSmoothness.Exit.SplitView.8windowsincludingminimizedoverviewwindows",
	"Ash.Overview.AnimationSmoothness.Exit.TabletMode.8windows",
}

var windowCyclePerfMetrics = []string{
	"Ash.WindowCycleController.Enter.PresentationTime.8windows",
	"Ash.WindowCycleView.AnimationSmoothness.Container.8windows",
	"Ash.WindowCycleView.AnimationSmoothness.Show.8windows",
}

var loginPerfMetrics = []string{
	"BootTime.Login2",
	"TPS.OnAuthSuccess",
	"TPS.UserProfileGotten",
	"TPS.SessionRestore-Start",
	"TPS.SessionRestore-End",
	"Ash.LoginPerf.AutoRestore.AllBrowserWindowsCreated",
	"Ash.LoginPerf.AutoRestore.AllBrowserWindowsShown",
	"Ash.LoginPerf.AutoRestore.AllBrowserWindowsPresented",
	"Ash.LoginPerf.AutoRestore.AllShelfIconsLoaded",
	"Ash.LoginPerf.AutoRestore.ShelfLoginAnimationEnd",
}

var meetMetrics = []string{
	"TPS.Power.Timeline",
	"Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
	"EventLatency.MousePressed.TotalLatency",
	"EventLatency.KeyPressed.TotalLatency",
	"EventLatency.TotalLatency",
	"WebRTC.Video.DroppedFrames.Capturer",
	"WebRTC.Video.RenderFramesPerSecond",
}

var webglAquariumMetrics = []string{
	"avg_fps",
	"avg_interframe_time",
	"cpu_usage",
	"frequency",
	"gpu_usage",
	"rc6_usage",
}

var powerMetrics = []string{
	"perf.minutes_battery_life",
}

var webRTCVideoConferenceMetrics = []string{
	"perf.minutes_battery_life",
	"EventLatency.KeyPressed.TotalLatency_mean",
	"EventLatency.MouseDragged.TotalLatency_mean",
	"EventLatency.MousePressed.TotalLatency_mean",
	"Graphics.Smoothness.PercentDroppedFrames3.AllSequences_mean",
}

var hotseatScrollMetrics = []string{
	"Apps.ScrollableShelf.AnimationSmoothness.TabletMode.LauncherHidden",
	"Apps.ScrollableShelf.AnimationSmoothness.TabletMode.LauncherHidden.OverviewShown",
	"Apps.ScrollableShelf.AnimationSmoothness.TabletMode.LauncherVisible",
}

var hotseatAnimationMetrics = []string{
	"Apps.HomeLauncherTransition.AnimationSmoothness.FadeInOverview",
	"Apps.HomeLauncherTransition.AnimationSmoothness.FadeOutOverview",
	"Ash.HotseatWidgetAnimation.Widget.AnimationSmoothness.TransitionToExtendedHotseat",
	"Ash.HotseatWidgetAnimation.Widget.AnimationSmoothness.TransitionToHiddenHotseat.WindowActivation",
	"Ash.HotseatWidgetAnimation.Widget.AnimationSmoothness.TransitionToHiddenHotseat.WindowCreation",
	"Ash.HotseatWidgetAnimation.Widget.AnimationSmoothness.TransitionToShownHotseat",
	"Ash.HotseatWidgetAnimation.Widget.AnimationSmoothness.TransitionToShownHotseat.WindowCreation",
}

var overviewDragMetrics = []string{
	"Ash.Overview.WindowDrag.PresentationTime.TabletMode.DragToClose.2windows",
	"Ash.Overview.WindowDrag.PresentationTime.TabletMode.DragToClose.2windows",
}

var splitViewMetrics = []string{
	"Ash.SplitViewResize.AnimationSmoothness.DividerAnimation",
	"Ash.SplitViewResize.PresentationTime.MaxLatency.TabletMode.MultiWindow",
	"Ash.SplitViewResize.PresentationTime.MaxLatency.TabletMode.SingleWindow",
	"Ash.SplitViewResize.PresentationTime.MaxLatency.TabletMode.WithOverview",
	"Ash.SplitViewResize.PresentationTime.TabletMode.MultiWindow",
	"Ash.SplitViewResize.PresentationTime.TabletMode.SingleWindow",
	"Ash.SplitViewResize.PresentationTime.TabletMode.WithOverview",
}

var playbackMetrics = []string{
	"dropped_frames",
	"dropped_frames_percent",
}

// Registry maps test name to its corresponding metadata.
var Registry = map[string]Metadata{
	"ui.DesksCUJ": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.DesksCUJ.pvsched": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{pvSched},
	},
	"ui.DesksCUJ.field_trials": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{fieldTrials},
	},
	"ui.DesksCUJ.battery_saver": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{batterySaver},
	},
	"ui.DesksCUJ.rounded_windows": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{roundedWindows},
	},
	"ui.DesksCUJ.vulkan": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{vulkan},
	},
	"ui.DesksCUJ.arc_disabled": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{arcDisabled},
	},
	"ui.BenchmarkCUJ.speedometer": Metadata{
		Metrics: []string{speedometerMetric},
	},
	"ui.BenchmarkCUJ.speedometer3": Metadata{
		Metrics: []string{speedometer3Metric},
	},
	"ui.BenchmarkCUJ.motionmark": Metadata{
		Metrics: []string{motionmarkMetric},
	},
	"ui.BenchmarkCUJ.vulkan_motionmark": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.motionmark"},
		Features:      []feature{vulkan},
	},
	"ui.BenchmarkCUJ.motionmark1_3": Metadata{
		DisplayName: "BenchmarkMotionmark 1.3",
		Metrics:     []string{motionmarkMetric},
	},
	"ui.BenchmarkCUJ.vulkan_motionmark1_3": Metadata{
		DisplayName:   "BenchmarkMotionmarkVulkan 1.3",
		BaseTestNames: []string{"ui.BenchmarkCUJ.vulkan_motionmark"},
		Features:      []feature{vulkan},
	},
	"ui.BenchmarkCUJ.jetstream": Metadata{
		Metrics: []string{jetstreamMetric},
	},
	"ui.BenchmarkCUJ.kraken": Metadata{
		Metrics: []string{krakenMetric},
	},
	"ui.BenchmarkCUJ.octane": Metadata{
		Metrics: []string{octaneMetric},
	},
	"ui.BenchmarkCUJ.webxprt4": Metadata{
		Metrics: []string{webxprt4Metric},
	},
	"ui.BenchmarkCUJ.vulkan_webxprt4": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.webxprt4"},
		Features:      []feature{vulkan},
	},
	"ui.BenchmarkCUJ.speedometer_wpr": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.speedometer"},
		Features:      []feature{wprFeature},
	},
	"ui.BenchmarkCUJ.motionmark_wpr": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.motionmark"},
		Features:      []feature{wprFeature},
	},
	"ui.BenchmarkCUJ.kraken_wpr": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.kraken"},
		Features:      []feature{wprFeature},
	},
	"ui.BenchmarkCUJ.octane_wpr": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.octane"},
		Features:      []feature{wprFeature},
	},
	"ui.GoogleSheetsCUJ": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.GoogleSheetsCUJ.field_trials": Metadata{
		BaseTestNames: []string{"ui.GoogleSheetsCUJ"},
		Features:      []feature{fieldTrials},
	},
	"ui.TabSwitchPerf": Metadata{
		Metrics: []string{
			"Chrome.Tabs.AnimationSmoothness.TabLoading",
			"Browser.Tabs.TotalSwitchDuration3.WithSavedFrames",
			"Browser.Tabs.TotalSwitchDuration",
			"Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
		},
	},
	"ui.TaskSwitchCUJ": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.TaskSwitchCUJ.tablet": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.TaskSwitchCUJ.field_trials": Metadata{
		BaseTestNames: []string{"ui.TaskSwitchCUJ"},
		Features:      []feature{fieldTrials},
	},
	"ui.TaskSwitchCUJ.pvsched": Metadata{
		BaseTestNames: []string{"ui.TaskSwitchCUJ"},
		Features:      []feature{pvSched},
	},
	"ui.VideoCUJ.field_trials": Metadata{
		BaseTestNames: []string{"ui.VideoCUJ"},
		Features:      []feature{fieldTrials},
	},
	"ui.VideoCUJ": Metadata{
		Metrics: videoMetrics,
	},
	"ui.VideoCUJ.vulkan": Metadata{
		BaseTestNames: []string{"ui.VideoCUJ"},
		Features:      []feature{vulkan},
	},
	"ui.VideoCUJ.pvsched": Metadata{
		BaseTestNames: []string{"ui.VideoCUJ"},
		Features:      []feature{pvSched},
	},
	"ui.DocsCUJ": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.DocsCUJ.field_trials": Metadata{
		BaseTestNames: []string{"ui.DocsCUJ"},
		Features:      []feature{fieldTrials},
	},
	"ui.DocsCUJ.chromevox": Metadata{
		BaseTestNames: []string{"ui.DocsCUJ"},
		Features:      []feature{chromevox},
	},
	"ui.DocsCUJ.vulkan": Metadata{
		BaseTestNames: []string{"ui.DocsCUJ"},
		Features:      []feature{vulkan},
	},
	"ui.DocsCUJ.image_indexing": Metadata{
		BaseTestNames: []string{"ui.DocsCUJ"},
		Features:      []feature{imageIndexing},
	},
	"ui.GalleryCUJ": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.DeskTemplatesCUJ": Metadata{
		Metrics: []string{
			"TPS.Power.Timeline",
			"Ash.Smoothness.PercentDroppedFrames_1sWindow2",
			"Ash.EventLatency.TotalLatency",
		},
	},
	"ui.DeskTemplatesCUJ.arc_enabled": Metadata{
		BaseTestNames: []string{"ui.DeskTemplatesCUJ"},
		Features:      []feature{arcEnabled},
	},
	"ui.DragMaximizedWindowPerf": Metadata{
		Metrics: []string{
			"Ash.PhantomWindowController.Show.PresentationTime",
			"Ash.Window.AnimationSmoothness.CrossFade.DragMaximize",
			"Ash.Window.AnimationSmoothness.CrossFade.DragUnmaximize",
		},
	},
	"ui.DragWindowFromShelfPerf": Metadata{
		Metrics: []string{
			"Ash.DragWindowFromShelf.PresentationTime.MaxLatency",
			"Ash.DragWindowFromShelf.PresentationTime",
			"Ash.Overview.Enter.PresentationTime",
			"Ash.Overview.Exit.PresentationTime",
		},
	},
	"ui.IdlePerf": Metadata{
		Metrics: idlePerfMetrics,
	},
	"ui.IdlePerf.arc_disabled": Metadata{
		BaseTestNames: []string{"ui.IdlePerf"},
		Features:      []feature{arcDisabled},
	},
	"ui.IdlePerf.focusmode": Metadata{
		BaseTestNames: []string{"ui.IdlePerf"},
		Features:      []feature{focusMode},
	},
	"ui.IdlePerf.facegaze": Metadata{
		Metrics: append(idlePerfMetrics, "Accessibility.FaceGaze.AverageFaceLandmarkerLatency"),
	},
	"ui.OverviewPerf": Metadata{
		Metrics: overviewPerfMetrics,
	},
	"ui.OverviewPerf.passthrough": Metadata{
		BaseTestNames: []string{"ui.OverviewPerf"},
		Features:      []feature{passthrough},
	},
	"ui.OverviewPerf.oak": Metadata{
		BaseTestNames: []string{"ui.OverviewPerf"},
		Features:      []feature{oak},
	},
	"ui.OverviewScrollPerf": Metadata{
		Metrics: []string{"Ash.Overview.Scroll.PresentationTime.TabletMode"},
	},
	"ui.WindowCyclePerf": Metadata{
		Metrics: windowCyclePerfMetrics,
	},
	"ui.WindowCyclePerf.noibat": Metadata{
		BaseTestNames: []string{"ui.WindowCyclePerf"},
		Features:      []feature{noibat},
	},
	"ui.TabletTransitionPerf": Metadata{
		Metrics: []string{
			"Ash.TabletMode.AnimationSmoothness.Enter",
			"Ash.TabletMode.AnimationSmoothness.Enter",
		},
	},
	"ui.SnapPerf": Metadata{
		Metrics: []string{
			"Ash.Window.AnimationSmoothness.Snap",
		},
	},
	"ui.LoginPerf": Metadata{
		Metrics: loginPerfMetrics,
	},
	"ui.LoginPerf.noarc_2windows": Metadata{
		BaseTestNames: []string{
			"ui.LoginPerf.noarc",
			"ui.LoginPerf.2windows",
		},
		Features: []feature{twoWindows, arcDisabled},
	},
	"ui.LoginPerf.noarc": Metadata{
		BaseTestNames: []string{"ui.LoginPerf"},
		Features:      []feature{arcDisabled},
	},
	"ui.LoginPerf.2windows": Metadata{
		BaseTestNames: []string{"ui.LoginPerf"},
		Features:      []feature{twoWindows},
	},
	"ui.LoginPerf.tablet": Metadata{
		BaseTestNames: []string{"ui.LoginPerf"},
		Features:      []feature{tablet},
	},
	"ui.LoginPerf.defer_tab_load": Metadata{
		BaseTestNames: []string{"ui.LoginPerf"},
		Features:      []feature{deferTabLoad},
	},
	"ui.LoginPerf.2windows_defer_concierge": Metadata{
		BaseTestNames: []string{"ui.LoginPerf.2windows"},
		Features:      []feature{deferConcierge},
	},
	"ui.LoginPerf.2windows_defer_concierge_arc": Metadata{
		BaseTestNames: []string{
			"ui.LoginPerf.2windows",
			"ui.LoginPerf.2windows_defer_concierge",
		},
		Features: []feature{deferConcierge, arcEnabled},
	},
	"ui.MeetCUJ": Metadata{
		Metrics: meetMetrics,
	},
	"ui.MeetCUJ.docs": Metadata{
		Metrics: meetMetrics,
	},
	"ui.MeetCUJ.present": Metadata{
		Metrics: meetMetrics,
	},
	"ui.MeetCUJ.docs_arc_disabled": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{arcDisabled},
	},
	"ui.MeetCUJ.docs_echo_measured": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{echo},
	},
	"ui.MeetCUJ.docs_nc_echo_measured": Metadata{
		BaseTestNames: []string{
			"ui.MeetCUJ.docs_echo_measured",
			"ui.MeetCUJ.docs_noise_cancellation",
		},
		Features: []feature{noiseCancellation, echo},
	},
	"ui.MeetCUJ.docs_pvsched": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{pvSched},
	},
	"ui.MeetCUJ.docs_no_effects": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{noMeetEffects},
	},
	"ui.MeetCUJ.docs_no_effects_npu": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs_no_effects"},
		Features:      []feature{npu},
	},
	"ui.MeetCUJ.docs_audio_effects": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{audioEffects},
	},
	"ui.MeetCUJ.docs_audio_effects_studio_mic": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{audioEffects},
	},
	"ui.MeetCUJ.docs_noise_cancellation": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{noiseCancellation},
	},
	"ui.MeetCUJ.docs_studio_mic": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{studioMic},
	},
	"ui.MeetCUJ.docs_live_captions": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{liveCaptions},
	},
	"ui.MeetCUJ.docs_background_blur": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{backgroundBlur},
	},
	"ui.MeetCUJ.docs_background_blur_and_meet_effects": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs_background_blur"},
		Features:      []feature{meetEffects},
	},
	"ui.MeetCUJ.docs_background_blur_npu_and_meet_effects": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs_background_blur_and_meet_effects"},
		Features:      []feature{npu},
	},
	"ui.MeetCUJ.docs_adjust_lighting": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{adjustLighting},
	},
	"ui.MeetCUJ.docs_adjust_lighting_npu": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs_adjust_lighting"},
		Features:      []feature{npu},
	},
	"ui.MeetCUJ.docs_retouch": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{retouch},
	},
	"ui.MeetCUJ.docs_adjust_lighting_and_retouch": Metadata{
		BaseTestNames: []string{
			"ui.MeetCUJ.docs_adjust_lighting",
			"ui.MeetCUJ.docs_retouch",
		},
		Features: []feature{retouch, adjustLighting},
	},
	"ui.MeetCUJ.docs_video_effects": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{videoEffects},
	},
	"ui.MeetCUJ.docs_video_effects_npu": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs_video_effects"},
		Features:      []feature{npu},
	},
	"ui.MeetCUJ.docs_platform_effects": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{platformEffects},
	},
	"ui.MeetCUJ.docs_platform_effects_npu": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs_platform_effects"},
		Features:      []feature{npu},
	},
	"ui.MeetCUJ.docs_platform_effects_studio_mic": Metadata{
		BaseTestNames: []string{
			"ui.MeetCUJ.docs_platform_effects",
			"ui.MeetCUJ.docs_studio_mic",
		},
		Features: []feature{studioMic, platformEffects},
	},
	"ui.MeetCUJ.docs_sched_rt": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{schedRt},
	},
	"ui.MeetCUJ.docs_enterprise": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{enterprise},
	},
	"ui.MeetCUJ.docs_field_trials": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.docs"},
		Features:      []feature{fieldTrials},
	},
	"ui.MeetCUJ.9p_mute_camera": Metadata{
		Metrics: meetMetrics,
	},
	"ui.MeetCUJ.9p_mute_camera_vsync_decoding": Metadata{
		BaseTestNames: []string{"ui.MeetCUJ.9p_mute_camera"},
		Features:      []feature{vsyncDecoding},
	},
	"ui.PageLoadPerf": Metadata{
		Metrics: []string{
			"PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
			"PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
		},
	},
	"graphics.WebGLAquarium.50_fishes": Metadata{
		Metrics: webglAquariumMetrics,
	},
	"graphics.WebGLAquarium.1000_fishes": Metadata{
		Metrics: webglAquariumMetrics,
	},
	// There are no recommended metrics for this test, as the metric names
	// vary based on which processes are active during the test.
	"multivm.PerProcessMemory.arc": Metadata{},
	"power.Browsing.fast_ash": Metadata{
		Metrics: powerMetrics,
	},
	"power.Idle.default_fast_ash": Metadata{
		Metrics: powerMetrics,
	},
	"power.VideoCall.3m_ash": Metadata{
		Metrics: powerMetrics,
	},
	"power.VideoPlayback.h264_1080_30fps_ash": Metadata{
		Metrics: powerMetrics,
	},
	"power.VideoPlayback.vp9_1080_30fps_ash": Metadata{
		Metrics: powerMetrics,
	},
	"power.VideoPlayback.vp9_bt2020_1080_30fps_ash": Metadata{
		Metrics: powerMetrics,
	},
	"ui.DesksCUJV2": Metadata{
		Metrics: defaultMetrics,
	},
	"rtc.WebRTCVideoConference.4p_present_mouse": Metadata{
		Metrics: webRTCVideoConferenceMetrics,
	},
	"rtc.WebRTCVideoConference.4p_present_text": Metadata{
		Metrics: webRTCVideoConferenceMetrics,
	},
	"ui.BubbleLauncherAnimationPerf": Metadata{
		Metrics: []string{
			"Apps.ClamshellLauncher.AnimationSmoothness.Close.0Windows",
			"Apps.ClamshellLauncher.AnimationSmoothness.Close.2Windows",
			"Apps.ClamshellLauncher.AnimationSmoothness.OpenAppsPage.0Windows",
			"Apps.ClamshellLauncher.AnimationSmoothness.OpenAppsPage.2Windows",
		},
	},
	"ui.HotseatAnimation.non_overflow_shelf": Metadata{
		Metrics: hotseatAnimationMetrics,
	},
	"ui.HotseatAnimation.overflow_shelf": Metadata{
		Metrics: hotseatAnimationMetrics,
	},
	"ui.HotseatAnimation.shelf_with_navigation_widget": Metadata{
		Metrics: hotseatAnimationMetrics,
	},
	"ui.HotseatDrag": Metadata{
		Metrics: []string{
			"Ash.HotseatTransition.Drag.PresentationTime",
			"Ash.HotseatTransition.Drag.PresentationTime.MaxLatency",
		},
	},
	"ui.HotseatScrollPerf": Metadata{
		Metrics: hotseatScrollMetrics,
	},
	"ui.HotseatScrollPerf.clamshell_mode": Metadata{
		Metrics: hotseatScrollMetrics,
	},
	"ui.LauncherPageSwitchPerf": Metadata{
		Metrics: []string{
			"Apps.PaginationTransition.AnimationSmoothness.TabletMode",
			"Apps.PaginationTransition.DragScroll.PresentationTime.MaxLatency.TabletMode",
			"Apps.PaginationTransition.DragScroll.PresentationTime.TabletMode",
		},
	},
	"ui.OverviewDragWindowPerf.drag_to_close": Metadata{
		Metrics: overviewDragMetrics,
	},
	"ui.OverviewDragWindowPerf.drag_to_snap": Metadata{
		Metrics: overviewDragMetrics,
	},
	"ui.OverviewDragWindowPerf.normal_drag": Metadata{
		Metrics: overviewDragMetrics,
	},
	"ui.ScreenRotationPerf": Metadata{
		Metrics: []string{
			"Ash.Rotation.AnimationSmoothness.2windows",
			"Ash.Rotation.AnimationSmoothness.8windows",
		},
	},
	"ui.SplitViewResizePerf": Metadata{
		Metrics: splitViewMetrics,
	},
	"ui.SplitViewResizePerf.clamshell_mode": Metadata{
		Metrics: splitViewMetrics,
	},
	"ui.TabLoadingAnimationPerf": Metadata{
		Metrics: []string{
			"Chrome.Tabs.AnimationSmoothness.TabLoading",
		},
	},
	"ui.UnlockPerf": Metadata{
		Metrics: []string{
			"Ash.UnlockAnimation.Smoothness.ClamshellMode.2windows",
			"Ash.UnlockAnimation.Smoothness.ClamshellMode.8windows",
			"Ash.UnlockAnimation.Smoothness.TabletMode.2windows",
			"Ash.UnlockAnimation.Smoothness.TabletMode.8windows",
		},
	},
	"ui.UnlockPerf.passthrough": Metadata{
		BaseTestNames: []string{"ui.UnlockPerf"},
		Features:      []feature{passthrough},
	},
	"ui.WindowResizePerf": Metadata{
		Metrics: []string{
			"Ash.InteractiveWindowResize.TimeToPresent.1windows",
			"Ash.InteractiveWindowResize.TimeToPresent.2windows",
		},
	},
	"video.PlaybackPerf.av1_1080p_30fps_hw": Metadata{
		Metrics: playbackMetrics,
	},
	"video.PlaybackPerf.h264_1080p_30fps_hw": Metadata{
		Metrics: playbackMetrics,
	},
	"video.PlaybackPerf.vp8_1080p_30fps_hw": Metadata{
		Metrics: playbackMetrics,
	},
	"video.PlaybackPerf.vp9_1080p_30fps_hw": Metadata{
		Metrics: playbackMetrics,
	},

	// The TPS Dashboard assumes that the metrics passed as part of the
	// metadata for SlidesCUJ are the fallback metrics (default metrics) that
	// should be recommended for tests that don't have corresponding metadata.
	// Thus, any changes here would update default metrics for all tests
	// that aren't defined in this map.
	"ui.GoogleSlidesCUJ": Metadata{
		Metrics: defaultMetrics,
	},
}

// WriteMetadataFile stores a metadata.json file in the testing out
// directory representing the test |testName| found in |registry|.
// We log failures if a test is unregistered or there is an issue saving the
// metadata. If a test registration is found, but the registration is
// malformed, the code will panic.
func WriteMetadataFile(ctx context.Context, testName string) {
	if _, ok := Registry[testName]; !ok {
		testing.ContextLogf(ctx, "Failed to find %s in the metadata registry", testName)
		return
	}

	testing.ContextLogf(ctx, "Writing metadata file for %s", testName)

	test := Registry[testName]
	test.TestName = testName

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		testing.ContextLog(ctx, "Failed to get the out directory")
		return
	}

	json, err := json.MarshalIndent(&test, "", "  ")
	if err != nil {
		testing.ContextLog(ctx, "Failed to marshal metadata: ", err)
	}

	if err := os.WriteFile(filepath.Join(outDir, "metadata.json"), json, 0644); err != nil {
		testing.ContextLog(ctx, "Failed to write metadata: ", err)
	}
}
