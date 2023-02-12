[![GoDoc](https://godoc.org/github.com/Wifx/gonetworkmanager?status.svg)](https://pkg.go.dev/github.com/Wifx/gonetworkmanager)
[![Go build](https://github.com/Wifx/gonetworkmanager/workflows/Go/badge.svg)](https://github.com/Wifx/gonetworkmanager/actions?query=workflow%3AGo)

gonetworkmanager
================

Go D-Bus bindings for [NetworkManager](https://networkmanager.dev/).

## Usage

You can find some examples in the [examples](examples) directory.

## External documentations

- [NetworkManager D-Bus Spec](https://networkmanager.dev/docs/api/latest/spec.html)
- [nm-settings-dbus](https://networkmanager.dev/docs/api/latest/nm-settings-dbus.html)


## Backward compatibility

The library is most likely compatible with NetworkManager 0.9 to 1.40.

## Tests

Tested with NetworkManager 1.40.0.

There are no automated tests for this library. Tests are made manually on a best-effort basis. Unit tests PRs are welcome.

## Development and contributions

There is no active development workforce from the maintainer. PRs are welcome.

## Issues

Before reporting an issue, please test the scenario with nmcli (if possible) to ensure the problem comes from the library. 
