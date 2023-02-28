# Changes
## Version 1.0
Stable version for basic iridium sniffing.
- uses D-Bus for GPS Modem interaction
- uses GPS location
- uses ethernet or wifi if available, LTE as fallback
- system relevant jobs: reboot, get_(full_)status, get_logs, set_network_conn, set_eth/wifi_config
- one research related job: iridium_sniffing

## Version 1.1
Code quality iteration & Feature update
- uses D-Bus for NetworkManager communication
- add stdreader for easy process handling with std[out|err] access
  - tests are also available for some use-cases
- complete iridium job rewrite, now uses stdreader
- fixed file/path handling problems
- removed manual network handling code
- add reset job that forcibly resets the system
- rework reboot job to tear-down apogee first and reboot before exit

