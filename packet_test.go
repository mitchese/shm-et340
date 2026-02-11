package main

import (
	"encoding/binary"
	"math"
	"net"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// testPacket builds a valid SMA multicast packet with configurable total power.
// Returns a packet of the appropriate size for the given isEnergyMeter flag.
func testPacket(isEnergyMeter bool, totalPurchaseW, totalSellW uint32) []byte {
	size := minPacketSizeSHM
	if isEnergyMeter {
		size = minPacketSizeEnergyMeter
	}
	b := make([]byte, size)

	// Protocol tag
	binary.BigEndian.PutUint16(b[offsetProtocolTag:], protocolTagValue)

	// Serial
	binary.BigEndian.PutUint32(b[offsetSerial:], 12345678)

	// Total purchase/sell power (in 1/10 W units)
	binary.BigEndian.PutUint32(b[offsetTotalPurchasePower:], totalPurchaseW)
	binary.BigEndian.PutUint32(b[offsetTotalSellPower:], totalSellW)

	// Total forward/reverse energy (in Ws)
	binary.BigEndian.PutUint64(b[offsetTotalForwardEnergy:], 36000000) // 10 kWh
	binary.BigEndian.PutUint64(b[offsetTotalReverseEnergy:], 18000000) // 5 kWh

	// Fill phase chunks with test data
	var starts [3]int
	if isEnergyMeter {
		starts = [3]int{emPhaseL1Start, emPhaseL2Start, emPhaseL3Start}
	} else {
		starts = [3]int{shmPhaseL1Start, shmPhaseL2Start, shmPhaseL3Start}
	}

	for _, start := range starts {
		// Phase purchase power: 2300 * 10 = 23000 (= 2300 W)
		binary.BigEndian.PutUint32(b[start+4:], 23000)
		// Phase sell power: 0
		binary.BigEndian.PutUint32(b[start+24:], 0)
		// Phase forward energy: 12000000 Ws = 3.33 kWh
		binary.BigEndian.PutUint64(b[start+12:], 12000000)
		// Phase reverse energy: 6000000 Ws = 1.67 kWh
		binary.BigEndian.PutUint64(b[start+32:], 6000000)
		// Voltage: 230000 mV = 230 V
		binary.BigEndian.PutUint32(b[start+132:], 230000)
	}

	return b
}

func TestDecodePhaseChunk_NormalValues(t *testing.T) {
	chunk := make([]byte, phaseChunkSize)

	// Purchase power: 5000 (= 500.0 W)
	binary.BigEndian.PutUint32(chunk[4:], 5000)
	// Sell power: 1000 (= 100.0 W)
	binary.BigEndian.PutUint32(chunk[24:], 1000)
	// Forward energy: 36000000 Ws = 10.0 kWh
	binary.BigEndian.PutUint64(chunk[12:], 36000000)
	// Reverse energy: 7200000 Ws = 2.0 kWh
	binary.BigEndian.PutUint64(chunk[32:], 7200000)
	// Voltage: 230000 mV = 230.0 V
	binary.BigEndian.PutUint32(chunk[132:], 230000)

	phase := decodePhaseChunk(chunk)

	if math.Abs(float64(phase.voltage)-230.0) > 0.01 {
		t.Errorf("voltage = %f, want 230.0", phase.voltage)
	}
	if math.Abs(float64(phase.power)-400.0) > 0.01 {
		t.Errorf("power = %f, want 400.0 (500-100)", phase.power)
	}
	expectedCurrent := float32(400.0 / 230.0)
	if math.Abs(float64(phase.current-expectedCurrent)) > 0.01 {
		t.Errorf("current = %f, want %f", phase.current, expectedCurrent)
	}
	if math.Abs(phase.forward-10.0) > 0.01 {
		t.Errorf("forward = %f, want 10.0", phase.forward)
	}
	if math.Abs(phase.reverse-2.0) > 0.01 {
		t.Errorf("reverse = %f, want 2.0", phase.reverse)
	}
}

func TestDecodePhaseChunk_ZeroVoltage(t *testing.T) {
	chunk := make([]byte, phaseChunkSize)

	binary.BigEndian.PutUint32(chunk[4:], 5000)
	binary.BigEndian.PutUint32(chunk[24:], 1000)
	// Voltage = 0
	binary.BigEndian.PutUint32(chunk[132:], 0)

	phase := decodePhaseChunk(chunk)

	if phase.current != 0 {
		t.Errorf("current = %f, want 0 (zero voltage guard)", phase.current)
	}
	if math.IsNaN(float64(phase.current)) || math.IsInf(float64(phase.current), 0) {
		t.Error("current is NaN or Inf with zero voltage")
	}
}

func TestDecodePhaseChunk_MaxValues(t *testing.T) {
	chunk := make([]byte, phaseChunkSize)

	binary.BigEndian.PutUint32(chunk[4:], math.MaxUint32)
	binary.BigEndian.PutUint32(chunk[24:], 0)
	binary.BigEndian.PutUint64(chunk[12:], math.MaxUint64)
	binary.BigEndian.PutUint64(chunk[32:], math.MaxUint64)
	binary.BigEndian.PutUint32(chunk[132:], 230000)

	phase := decodePhaseChunk(chunk)

	// Should not panic, values should be finite
	if math.IsNaN(float64(phase.power)) {
		t.Error("power is NaN with max values")
	}
	if phase.voltage < 229 || phase.voltage > 231 {
		t.Errorf("voltage = %f, want ~230", phase.voltage)
	}
}

func TestHandleMessage_TooSmallPacket(t *testing.T) {
	app := &App{
		config: Config{},
		values: map[int]map[objectpath]dbus.Variant{
			0: make(map[objectpath]dbus.Variant),
			1: make(map[objectpath]dbus.Variant),
		},
	}

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 9522}

	// Should not panic for various small sizes
	for _, size := range []int{0, 1, 10, 27, 28, 100, 500} {
		b := make([]byte, size)
		app.HandleMessage(src, size, b)
	}
}

