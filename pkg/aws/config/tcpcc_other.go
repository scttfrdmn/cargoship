//go:build !linux

package config

import (
	"errors"
	"runtime"
)

func probeHost() hostCongestion {
	return hostCongestion{
		supported: false,
		reason:    "per-socket TCP congestion control is only supported on Linux (this is " + runtime.GOOS + ")",
	}
}

func setSocketCongestion(uintptr, string) error {
	return errors.New("per-socket TCP congestion control is only supported on Linux")
}
