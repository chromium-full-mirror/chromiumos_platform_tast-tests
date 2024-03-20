# Metrics

This document lists the metrics collected in power package through TestMetrics().

| Metric Class                        | Unit  | Note                                                                         |
|---                          |---    |---                                                                           |
| **CPU Idle State**  |
| Name                        | Unit  | Note                                                                         |
| cpu_usage                   | %     | The percent of time all CPUs spent not in idle.                              |
| **Procfs CPU**  |
| cpu-${state name}           | %     | The percent of time all CPUs spend in a certain idle state.                  |
| cpu[0-9]-${state name}      | %     | The percent of time a single CPU spends in a certain idle state.             |
| **Memory Metrics**          |
| Name                        | Unit  | Note                                                                         |
| used                        | KiB   | Currently used RAM not including caches or reclaimable memory.               |
| cached                      | KiB   | In-memory caches for disk reads including Slab-reclaimable memory            |
| buffers                     | KiB   | Temporary storage buffers for block devices.                                 |
| swap                        | KiB   | Currently used swap memory.                                                  |
| page_tables                 | KiB   | Memory used for the lowest-level page tables.                                |
| free                        | KiB   | Amount of free memory.                                                       |
| available                   | KiB   | Amount of available memory without swapping.                                 |
| drm_rank${rank}\_memory_${type}                     | KiB     | The amount of memory that a GPU process is using, ranked by GPU utilization. Types include _total, active, shared_.                                 |
| **RAPL Power**      |
| Name                        | Unit  | Note                                                                         |
| package-0                   | W     | Running average power consumption across the entire SoC.                     |
| non_SoC                     | W     | Running average power consumption of all subsystems (system total minus SoC).|
| PL1                         | W     | A threshold that power should not exceed on average in a longer span of time.|
| core                        | W     | Running average power consumption across all cpu cores.                      |
| uncore                      | W     | Running average power consumption across integrated graphics.                |
| dram                        | W     | Running average power consumption across the DRAM.                           |
| **Sysfs Battery**   |
| Name                        | Unit  | Note                                                                         |
| system                      | W     | Instantaneous power consumption out of the battery.                          |
| discharge_mwh               | mWh   | Total energy consumption by integral of system power during test run.        |
| battery_percent             | %     | Remaining battery charge percentage over full charge by design.              |
| **Sysfs Thermal**   |
| Name                        | Unit  | Note                                                                         |
| TCPU                        | C     | Temperature of the CPU.                                                      |
| x86_pkg_temp                | C     | Temperature of the x86 SoC.                                                  |
| ${thermal zone name}        | C     | Temperature of the a thermal zone, depending on device support.              |
| **Package C States** |
| Name                        | Unit  | Note                                                                         |
| package-C0_C1               | %     | The percent of time that the CPU is in package C0 and package C1 state.      |
| package-non-C0_C1           | %     | The percent of time that the CPU is *not* in package C0 and package C1 state.|
| package-${state name}       | %     | The percent of time that the CPU is in a certain state.                      |
| **Fan**             |
| Name                        | Unit  | Note                                                                         |
| fan_${fan name}             | RPM   | Speed of each fan.                                                           |
| **GPU Usage**             |
| Name                        | Unit  | Note                                                                         |
| drm_rank${rank}\_utilization_${process_name}\_${type}                     | %     | The percentage time the GPU was active ranked by per-process utilization. Types include GPU engines for render and video. If process name is empty, it is the sum total of all processes.                                 |
| **GPU Freq**             |
| gpu_freq                    | MHz   | GPU clock frequency.                                                         |
| **Zram IO**         |
| Name                        | Unit  | Note                                                                         |
| zram_read_IOs               | time  | Number of read I/Os processed.                                               |
| zram_write_IOs              | time  | Number of write I/Os processed.                                              |
| zram_IOs_in_flight          | time  | Number of I/Os currently in flight.                                          |
| **Sysfs Battery**                   |
| Name                        | Unit  | Note                                                                         |
| minutes_battery_life        | min   | Projected user battery life from 100% to battery shut down percent (~4%).    |
| minutes_battery_life_tested | min   | Actual test running time.                                                    |
