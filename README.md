# APOGEE
APOGEE is the system management and task scheduling daemon for SATOS. The following functionality is currently being provided.

## Commands
| Commands         | Arguments                                 | Description                                          |
|------------------|-------------------------------------------|------------------------------------------------------|
| get_status       | -- none --                                | push a brief status into the db-entry of the device  |
| get_full_status  | -- none --                                | get a full status report file of the device          |
| iridium_sniffing | centerfrequency_mhz:1624;bandwidth_mhz:5;gain:14;if_gain:40;bb_gain:20 | perform a iridium sniffing with the given parameters (sample_rate = bandwidth) |
| get_logs         | service:apogee_client.service             | get the logs (since reboot) of the specified service (default: apogee_client.service) |
| reboot           | -- none --                                | (currently not working) carefully reboots the client system  |
| reset            | -- none --                                | force reboots the client system                      |
| set_network_conn | eth:on;wifi:off;gsm:on                    | turn on/off network interfaces (until reboot)        |
| set_wifi_config  | autoconnect:true;ssid:wifiName;psk:wifiPassword;methodIPv4:auto;dnsIPv4:8.8.8.8 <br> methodIPv4:manual;addressesIPv4:1.2.3.4/24;gatewayIPv4:1.2.3.4;dnsIPv4:8.8.8.8| set wifi-config (default setting) <br> (manual ipv4 config)|
| set_eth_config   | autoconnect:true;methodIPv4:auto;dnsIPv4:8.8.8.8 <br> methodIPv4:manual;addressesIPv4:1.2.3.4/24;gatewayIPv4:1.2.3.4;dnsIPv4:8.8.8.8| set ethernet-config (default setting) <br> (manual ipv4 config)|
| set_gsm_config   | -- none --                                |  (curretnly not working)                             |
| get_sys_config   | type:all,shortcut                         |  all (default): returns system configs. shortcut: same as 'all' but configs are returned as error-code (case of filesystem misconfiguration)|
| set_sys_config   | job_temp_path:/run/apogee/jobs/;job_storage_path:/data/jobs/;polling_interval:60s | polling_intervall requires reboot |
|                  |                                           |                                                      |

autoconnect:true;ssid:wifiNameFoo;psk:wifiPasswordFoo;methodIPv4:manual;addressesIPv4:1.2.3.4/24;gatewayIPv4:1.2.3.4;dnsIPv4:8.8.8.8

## (Planned) Functionality
- [x] Modem GPS Starting
- [x] Task scheduling
- [x] D-Bus integration for NetworkManager
- [ ] ...

## Building
GO 1.18 or later is required for building the source code of this package externally. However, if the project is used within SATOS, Buildroot automatically builds the required tooling.

### Manual building instructions
Clone the project and execute: `make` to build all targets. The test version of the `modem_manager` can be invoked by using `make run`.

## Dependencies
APOGEE uses third-party libraries that are automatically managed. 
They reside in the `go.mod` file and their sources are stored within the `vendor` directory.

## Contact
In case of issues or questions contact [Martin Böh](mailto:contact@martb.dev)
