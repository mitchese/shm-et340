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
	"fmt"

	"github.com/godbus/dbus/v5"
)

// InitializeValues sets up the initial D-Bus values.
func (a *App) InitializeValues() {
	a.mu.Lock()
	defer a.mu.Unlock()

	set := func(path string, value, text interface{}) {
		a.values[0][objectpath(path)] = dbus.MakeVariant(value)
		a.values[1][objectpath(path)] = dbus.MakeVariant(text)
	}

	// Device identity
	set("/Connected", 1, "1")
	set("/CustomName", "Grid meter", "Grid meter")
	set("/DeviceInstance", 30, "30")
	set("/DeviceType", 71, "71")
	set("/ErrorCode", 0, "0")
	set("/FirmwareVersion", 2, "2")
	set("/ProductName", "Grid meter", "Grid meter")
	set("/ProductId", 45058, "45058")
	set("/Role", "grid", "grid")
	set("/Position", 0, "0")

	// Process identity
	set("/Mgmt/ProcessName", "shm-et340", "shm-et340")
	set("/Mgmt/Connection", fmt.Sprintf("Multicast UDP %s", a.config.MulticastAddress), fmt.Sprintf("Multicast UDP %s", a.config.MulticastAddress))
	set("/Mgmt/ProcessVersion", Version, Version)

	// Serial — overwritten on first packet with real SMA serial
	set("/Serial", "detecting...", "detecting...")

	// RefreshTime: data update interval in seconds (ET340 multicast updates every 1s)
	set("/RefreshTime", int32(1), "1")

	// Per-phase values initialized to zero
	for _, phase := range []string{"/Ac/L1", "/Ac/L2", "/Ac/L3"} {
		set(phase+"/Power", 0.0, "0.0 W")
		set(phase+"/Voltage", 0.0, "0.00 V")
		set(phase+"/Current", 0.0, "0.00 A")
		set(phase+"/Energy/Forward", 0.0, "0.00 kWh")
		set(phase+"/Energy/Reverse", 0.0, "0.00 kWh")
	}

	// Totals initialized to zero
	set("/Ac/Power", 0.0, "0.0 W")
	set("/Ac/Current", 0.0, "0.00 A")
	set("/Ac/Voltage", 0.0, "0.00 V")
	set("/Ac/Energy/Forward", 0.0, "0.00 kWh")
	set("/Ac/Energy/Reverse", 0.0, "0.00 kWh")
}
