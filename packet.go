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
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"time"

	"github.com/godbus/dbus/v5"
	log "github.com/sirupsen/logrus"
)

// Packet layout constants
const (
	headerSize = 28

	// Offsets within the packet for total values
	offsetTotalPurchasePower = 32
	offsetTotalSellPower     = 52
	offsetTotalForwardEnergy = 40
	offsetTotalReverseEnergy = 60

	// Protocol tag offset and expected value
	offsetProtocolTag = 16
	protocolTagValue  = 24681

	// SUSy ID / serial offset
	offsetSerial = 20

	// Minimum packet sizes
	minPacketSizeSHM          = 596
	minPacketSizeEnergyMeter  = 588

	// Phase chunk offsets for SHM 2.0
	shmPhaseL1Start = 164
	shmPhaseL2Start = 308
	shmPhaseL3Start = 452

	// Phase chunk offsets for Energy Meter 1.0
	emPhaseL1Start = 156
	emPhaseL2Start = 300
	emPhaseL3Start = 444

	// Phase chunk size
	phaseChunkSize = 144
)

// singlePhase holds decoded per-phase electrical data.
type singlePhase struct {
	voltage float32 // Volts
	current float32 // Amps (derived from power/voltage)
	power   float32 // Watts (purchase - sell)
	forward float64 // kWh, purchased energy
	reverse float64 // kWh, sold energy
}

// decodePhaseChunk decodes a 144-byte phase data chunk.
func decodePhaseChunk(b []byte) *singlePhase {
	purchaseW := float32(binary.BigEndian.Uint32(b[4:8])) / 10.0
	sellW := float32(binary.BigEndian.Uint32(b[24:28])) / 10.0

	forwardkWh := float64(binary.BigEndian.Uint64(b[12:20])) / 3600.0 / 1000.0
	reversekWh := float64(binary.BigEndian.Uint64(b[32:40])) / 3600.0 / 1000.0

	L := singlePhase{}
	L.voltage = float32(binary.BigEndian.Uint32(b[132:136])) / 1000.0 // millivolts
	L.power = purchaseW - sellW

	// Guard against division by zero
	if L.voltage > 0 {
		L.current = L.power / L.voltage
	}

	L.forward = forwardkWh
	L.reverse = reversekWh

	return &L
}

