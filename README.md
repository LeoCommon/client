# Client

Is the system management and task scheduling daemon for SATOS. The following functionality is currently being provided.

---

> ### ⚠️ Deprecation Notice
>
> All network configuration commands (**set_network_conn**, **set_wifi_config**, **set_eth_config**, and **set_gsm_config**) are now considered **deprecated**. Please use SATOS native tools for future implementations.

---

## Commands

| Commands               | Arguments                                                                                                         | Description                                                                                           |
| :--------------------- | :---------------------------------------------------------------------------------------------------------------- | :---------------------------------------------------------------------------------------------------- |
| **get_status**         | -- none --                                                                                                        | push a brief status into the db-entry of the device                                                   |
| **get_full_status**    | -- none --                                                                                                        | get a full status report file of the device                                                           |
| **iridium_sniffing**   | `centerfrequency_mhz:1624;bandwidth_mhz:5;gain:14;if_gain:40;bb_gain:20`                                          | perform a iridium sniffing with the given parameters (sample_rate = bandwidth, max 24h long)          |
| **get_logs**           | `service:client.service`                                                                                          | get the logs (since reboot) of the specified service (default: client.service)                        |
| **reboot**             | -- none --                                                                                                        | (currently not working) carefully reboots the client system                                           |
| **reset**              | -- none --                                                                                                        | force reboots the client system                                                                       |
| **get_sys_config**     | `type:all,shortcut`                                                                                               | all (default): returns system configs. shortcut: same as 'all' but configs are returned as error-code |
| **set_sys_config**     | `job_temp_path:/run/client/jobs/;job_storage_path:/data/jobs/;polling_interval:60s;upload_chunksize_byte:1000000` | polling_intervall requires reboot                                                                     |
| **force_provisioning** | -- none --                                                                                                        | Force the ground station into the SPOT Portal (closes automatically if no interaction is detected)    |

### Deprecated Network Commands

| Commands             | Arguments                                                                                                                                                                                   | Description                                                 |
| :------------------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | :---------------------------------------------------------- |
| **set_network_conn** | `eth:on;wifi:off;gsm:on`                                                                                                                                                                    | turn on/off network interfaces (until reboot)               |
| **set_wifi_config**  | `autoconnect:true;ssid:wifiName;psk:wifiPassword;methodIPv4:auto;dnsIPv4:8.8.8.8` <br><br> **OR** <br><br> `methodIPv4:manual;addressesIPv4:1.2.3.4/24;gatewayIPv4:1.2.3.4;dnsIPv4:8.8.8.8` | set wifi-config (default setting or manual ipv4 config)     |
| **set_eth_config**   | `autoconnect:true;methodIPv4:auto;dnsIPv4:8.8.8.8` <br><br> **OR** <br><br> `methodIPv4:manual;addressesIPv4:1.2.3.4/24;gatewayIPv4:1.2.3.4;dnsIPv4:8.8.8.8`                                | set ethernet-config (default setting or manual ipv4 config) |
| **set_gsm_config**   | -- none --                                                                                                                                                                                  | (currently not working)                                     |

---

## Building

GO 1.26 or later is required for building the source code of this package externally. However, if the project is used within SATOS, Buildroot automatically builds the required tooling.

### Manual building instructions

Clone the project and execute: `make` to build all targets. The test version of the `modem_manager` can be invoked by using `make run`.

## Dependencies

This client uses third-party libraries that are automatically managed. They reside in the `go.mod` file and their sources are stored within the `vendor` directory.
