# APOGEE
APOGEE is the system management and task scheduling daemon for SATOS. The following functionality is currently being provided.

## Commands
| Commands         | Arguments                                 | Description                                          |
|------------------|-------------------------------------------|------------------------------------------------------|
| get_status       | -- none --                                | push a brief status into the db-entry of the device  |
| get_full_status  | -- none --                                | get a full status report file of the device          |
| upload_test_file | -- none --                                | upload a small test file to the server               |
| iridium_sniffing | centerfrequency_mhz:1624; bandwidth_mhz:5 | perform a iridium sniffing with the given cf and bw  |
| get_logs         | service:apogee-client.service             | get the logs (since reboot) of the specified service |
|                  |                                           |                                                      |

## (Planned) Functionality
- [x] Modem GPS Starting
- [ ] Task scheduling
- [ ] D-Bus integration 

## Building
GO 1.17 or later is required for building the source code of this package externally. However, if the project is used within SATOS, Buildroot automatically builds the required tooling.

### Manual building instructions
Clone the project and execute: `make` to build all targets. The test version of the `modem_manager` can be invoked by using `make run`.

## Dependencies
APOGEE uses the following automatically managed third-party libraries. 
They are listed in the `go.mod` file and copies of the sources are stored within the `vendor` directory.
- https://github.com/go-resty/resty/v2
- https://github.com/tarm/serial
- https://gopkg.in/yaml.v2
- https://github.com/pilebones/go-udev
- https://go.uber.org/zap

## Contact
In case of issues or questions contact [Martin Böh](mailto:contact@martb.dev)
