# Metrics

This document lists the metrics collected in power package through TestMetrics().

|                             |       |                                                                              |
|---                          |---    |---                                                                           |
| **CPU Idle State Metrics**  |
| Name                        | Unit  | Note                                                                         |
| cpu-${state name}           | %     | The percent of time all CPUs spend in a certain state.                       |
| cpu[0-9]-${state name}      | %     | The percent of time a single CPU spends in a certain state.                  |
| **RAPL Power Metrics**      |
| Name                        | Unit  | Note                                                                         |
| package-0                   | W     | Energy consumption across the entire SoC.                                    |
| non_SoC                     | W     | Energy consumption of all subsystems (system total minus SoC).               |
| PL1                         | W     | A threshold that power should not exceed on average in a longer span of time.|
| core                        | W     | Energy consumption across all cpu cores.                                     |
| uncore                      | W     | Energy consumption across integrated graphics.                               |
| dram                        | W     | Energy consumption across the DRAM.                                          |
| **Sysfs Battery Metrics**   |
| Name                        | Unit  | Note                                                                         |
| system                      | W     | Instantaneous power consumption out of the battery.                          |
| discharge_mwh               | mWh   | Total energy consumption by integral of system power during test run.        |
| battery_percent             | %     | Remaining battery charge percentage over full charge by design.              |
| **Sysfs Thermal Metrics**   |
| Name                        | Unit  | Note                                                                         |
| TCPU                        | C     | Temperature of the CPU.                                                      |
| x86_pkg_temp                | C     | Temperature of the x86 SoC.                                                  |
| ${thermal zone name}        | C     | Temperature of the a thermal zone, depending on device support.              |
| **Package C State Metrics** |
| Name                        | Unit  | Note                                                                         |
| package-C0_C1               | %     | The percent of time that the CPU is in package C0 and package C1 state.      |
| package-non-C0_C1           | %     | The percent of time that the CPU is *not* in package C0 and package C1 state.|
| package-${state name}       | %     | The percent of time that the CPU is in a certain state.                      |
| **Fan Metrics**             |
| Name                        | Unit  | Note                                                                         |
| fan_${fan name}             | RPM   | Speed of each fan.                                                           |
| **GPU Metrics**             |
| Name                        | Unit  | Note                                                                         |
| gpu_rc0                     | %     | The percent of time the GPU is in RC0 state.                                 |
| gpu_rc6                     | %     | The percent of time the GPU is in RC6 state.                                 |
| gpu_freq                    | MHz   | GPU clock frequency.                                                         |
| **Zram IO Metrics**         |
| Name                        | Unit  | Note                                                                         |
| zram_read_IOs               | time  | Number of read I/Os processed.                                               |
| zram_write_IOs              | time  | Number of write I/Os processed.                                              |
| zram_IOs_in_flight          | time  | Number of I/Os currently in flight.                                          |
| **Other**                   |
| Name                        | Unit  | Note                                                                         |
| minutes_battery_life        | min   | Projected user battery life from 100% to battery shut down percent (~4%).    |
| minutes_battery_life_tested | min   | Actual test running time.                                                    |
