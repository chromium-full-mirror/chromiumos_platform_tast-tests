# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
import pathlib
import unittest

from analyzer.analysis import analysis_cfg


FILES_DIR: pathlib.Path = (
    pathlib.Path(__file__).parent.absolute().joinpath("files")
)


class AnalysisCfgTest(unittest.TestCase):
    def _load_experiment_cfg_per_test_cfg(self) -> analysis_cfg.ExperimentCfg:
        return analysis_cfg.ExperimentCfg.from_json(
            FILES_DIR.joinpath("experiment-cfg-per-test-cfg.json").read_text()
        )

    def _assert_test_metric_allowed(
        self, cfg: analysis_cfg.ExperimentCfg, test_name: str, metric_name: str
    ) -> None:
        per_test_cfg = cfg.compute_per_test_cfg(test_name)
        self.assertTrue(per_test_cfg.metric_allowed(metric_name))

    def _assert_test_metric_blocked(
        self, cfg: analysis_cfg.ExperimentCfg, test_name: str, metric_name: str
    ) -> None:
        per_test_cfg = cfg.compute_per_test_cfg(test_name)
        self.assertFalse(per_test_cfg.metric_allowed(metric_name))

    def test_allow_default(self) -> None:
        cfg = analysis_cfg.ExperimentCfg()

        self._assert_test_metric_allowed(cfg, "any", "any")

    def test_experiment_groups_cfg(self) -> None:
        cfg = analysis_cfg.ExperimentCfg.from_json(
            FILES_DIR.joinpath(
                "experiment-cfg-experiment-groups-cfg.json"
            ).read_text()
        )
        self.assertEqual(
            cfg,
            analysis_cfg.ExperimentCfg(
                experiment_groups_cfgs=[
                    analysis_cfg.ExperimentGroupsCfg(
                        metric_path_regex_list=["test1.a", "test2.b"],
                        test_name_regex_list=["test1", "test2"],
                    )
                ]
            ),
        )

    def test_unspecified_experiment_groups_cfg(self) -> None:
        cfg = self._load_experiment_cfg_per_test_cfg()
        self.assertIsNone(cfg.experiment_groups_cfgs)

    def test_blocked_metrics(self) -> None:
        cfg = self._load_experiment_cfg_per_test_cfg()

        self._assert_test_metric_blocked(
            cfg, "platform.BootPerfA", "seconds_kernel_to_login.summary"
        )
        self._assert_test_metric_blocked(
            cfg, "platform.BootPerf", "seconds_kernel_to_logi.summary"
        )
        self._assert_test_metric_blocked(
            cfg, "platform.BootPerf", "seconds_kernel_to_loginA.summary"
        )
        self._assert_test_metric_blocked(
            cfg, "arcappgameperf.DotaUnderlords.vm", "fps.summary"
        )
        self._assert_test_metric_blocked(
            cfg, "arcappgameperf.GachaClub.vm", "fps.summary"
        )
        self._assert_test_metric_blocked(
            cfg, "arcappgameperf.MinecraftConsumer.vm", "fps.summary"
        )
        self._assert_test_metric_blocked(
            cfg, "arcappgameperf.RaidShadowLegends.vm", "fps.summary"
        )
        self._assert_test_metric_blocked(cfg, "any", "any")

    def test_allowed_metrics(self) -> None:
        cfg = self._load_experiment_cfg_per_test_cfg()

        self._assert_test_metric_allowed(
            cfg, "platform.BootPerf", "seconds_kernel_to_login.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "platform.BootPerf", "seconds_power_on_to_kernel.summary"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.LoginPerf.ash_chrome_delay_login",
            "Ash.LoginAnimation.Duration.ClamshellMode.arcenabled.8windows.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.LoginPerf.ash_chrome_delay_login",
            "Ash.LoginAnimation.Duration.ClamshellMode.noarc.8windows.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.LoginPerf.ash_chrome_delay_login",
            "Ash.LoginAnimation.Duration2.ClamshellMode.arcenabled.8windows.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.LoginPerf.ash_chrome_delay_login",
            "Ash.LoginAnimation.Duration2.ClamshellMode.noarc.8windows.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.OobePerf", "OOBE.WebUI.LoadTime.FirstRun.Duration.average"
        )
        self._assert_test_metric_allowed(
            cfg, "multivm.MemoryCanaryPerf", "arc_perceptible.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "multivm.MemoryCanaryPerf", "tab_protected.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "multivm.MemoryCanaryPerf", "arc_foreground.summary"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.BenchmarkCUJ.speedometer",
            "Benchmark.Speedometer.Score.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.BenchmarkCUJ.motionmark",
            "Benchmark.MotionMark.Score.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.BenchmarkCUJ.jetstream",
            "Benchmark.JetStream.Score.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DesksCUJ",
            "Ash.Smoothness.PercentDroppedFrames_1sWindow2.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "Ash.EventLatency.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "EventLatency.MousePressed.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "EventLatency.KeyPressed.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "EventLatency.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DesksCUJ",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "Memory.PressureLevel2.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DesksCUJ",
            "PageLoad.InteractiveTiming.FirstInputDelay4.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "PageLoad.InteractiveTiming.InputDelay.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DesksCUJ",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DesksCUJ",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "psi_full_avg10_final.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "TPS.Power.Timeline.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "TPS.RAM.Zram.Max.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DocsCUJ",
            "Ash.Smoothness.PercentDroppedFrames_1sWindow2.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "Ash.EventLatency.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "EventLatency.MousePressed.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "EventLatency.KeyPressed.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "EventLatency.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DocsCUJ",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "Memory.PressureLevel2.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DocsCUJ",
            "PageLoad.InteractiveTiming.FirstInputDelay4.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "PageLoad.InteractiveTiming.InputDelay.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DocsCUJ",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DocsCUJ",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "psi_full_avg10_final.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "TPS.Power.Timeline.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "TPS.RAM.Zram.Max.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "Ash.Smoothness.PercentDroppedFrames_1sWindow2.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "Ash.EventLatency.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "EventLatency.MousePressed.TotalLatency.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "EventLatency.KeyPressed.TotalLatency.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "EventLatency.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "Memory.PressureLevel2.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "PageLoad.InteractiveTiming.FirstInputDelay4.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "PageLoad.InteractiveTiming.InputDelay.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "psi_full_avg10_final.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "TPS.Power.Timeline.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "TPS.RAM.Zram.Max.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "WebRTC.Video.DroppedFrames.Capturer.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "Ash.Smoothness.PercentDroppedFrames_1sWindow2.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "Ash.EventLatency.TotalLatency.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "EventLatency.MousePressed.TotalLatency.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "EventLatency.KeyPressed.TotalLatency.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "EventLatency.TotalLatency.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "Memory.PressureLevel2.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "PageLoad.InteractiveTiming.FirstInputDelay4.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "PageLoad.InteractiveTiming.InputDelay.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "psi_full_avg10_final.summary",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "TPS.Power.Timeline.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.4p_present_notes_split", "TPS.RAM.Zram.Max.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "WebRTC.Video.DroppedFrames.Capturer.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "Ash.Smoothness.PercentDroppedFrames_1sWindow2.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "Ash.EventLatency.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "EventLatency.MousePressed.TotalLatency.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "EventLatency.KeyPressed.TotalLatency.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "EventLatency.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "Memory.PressureLevel2.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "PageLoad.InteractiveTiming.FirstInputDelay4.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "PageLoad.InteractiveTiming.InputDelay.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "psi_full_avg10_final.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "TPS.Power.Timeline.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "TPS.RAM.Zram.Max.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "WebRTC.Video.DroppedFrames.Capturer.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.VideoCUJ",
            "Ash.Smoothness.PercentDroppedFrames_1sWindow2.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "Ash.EventLatency.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "EventLatency.MousePressed.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "EventLatency.KeyPressed.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "EventLatency.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.VideoCUJ",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "Memory.PressureLevel2.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.VideoCUJ",
            "PageLoad.InteractiveTiming.FirstInputDelay4.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "PageLoad.InteractiveTiming.InputDelay.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.VideoCUJ",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.VideoCUJ",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.average",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "psi_full_avg10_final.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "TPS.Power.Timeline.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "TPS.RAM.Zram.Max.average"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "CrosVideo.DroppedFrames.average"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AuthPerf.unmanaged", "play_store_shown.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AuthPerf.unmanaged", "sign_in_time.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.RegularBoot", "app_shown_time.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf", "total_score.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf", "io_score.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf", "memory_zram.zram_write_IOs.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf", "memory_zram.zram_read_IOs.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.PowerIdlePerf", "system.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.DotaUnderlords", "fps.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.DotaUnderlords", "launchTime.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.RaidShadowLegends", "fps.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.RaidShadowLegends", "launchTime.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.GachaClub", "fps.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.GachaClub", "launchTime.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.MinecraftConsumer", "fps.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.MinecraftConsumer", "launchTime.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.MousePerf", "avgMouseLeftClickLatency.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.KeyboardPerf", "avgKeyboardLatency.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.TouchPerf", "avgTouchScreenPressLatency.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.GamepadPerf", "avgGamepadButtonLatency.summary"
        )
        self._assert_test_metric_allowed(
            cfg,
            "multivm.Login.arc_container",
            "total_memory_used_quiet.summary",
        )
        self._assert_test_metric_allowed(
            cfg, "arc.OobeProvisioningPerf.unmanaged", "arc_total_kills.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.RegularBoot", "arc_total_kills.summary"
        )
        self._assert_test_metric_allowed(
            cfg,
            "arc.OobeProvisioningPerf.unmanaged",
            "provisioning_time.summary",
        )
        self._assert_test_metric_allowed(
            cfg, "arc.OobeProvisioningPerf.unmanaged", "arc_total_kills.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AuthPerf.unmanaged_vm", "play_store_shown.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AuthPerf.unmanaged_vm", "sign_in_time.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.RegularBoot.vm", "app_shown_time.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf.vm", "total_score.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf.vm", "io_score.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf.vm", "memory_zram.zram_write_IOs.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf.vm", "memory_zram.zram_read_IOs.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.PowerIdlePerf.vm", "system.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.DotaUnderlords.vm", "launchTime.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.RaidShadowLegends.vm", "launchTime.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.GachaClub.vm", "launchTime.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.MinecraftConsumer.vm", "launchTime.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.MousePerf.vm", "avgMouseLeftClickLatency.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.KeyboardPerf.vm", "avgKeyboardLatency.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.TouchPerf.vm", "avgTouchScreenPressLatency.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.GamepadPerf.vm", "avgGamepadButtonLatency.summary"
        )
        self._assert_test_metric_allowed(
            cfg, "multivm.Login.arc", "total_memory_used_quiet.summary"
        )
        self._assert_test_metric_allowed(
            cfg,
            "arc.OobeProvisioningPerf.unmanaged_vm",
            "arc_total_kills.summary",
        )
        self._assert_test_metric_allowed(
            cfg, "arc.RegularBoot.vm", "arc_total_kills.summary"
        )
        self._assert_test_metric_allowed(
            cfg,
            "arc.OobeProvisioningPerf.unmanaged_vm",
            "provisioning_time.summary",
        )
        self._assert_test_metric_allowed(
            cfg,
            "arc.OobeProvisioningPerf.unmanaged_vm",
            "arc_total_kills.summary",
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Browser.MainThreadsCongestion.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "PageLoad.InteractiveTiming.InputDelay3.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "WebVitals.FirstInputDelay2.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "WebVitals.InteractionToNextPaint2.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "WebVitals.FirstContentfulPaint3.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "WebVitals.LargestContentfulPaint2.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "any",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "any",
            "Graphics.Smoothness.Checkerboarding3.AllSequences.average",
        )
        self._assert_test_metric_allowed(
            cfg,
            "any",
            "EventLatency.GestureScrollUpdate.Touchscreen.TotalLatency.average",
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Memory.Browser.PrivateMemoryFootprint.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Memory.Gpu.PrivateMemoryFootprint.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Power.BatteryDischargeRate.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.LoginAnimation.Jank.ClamshellMode.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.LoginAnimation.Duration.ClamshellMode.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.LoginAnimation.Jank.TabletMode.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.LoginAnimation.Duration.TabletMode.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.Desks.AnimationLatency.DeskActivation.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.Desks.AnimationLatency.DeskRemoval.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.EventLatency.Core.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.EventLatency.KeyPressed.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.EventLatency.MousePressed.TotalLatency.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Memory.PressureLevel2.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "ChromeOS.CWP.PSIMemPressure.Some.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "ChromeOS.CWP.PSIMemPressure.Full.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "ChromeOS.CWP.PSIMemPressure.ArcSome.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "ChromeOS.CWP.PSIMemPressure.ArcFull.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "any",
            "Memory.PressureWindowDuration.CriticalToModerate.average",
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Memory.PressureWindowDuration.CriticalToNone.average"
        )
        self._assert_test_metric_allowed(
            cfg,
            "any",
            "Memory.PressureWindowDuration.ModerateToCritical.average",
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Memory.PressureWindowDuration.ModerateToNone.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "PageLoad.Cpu.TotalUsage.average"
        )
        self._assert_test_metric_allowed(cfg, "any", "BootTime.Total2.average")
        self._assert_test_metric_allowed(cfg, "any", "BootTime.System.average")
        self._assert_test_metric_allowed(cfg, "any", "BootTime.Login2.average")
        self._assert_test_metric_allowed(cfg, "any", "BootTime.Kernel.average")
        self._assert_test_metric_allowed(
            cfg, "any", "BootTime.Firmware.average"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Browser.Tabs.TotalSwitchDuration3.average"
        )
