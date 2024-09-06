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
    def _load_persistent_cfg_per_test_cfg(self) -> analysis_cfg.PersistentCfg:
        return analysis_cfg.PersistentCfg.from_json(
            FILES_DIR.joinpath("persistent-cfg-per-test-cfg.json").read_text()
        )

    def _assert_test_metric_allowed(
        self, cfg: analysis_cfg.PersistentCfg, test_name: str, metric_name: str
    ) -> None:
        per_test_cfg = cfg.compute_per_test_cfg(test_name)
        self.assertTrue(per_test_cfg.metric_allowed(metric_name))

    def _assert_test_metric_blocked(
        self, cfg: analysis_cfg.PersistentCfg, test_name: str, metric_name: str
    ) -> None:
        per_test_cfg = cfg.compute_per_test_cfg(test_name)
        self.assertFalse(per_test_cfg.metric_allowed(metric_name))

    def test_allow_default(self) -> None:
        cfg = analysis_cfg.PersistentCfg()

        self._assert_test_metric_allowed(cfg, "any", "any")

    def test_experiment_groups_cfg(self) -> None:
        cfg = analysis_cfg.PersistentCfg.from_json(
            FILES_DIR.joinpath(
                "persistent-cfg-experiment-groups-cfg.json"
            ).read_text()
        )
        self.assertEqual(
            cfg,
            analysis_cfg.PersistentCfg(
                experiment_groups_cfgs=[
                    analysis_cfg.ExperimentGroupsCfg(
                        metric_path_regex_list=["test1", "test2"]
                    )
                ]
            ),
        )

    def test_unspecified_experiment_groups_cfg(self) -> None:
        cfg = self._load_persistent_cfg_per_test_cfg()
        self.assertIsNone(cfg.experiment_groups_cfgs)

    def test_blocked_metrics(self) -> None:
        cfg = self._load_persistent_cfg_per_test_cfg()

        self._assert_test_metric_blocked(
            cfg, "platform.BootPerfA", "seconds_kernel_to_login"
        )
        self._assert_test_metric_blocked(
            cfg, "platform.BootPerf", "seconds_kernel_to_logi"
        )
        self._assert_test_metric_blocked(
            cfg, "platform.BootPerf", "seconds_kernel_to_loginA"
        )
        self._assert_test_metric_blocked(
            cfg, "arcappgameperf.DotaUnderlords.vm", "fps"
        )
        self._assert_test_metric_blocked(
            cfg, "arcappgameperf.GachaClub.vm", "fps"
        )
        self._assert_test_metric_blocked(
            cfg, "arcappgameperf.MinecraftConsumer.vm", "fps"
        )
        self._assert_test_metric_blocked(
            cfg, "arcappgameperf.RaidShadowLegends.vm", "fps"
        )
        self._assert_test_metric_blocked(cfg, "any", "any")

    def test_allowed_metrics(self) -> None:
        cfg = self._load_persistent_cfg_per_test_cfg()

        self._assert_test_metric_allowed(
            cfg, "platform.BootPerf", "seconds_kernel_to_login"
        )
        self._assert_test_metric_allowed(
            cfg, "platform.BootPerf", "seconds_power_on_to_kernel"
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
            cfg, "multivm.MemoryCanaryPerf", "arc_perceptible"
        )
        self._assert_test_metric_allowed(
            cfg, "multivm.MemoryCanaryPerf", "tab_protected"
        )
        self._assert_test_metric_allowed(
            cfg, "multivm.MemoryCanaryPerf", "arc_foreground"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.BenchmarkCUJ.speedometer", "Benchmark.Speedometer.Score"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.BenchmarkCUJ.motionmark", "Benchmark.MotionMark.Score"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.BenchmarkCUJ.jetstream", "Benchmark.JetStream.Score"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "Ash.Smoothness.PercentDroppedFrames_1sWindow2"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "Ash.EventLatency.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "EventLatency.MousePressed.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "EventLatency.KeyPressed.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "EventLatency.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DesksCUJ",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "Memory.PressureLevel2"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "PageLoad.InteractiveTiming.FirstInputDelay4"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "PageLoad.InteractiveTiming.InputDelay"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DesksCUJ",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DesksCUJ",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "psi_full_avg10_final"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DesksCUJ", "TPS.Power.Timeline"
        )
        self._assert_test_metric_allowed(cfg, "ui.DesksCUJ", "TPS.RAM.Zram.Max")
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "Ash.Smoothness.PercentDroppedFrames_1sWindow2"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "Ash.EventLatency.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "EventLatency.MousePressed.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "EventLatency.KeyPressed.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "EventLatency.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DocsCUJ",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "Memory.PressureLevel2"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "PageLoad.InteractiveTiming.FirstInputDelay4"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "PageLoad.InteractiveTiming.InputDelay"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DocsCUJ",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.DocsCUJ",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "psi_full_avg10_final"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.DocsCUJ", "TPS.Power.Timeline"
        )
        self._assert_test_metric_allowed(cfg, "ui.DocsCUJ", "TPS.RAM.Zram.Max")
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "Ash.Smoothness.PercentDroppedFrames_1sWindow2",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "Ash.EventLatency.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "EventLatency.MousePressed.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "EventLatency.KeyPressed.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "EventLatency.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "Memory.PressureLevel2"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "PageLoad.InteractiveTiming.FirstInputDelay4",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "PageLoad.InteractiveTiming.InputDelay"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.docs",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "psi_full_avg10_final"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "TPS.Power.Timeline"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "TPS.RAM.Zram.Max"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.docs", "WebRTC.Video.DroppedFrames.Capturer"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "Ash.Smoothness.PercentDroppedFrames_1sWindow2",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "Ash.EventLatency.TotalLatency",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "EventLatency.MousePressed.TotalLatency",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "EventLatency.KeyPressed.TotalLatency",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "EventLatency.TotalLatency",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.4p_present_notes_split", "Memory.PressureLevel2"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "PageLoad.InteractiveTiming.FirstInputDelay4",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "PageLoad.InteractiveTiming.InputDelay",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.4p_present_notes_split", "psi_full_avg10_final"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.4p_present_notes_split", "TPS.Power.Timeline"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.4p_present_notes_split", "TPS.RAM.Zram.Max"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.4p_present_notes_split",
            "WebRTC.Video.DroppedFrames.Capturer",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "Ash.Smoothness.PercentDroppedFrames_1sWindow2",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "Ash.EventLatency.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "EventLatency.MousePressed.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "EventLatency.KeyPressed.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "EventLatency.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "Memory.PressureLevel2"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "PageLoad.InteractiveTiming.FirstInputDelay4"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "PageLoad.InteractiveTiming.InputDelay"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.MeetCUJ.49p",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "psi_full_avg10_final"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "TPS.Power.Timeline"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "TPS.RAM.Zram.Max"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.MeetCUJ.49p", "WebRTC.Video.DroppedFrames.Capturer"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "Ash.Smoothness.PercentDroppedFrames_1sWindow2"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "Ash.EventLatency.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "EventLatency.MousePressed.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "EventLatency.KeyPressed.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "EventLatency.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.VideoCUJ",
            "Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "Memory.PressureLevel2"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "PageLoad.InteractiveTiming.FirstInputDelay4"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "PageLoad.InteractiveTiming.InputDelay"
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.VideoCUJ",
            "PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
        )
        self._assert_test_metric_allowed(
            cfg,
            "ui.VideoCUJ",
            "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "psi_full_avg10_final"
        )
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "TPS.Power.Timeline"
        )
        self._assert_test_metric_allowed(cfg, "ui.VideoCUJ", "TPS.RAM.Zram.Max")
        self._assert_test_metric_allowed(
            cfg, "ui.VideoCUJ", "CrosVideo.DroppedFrames"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AuthPerf.unmanaged", "play_store_shown"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AuthPerf.unmanaged", "sign_in_time"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.RegularBoot", "app_shown_time"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf", "total_score"
        )
        self._assert_test_metric_allowed(cfg, "arc.AppLoadingPerf", "io_score")
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf", "memory_zram.zram_write_IOs"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf", "memory_zram.zram_read_IOs"
        )
        self._assert_test_metric_allowed(cfg, "arc.PowerIdlePerf", "system")
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.DotaUnderlords", "fps"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.DotaUnderlords", "launchTime"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.RaidShadowLegends", "fps"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.RaidShadowLegends", "launchTime"
        )
        self._assert_test_metric_allowed(cfg, "arcappgameperf.GachaClub", "fps")
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.GachaClub", "launchTime"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.MinecraftConsumer", "fps"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.MinecraftConsumer", "launchTime"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.MousePerf", "avgMouseLeftClickLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.KeyboardPerf", "avgKeyboardLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.TouchPerf", "avgTouchScreenPressLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.GamepadPerf", "avgGamepadButtonLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "multivm.Login.arc_container", "total_memory_used_quiet.."
        )
        self._assert_test_metric_allowed(
            cfg, "arc.OobeProvisioningPerf.unmanaged", "arc_total_kills"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.RegularBoot", "arc_total_kills"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.OobeProvisioningPerf.unmanaged", "provisioning_time"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.OobeProvisioningPerf.unmanaged", "arc_total_kills"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AuthPerf.unmanaged_vm", "play_store_shown"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AuthPerf.unmanaged_vm", "sign_in_time"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.RegularBoot.vm", "app_shown_time"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf.vm", "total_score"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf.vm", "io_score"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf.vm", "memory_zram.zram_write_IOs"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.AppLoadingPerf.vm", "memory_zram.zram_read_IOs"
        )
        self._assert_test_metric_allowed(cfg, "arc.PowerIdlePerf.vm", "system")
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.DotaUnderlords.vm", "launchTime"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.RaidShadowLegends.vm", "launchTime"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.GachaClub.vm", "launchTime"
        )
        self._assert_test_metric_allowed(
            cfg, "arcappgameperf.MinecraftConsumer.vm", "launchTime"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.MousePerf.vm", "avgMouseLeftClickLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.KeyboardPerf.vm", "avgKeyboardLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.TouchPerf.vm", "avgTouchScreenPressLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.GamepadPerf.vm", "avgGamepadButtonLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "multivm.Login.arc", "total_memory_used_quiet.."
        )
        self._assert_test_metric_allowed(
            cfg, "arc.OobeProvisioningPerf.unmanaged_vm", "arc_total_kills"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.RegularBoot.vm", "arc_total_kills"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.OobeProvisioningPerf.unmanaged_vm", "provisioning_time"
        )
        self._assert_test_metric_allowed(
            cfg, "arc.OobeProvisioningPerf.unmanaged_vm", "arc_total_kills"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Browser.MainThreadsCongestion"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "PageLoad.InteractiveTiming.InputDelay3"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "WebVitals.FirstInputDelay2"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "WebVitals.InteractionToNextPaint2"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "WebVitals.FirstContentfulPaint3"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "WebVitals.LargestContentfulPaint2"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Graphics.Smoothness.PercentDroppedFrames3.AllSequences"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Graphics.Smoothness.Checkerboarding3.AllSequences"
        )
        self._assert_test_metric_allowed(
            cfg,
            "any",
            "EventLatency.GestureScrollUpdate.Touchscreen.TotalLatency",
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Memory.Browser.PrivateMemoryFootprint"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Memory.Gpu.PrivateMemoryFootprint"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Power.BatteryDischargeRate"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.LoginAnimation.Jank.ClamshellMode"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.LoginAnimation.Duration.ClamshellMode"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.LoginAnimation.Jank.TabletMode"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.LoginAnimation.Duration.TabletMode"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.Desks.AnimationLatency.DeskActivation"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.Desks.AnimationLatency.DeskRemoval"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.EventLatency.Core.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.EventLatency.KeyPressed.TotalLatency"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Ash.EventLatency.MousePressed.TotalLatency"
        )
        self._assert_test_metric_allowed(cfg, "any", "Memory.PressureLevel2")
        self._assert_test_metric_allowed(
            cfg, "any", "ChromeOS.CWP.PSIMemPressure.Some"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "ChromeOS.CWP.PSIMemPressure.Full"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "ChromeOS.CWP.PSIMemPressure.ArcSome"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "ChromeOS.CWP.PSIMemPressure.ArcFull"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Memory.PressureWindowDuration.CriticalToModerate"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Memory.PressureWindowDuration.CriticalToNone"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Memory.PressureWindowDuration.ModerateToCritical"
        )
        self._assert_test_metric_allowed(
            cfg, "any", "Memory.PressureWindowDuration.ModerateToNone"
        )
        self._assert_test_metric_allowed(cfg, "any", "PageLoad.Cpu.TotalUsage")
        self._assert_test_metric_allowed(cfg, "any", "BootTime.Total2")
        self._assert_test_metric_allowed(cfg, "any", "BootTime.System")
        self._assert_test_metric_allowed(cfg, "any", "BootTime.Login2")
        self._assert_test_metric_allowed(cfg, "any", "BootTime.Kernel")
        self._assert_test_metric_allowed(cfg, "any", "BootTime.Firmware")
        self._assert_test_metric_allowed(
            cfg, "any", "Browser.Tabs.TotalSwitchDuration3"
        )