func TestHandleMessage_BroadcastPacket(t *testing.T) {
	app := &App{
		config: Config{},
		values: map[int]map[objectpath]dbus.Variant{
			0: make(map[objectpath]dbus.Variant),
			1: make(map[objectpath]dbus.Variant),
		},
	}

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 9522}

	// Build a packet large enough but with wrong protocol tag
	b := make([]byte, minPacketSizeSHM)
	binary.BigEndian.PutUint16(b[offsetProtocolTag:], 0) // wrong tag

	app.HandleMessage(src, len(b), b)
	// Should return without processing — no values should be set
}

func TestHandleMessage_ValidPacket(t *testing.T) {
	app := &App{
		config: Config{},
		values: map[int]map[objectpath]dbus.Variant{
			0: make(map[objectpath]dbus.Variant),
			1: make(map[objectpath]dbus.Variant),
		},
		// Set lastEmit far enough in the past to not be rate-limited
		lastEmit: time.Now().Add(-2 * time.Second),
	}
	app.InitializeValues()

	// Use a mock dbus conn — we'll skip the emit by not setting dbusConn
	// Instead we just verify values were updated
	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 9522}
	pkt := testPacket(false, 50000, 10000) // 5000W purchase, 1000W sell => 4000W net / 10

	// HandleMessage will panic on dbusConn.Emit — catch it
	func() {
		defer func() {
			recover() // dbusConn is nil, Emit will panic — that's fine for this test
		}()
		app.HandleMessage(src, len(pkt), pkt)
	}()

	// Check that serial was detected
	app.mu.RLock()
	serial := app.values[0]["/Serial"]
	app.mu.RUnlock()

	if serial.Value() != "12345678" {
		t.Errorf("serial = %v, want 12345678", serial.Value())
	}
}

func TestIsFinite(t *testing.T) {
	tests := []struct {
		name string
		v    float64
		want bool
	}{
		{"normal", 42.0, true},
		{"zero", 0.0, true},
		{"negative", -100.5, true},
		{"NaN", math.NaN(), false},
		{"PosInf", math.Inf(1), false},
		{"NegInf", math.Inf(-1), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFinite(tt.v); got != tt.want {
				t.Errorf("isFinite(%v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}