// HandleMessage processes a received SMA multicast packet.
func (a *App) HandleMessage(src *net.UDPAddr, n int, b []byte) {
	// Rate-limit: max 1 update/second
	a.mu.Lock()
	if time.Since(a.lastEmit) < time.Second {
		a.mu.Unlock()
		return
	}
	a.lastEmit = time.Now()
	a.mu.Unlock()

	// Validate minimum header size
	if n < headerSize {
		log.Debug("Packet too small for header: ", n)
		return
	}

	// Filter broadcast packets (not from the meter)
	if binary.BigEndian.Uint16(b[offsetProtocolTag:offsetProtocolTag+2]) != protocolTagValue {
		log.Debug("Broadcast packet, not from meter")
		return
	}

	// Validate packet size based on meter type
	minSize := minPacketSizeSHM
	if a.config.IsEnergyMeter {
		minSize = minPacketSizeEnergyMeter
	}
	if n < minSize {
		log.Debugf("Packet too small (%d bytes, need %d)", n, minSize)
		return
	}

	// Check SUSy ID filter
	serial := binary.BigEndian.Uint32(b[offsetSerial : offsetSerial+4])
	if a.config.SMASusyID > 0 && a.config.SMASusyID != serial {
		log.Debugf("Ignoring packet from serial %d (filtering for %d)", serial, a.config.SMASusyID)
		return
	}

	log.Debug("----------------------")
	log.Debug("Received datagram from meter")

	// Update last received timestamp
	a.mu.Lock()
	a.lastReceived = time.Now()
	a.mu.Unlock()

	// Auto-detect serial from first valid packet
	if !a.serialSet {
		a.mu.Lock()
		a.serialSet = true
		serialStr := fmt.Sprintf("%d", serial)
		a.values[0]["/Serial"] = dbus.MakeVariant(serialStr)
		a.values[1]["/Serial"] = dbus.MakeVariant(serialStr)
		a.mu.Unlock()
		log.Infof("Detected SMA serial: %s", serialStr)
	}

	// Decode total values
	totalPurchasePower := float32(binary.BigEndian.Uint32(b[offsetTotalPurchasePower : offsetTotalPurchasePower+4]))
	totalSellPower := float32(binary.BigEndian.Uint32(b[offsetTotalSellPower : offsetTotalSellPower+4]))
	totalPower := (totalPurchasePower - totalSellPower) / 10.0

	totalForward := float64(binary.BigEndian.Uint64(b[offsetTotalForwardEnergy:offsetTotalForwardEnergy+8])) / 3600.0 / 1000.0
	totalReverse := float64(binary.BigEndian.Uint64(b[offsetTotalReverseEnergy:offsetTotalReverseEnergy+8])) / 3600.0 / 1000.0

	// Decode per-phase values
	var L1, L2, L3 *singlePhase
	if a.config.IsEnergyMeter {
		L1 = decodePhaseChunk(b[emPhaseL1Start : emPhaseL1Start+phaseChunkSize])
		L2 = decodePhaseChunk(b[emPhaseL2Start : emPhaseL2Start+phaseChunkSize])
		L3 = decodePhaseChunk(b[emPhaseL3Start : emPhaseL3Start+phaseChunkSize])
	} else {
		L1 = decodePhaseChunk(b[shmPhaseL1Start : shmPhaseL1Start+phaseChunkSize])
		L2 = decodePhaseChunk(b[shmPhaseL2Start : shmPhaseL2Start+phaseChunkSize])
		L3 = decodePhaseChunk(b[shmPhaseL3Start : shmPhaseL3Start+phaseChunkSize])
	}

	if log.IsLevelEnabled(log.DebugLevel) {
		PrintPhaseTable(L1, L2, L3)
	}

	// Collect all changes into a local map, then apply under a single lock
	type valUpdate struct {
		path      string
		unit      string
		value     float64
		precision int
	}

	totalCurrent := float64(L1.current + L2.current + L3.current)
	totalVoltage := float64(L1.voltage+L2.voltage+L3.voltage) / 3.0

	updates := []valUpdate{
		{"/Ac/Power", "W", float64(totalPower), 1},
		{"/Ac/Energy/Forward", "kWh", totalForward, 2},
		{"/Ac/Energy/Reverse", "kWh", totalReverse, 2},
		{"/Ac/Current", "A", totalCurrent, 2},
		{"/Ac/Voltage", "V", totalVoltage, 2},

		{"/Ac/L1/Power", "W", float64(L1.power), 1},
		{"/Ac/L1/Voltage", "V", float64(L1.voltage), 2},
		{"/Ac/L1/Current", "A", float64(L1.current), 2},
		{"/Ac/L1/Energy/Forward", "kWh", L1.forward, 2},
		{"/Ac/L1/Energy/Reverse", "kWh", L1.reverse, 2},

		{"/Ac/L2/Power", "W", float64(L2.power), 1},
		{"/Ac/L2/Voltage", "V", float64(L2.voltage), 2},
		{"/Ac/L2/Current", "A", float64(L2.current), 2},
		{"/Ac/L2/Energy/Forward", "kWh", L2.forward, 2},
		{"/Ac/L2/Energy/Reverse", "kWh", L2.reverse, 2},

		{"/Ac/L3/Power", "W", float64(L3.power), 1},
		{"/Ac/L3/Voltage", "V", float64(L3.voltage), 2},
		{"/Ac/L3/Current", "A", float64(L3.current), 2},
		{"/Ac/L3/Energy/Forward", "kWh", L3.forward, 2},
		{"/Ac/L3/Energy/Reverse", "kWh", L3.reverse, 2},
	}

	changedItems := make(map[string]map[string]dbus.Variant)

	a.mu.Lock()
	for _, u := range updates {
		if !isFinite(u.value) {
			continue
		}

		formatStr := fmt.Sprintf("%%.%df %s", u.precision, u.unit)
		textValue := fmt.Sprintf(formatStr, u.value)

		currentValue, exists := a.values[0][objectpath(u.path)]
		if !exists || currentValue.Value() != u.value {
			a.values[0][objectpath(u.path)] = dbus.MakeVariant(u.value)
			a.values[1][objectpath(u.path)] = dbus.MakeVariant(textValue)

			changedItems[u.path] = map[string]dbus.Variant{
				"Value": dbus.MakeVariant(u.value),
				"Text":  dbus.MakeVariant(textValue),
			}
		}
	}
	a.mu.Unlock()

	a.emitItemsChanged(changedItems)

	log.Infof("Meter update received and published to D-Bus: %.1f W", totalPower)
}

// isFinite returns true if the value is neither NaN nor Inf.
func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// PrintPhaseTable logs a debug table of per-phase values.
func PrintPhaseTable(L1, L2, L3 *singlePhase) {
	log.Println("+-----+-------------+---------------+---------------+")
	log.Println("|value|   L1 \t|     L2  \t|   L3  \t|")
	log.Println("+-----+-------------+---------------+---------------+")
	log.Printf("|  V  | %8.2f \t| %8.2f \t| %8.2f \t|", L1.voltage, L2.voltage, L3.voltage)
	log.Printf("|  A  | %8.2f \t| %8.2f \t| %8.2f \t|", L1.current, L2.current, L3.current)
	log.Printf("|  W  | %8.2f \t| %8.2f \t| %8.2f \t|", L1.power, L2.power, L3.power)
	log.Printf("| kWh | %8.2f \t| %8.2f \t| %8.2f \t|", L1.forward, L2.forward, L3.forward)
	log.Printf("| kWh | %8.2f \t| %8.2f \t| %8.2f \t|", L1.reverse, L2.reverse, L3.reverse)
	log.Println("+-----+-------------+---------------+---------------+")
}
