/*
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Config holds all configuration for the application.
type Config struct {
	MulticastAddress string
	Interface        string        // bind to specific NIC (e.g. "eth0")
	DBusName         string
	SMASusyID        uint32
	IsEnergyMeter    bool
	LogLevel         string
	DiagnoseMode     bool
	StaleTimeout     time.Duration
	ShowVersion      bool
}

// ParseConfig parses CLI flags with env-var fallbacks for backward compatibility.
func ParseConfig() Config {
	var cfg Config
	var susyID uint64
	var staleSeconds int

	flag.StringVar(&cfg.MulticastAddress, "multicast-address", envOrDefault("MULTICAST_ADDRESS", "239.12.255.254:9522"), "SMA multicast address:port")
	flag.StringVar(&cfg.Interface, "interface", envOrDefault("INTERFACE", ""), "network interface to bind (e.g. eth0)")
	flag.StringVar(&cfg.DBusName, "dbus-name", envOrDefault("DBUS_NAME", "com.victronenergy.grid.cgwacs_ttyUSB0_di30_mb1"), "D-Bus service name")
	flag.StringVar(&cfg.LogLevel, "log-level", envOrDefault("LOG_LEVEL", "info"), "log level (debug, info, warn, error)")
	flag.BoolVar(&cfg.IsEnergyMeter, "energy-meter", envBoolOrDefault("SMA_ENERGY_METER", false), "SMA Energy Meter 1.0 mode (vs SHM 2.0)")
	flag.BoolVar(&cfg.DiagnoseMode, "diagnose", false, "run diagnostic checks and exit")
	flag.BoolVar(&cfg.ShowVersion, "version", false, "print version and exit")
	flag.IntVar(&staleSeconds, "stale-timeout", envIntOrDefault("STALE_TIMEOUT", 30), "seconds without data before marking stale")

	susyDefault := envOrDefault("SMASUSYID", "0")
	flag.Uint64Var(&susyID, "susy-id", parseUint64(susyDefault), "SMA SUSy ID filter (0 = accept all)")

	flag.Parse()

	cfg.SMASusyID = uint32(susyID)
	cfg.StaleTimeout = time.Duration(staleSeconds) * time.Second

	return cfg
}

func envOrDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func envBoolOrDefault(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

func envIntOrDefault(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func parseUint64(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}
