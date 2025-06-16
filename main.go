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
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/dmichael/go-multicast/multicast"
	"github.com/godbus/dbus/introspect"
	"github.com/godbus/dbus/v5"
	log "github.com/sirupsen/logrus"
)

const (
	address = "239.12.255.254:9522"
)

// Config holds all configuration for the application
type Config struct {
	MulticastAddress string
	DBusName         string
	SMASusyID        uint32
	LogLevel         string
}

// App represents the main application
type App struct {
	config     Config
	dbusConn   *dbus.Conn
	values     map[int]map[objectpath]dbus.Variant
	mu         sync.RWMutex
	shutdownCh chan struct{}
}

// NewApp creates a new application instance
func NewApp(config Config) (*App, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system bus: %w", err)
	}

	app := &App{
		config:     config,
		dbusConn:   conn,
		values:     make(map[int]map[objectpath]dbus.Variant),
		shutdownCh: make(chan struct{}),
	}

	// Initialize the values maps
	app.values[0] = make(map[objectpath]dbus.Variant) // For VALUE variant
	app.values[1] = make(map[objectpath]dbus.Variant) // For STRING variant

	return app, nil
}

type singlePhase struct {
	voltage float32 // Volts: 230,0
	a       float32 // Amps: 8,3
	power   float32 // Watts: 1909
	forward float64 // kWh, purchased power
	reverse float64 // kWh, sold power
}

const intro = `
<node>
   <interface name="com.victronenergy.BusItem">
    <signal name="PropertiesChanged">
      <arg type="a{sv}" name="properties" />
    </signal>
    <method name="SetValue">
      <arg direction="in"  type="v" name="value" />
      <arg direction="out" type="i" />
    </method>
    <method name="GetText">
      <arg direction="out" type="s" />
    </method>
    <method name="GetValue">
      <arg direction="out" type="v" />
    </method>
	</interface>` + introspect.IntrospectDataString + `</node> `

type objectpath string

var victronValues = map[int]map[objectpath]dbus.Variant{
	// 0: This will be used to store the VALUE variant
	0: map[objectpath]dbus.Variant{},
	// 1: This will be used to store the STRING variant
	1: map[objectpath]dbus.Variant{},
}

// GetValue returns the value variant for the object path
func (f objectpath) GetValue() (dbus.Variant, *dbus.Error) {
	log.Debug("GetValue() called for ", f)
	app := GetApp()
	if app == nil {
		return dbus.Variant{}, dbus.NewError("com.victronenergy.BusItem.Error", []interface{}{"Application not initialized"})
	}
	app.mu.RLock()
	defer app.mu.RUnlock()
	log.Debug("...returning ", app.values[0][f])
	return app.values[0][f], nil
}

// GetText returns the text variant for the object path
func (f objectpath) GetText() (string, *dbus.Error) {
	log.Debug("GetText() called for ", f)
	app := GetApp()
	if app == nil {
		return "", dbus.NewError("com.victronenergy.BusItem.Error", []interface{}{"Application not initialized"})
	}
	app.mu.RLock()
	defer app.mu.RUnlock()
	log.Debug("...returning ", app.values[1][f])
	return strings.Trim(app.values[1][f].String(), "\""), nil
}

// SetValue sets the value for the object path
func (f objectpath) SetValue(value dbus.Variant) (int32, *dbus.Error) {
	log.Debug("SetValue() called for ", f, " with value ", value)
	app := GetApp()
	if app == nil {
		return 0, dbus.NewError("com.victronenergy.BusItem.Error", []interface{}{"Application not initialized"})
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	app.values[0][f] = value
	return 0, nil
}

// globalApp is used to store the application instance for the objectpath methods
var globalApp *App

// GetApp returns the global application instance
func GetApp() *App {
	return globalApp
}

// SetApp sets the global application instance
func SetApp(app *App) {
	globalApp = app
}

func init() {
	lvl, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		lvl = "info"
	}

	ll, err := log.ParseLevel(lvl)
	if err != nil {
		ll = log.DebugLevel
	}

	log.SetLevel(ll)
}

// HandleMessage processes incoming multicast messages
func (a *App) HandleMessage(src *net.UDPAddr, n int, b []byte) {
	// Check protocol ID
	if 24681 != binary.BigEndian.Uint16(b[16:18]) {
		log.Debug("The protocol ID didn't match 0x6069, it's not a meter update. ProtocolID: ", binary.BigEndian.Uint16(b[16:18]))
		return
	}

	if binary.BigEndian.Uint32(b[20:24]) == 0xffffffff {
		log.Debug("Implausible serial, rejecting")
		return
	}

	if a.config.SMASusyID > 0 && uint32(a.config.SMASusyID) != binary.BigEndian.Uint32(b[20:24]) {
		log.Debugf("Oops, I was told to only listen for updates from %d, but this update is from %d",
			a.config.SMASusyID, binary.BigEndian.Uint32(b[20:24]))
		return
	}

	if n < 500 {
		log.Debug("Received packet is probably too small. Size: ", n)
		log.Debug("Serial: ", binary.BigEndian.Uint32(b[20:24]))
		return
	}

	log.Debug("Uid: ", binary.BigEndian.Uint32(b[4:8]))
	log.Debug("Serial: ", binary.BigEndian.Uint32(b[20:24]))

	// Calculate total power (buy - sell) in watts
	powertot := ((float32(binary.BigEndian.Uint32(b[32:36])) - float32(binary.BigEndian.Uint32(b[52:56]))) / 10.0)

	// Convert watt seconds to kWh
	bezugtot := float64(binary.BigEndian.Uint64(b[40:48])) / 3600.0 / 1000.0
	einsptot := float64(binary.BigEndian.Uint64(b[60:68])) / 3600.0 / 1000.0

	log.Debug("Total W: ", powertot)
	log.Debug("Total Buy kWh: ", bezugtot)
	log.Debug("Total Sell kWh: ", einsptot)

	log.Info(fmt.Sprintf("Meter update received: %.2f kWh bought and %.2f kWh sold, %.1f W currently flowing",
		bezugtot, einsptot, powertot))

	a.UpdateVariant(float64(powertot), "W", "/Ac/Power")
	a.UpdateVariant(float64(einsptot), "kWh", "/Ac/Energy/Reverse")
	a.UpdateVariant(float64(bezugtot), "kWh", "/Ac/Energy/Forward")

	L1 := decodePhaseChunk(b[164:308])
	L2 := decodePhaseChunk(b[308:452])
	L3 := decodePhaseChunk(b[452:596])

	log.Debug("+-----+-------------+---------------+---------------+")
	log.Debug("|value|   L1 \t|     L2  \t|   L3  \t|")
	log.Debug("+-----+-------------+---------------+---------------+")
	log.Debug(fmt.Sprintf("|  V  | %8.2f \t| %8.2f \t| %8.2f \t|", L1.voltage, L2.voltage, L3.voltage))
	log.Debug(fmt.Sprintf("|  A  | %8.2f \t| %8.2f \t| %8.2f \t|", L1.a, L2.a, L3.a))
	log.Debug(fmt.Sprintf("|  W  | %8.2f \t| %8.2f \t| %8.2f \t|", L1.power, L2.power, L3.power))
	log.Debug(fmt.Sprintf("| kWh | %8.2f \t| %8.2f \t| %8.2f \t|", L1.forward, L2.forward, L3.forward))
	log.Debug(fmt.Sprintf("| kWh | %8.2f \t| %8.2f \t| %8.2f \t|", L1.reverse, L2.reverse, L3.reverse))
	log.Debug("+-----+-------------+---------------+---------------+")

	// Update L1 values
	a.UpdateVariant(float64(L1.power), "W", "/Ac/L1/Power")
	a.UpdateVariant(float64(L1.voltage), "V", "/Ac/L1/Voltage")
	a.UpdateVariant(float64(L1.a), "A", "/Ac/L1/Current")
	a.UpdateVariant(L1.forward, "kWh", "/Ac/L1/Energy/Forward")
	a.UpdateVariant(L1.reverse, "kWh", "/Ac/L1/Energy/Reverse")

	// Update L2 values
	a.UpdateVariant(float64(L2.power), "W", "/Ac/L2/Power")
	a.UpdateVariant(float64(L2.voltage), "V", "/Ac/L2/Voltage")
	a.UpdateVariant(float64(L2.a), "A", "/Ac/L2/Current")
	a.UpdateVariant(L2.forward, "kWh", "/Ac/L2/Energy/Forward")
	a.UpdateVariant(L2.reverse, "kWh", "/Ac/L2/Energy/Reverse")

	// Update L3 values
	a.UpdateVariant(float64(L3.power), "W", "/Ac/L3/Power")
	a.UpdateVariant(float64(L3.voltage), "V", "/Ac/L3/Voltage")
	a.UpdateVariant(float64(L3.a), "A", "/Ac/L3/Current")
	a.UpdateVariant(L3.forward, "kWh", "/Ac/L3/Energy/Forward")
	a.UpdateVariant(L3.reverse, "kWh", "/Ac/L3/Energy/Reverse")
}

// Run starts the application
func (a *App) Run() error {
	// Initialize DBus values
	a.InitializeValues()

	// Register DBus paths
	if err := a.RegisterDBusPaths(); err != nil {
		return fmt.Errorf("failed to register DBus paths: %w", err)
	}

	log.Info("Successfully connected to dbus and registered as a meter... Commencing reading of the SMA meter")

	// Start multicast listener
	multicast.Listen(a.config.MulticastAddress, a.HandleMessage)

	// Wait for shutdown signal
	<-a.shutdownCh
	return nil
}

// Shutdown gracefully stops the application
func (a *App) Shutdown() {
	close(a.shutdownCh)
	if a.dbusConn != nil {
		a.dbusConn.Close()
	}
}

func main() {
	// Configure logging
	lvl, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		lvl = "info"
	}

	ll, err := log.ParseLevel(lvl)
	if err != nil {
		ll = log.DebugLevel
	}
	log.SetLevel(ll)

	// Create configuration
	config := Config{
		MulticastAddress: "239.12.255.254:9522",
		DBusName:         "com.victronenergy.grid.cgwacs_ttyUSB0_di30_mb1",
		LogLevel:         lvl,
	}

	// Parse SMA Susy ID if provided
	if smasusyIDStr := os.Getenv("SMASUSYID"); smasusyIDStr != "" {
		if id, err := strconv.ParseUint(smasusyIDStr, 10, 32); err == nil {
			config.SMASusyID = uint32(id)
		}
	}

	// Create and run application
	app, err := NewApp(config)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// Set the global app instance
	SetApp(app)

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		app.Shutdown()
	}()

	// Run the application
	if err := app.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}

func decodePhaseChunk(b []byte) *singlePhase {

	// why does this measure in 1/10 of watts?!
	bezugW := float32(binary.BigEndian.Uint32(b[4:8])) / 10.0
	einspeiseW := float32(binary.BigEndian.Uint32(b[24:28])) / 10.0

	// this is in watt seconds ... chagne to kilo(100)watthour(3600)s:
	bezugkWh := float64(binary.BigEndian.Uint64(b[12:20])) / 3600.0 / 1000.0
	einspeisekWh := float64(binary.BigEndian.Uint64(b[32:40])) / 3600.0 / 1000.0

	// not used, but leaving here for future
	//bezugVA := float32(binary.BigEndian.Uint32(b[84:88])) / 10
	//einspeiseVA := float32(binary.BigEndian.Uint32(b[104:108])) / 10

	L := singlePhase{}
	L.voltage = float32(binary.BigEndian.Uint32(b[132:136])) / 1000 // millivolts!
	L.power = bezugW - einspeiseW
	L.a = L.power / L.voltage
	L.forward = bezugkWh
	L.reverse = einspeisekWh

	return &L
	//log.Println(phase, "Buy: ", float32(binary.BigEndian.Uint32(b[4:8]))/10)
	//log.Println(phase, "Sell: ", float32(binary.BigEndian.Uint32(b[24:28]))/10)
	//return
}

// RegisterDBusPaths registers all the DBus paths for the application
func (a *App) RegisterDBusPaths() error {
	reply, err := a.dbusConn.RequestName(a.config.DBusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("failed to request DBus name: %w", err)
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("name %s already taken on dbus", a.config.DBusName)
	}

	basicPaths := []dbus.ObjectPath{
		"/Connected",
		"/CustomName",
		"/DeviceInstance",
		"/DeviceType",
		"/ErrorCode",
		"/FirmwareVersion",
		"/Mgmt/Connection",
		"/Mgmt/ProcessName",
		"/Mgmt/ProcessVersion",
		"/Position",
		"/ProductId",
		"/ProductName",
		"/Serial",
	}

	updatingPaths := []dbus.ObjectPath{
		"/Ac/L1/Power",
		"/Ac/L2/Power",
		"/Ac/L3/Power",
		"/Ac/L1/Voltage",
		"/Ac/L2/Voltage",
		"/Ac/L3/Voltage",
		"/Ac/L1/Current",
		"/Ac/L2/Current",
		"/Ac/L3/Current",
		"/Ac/L1/Energy/Forward",
		"/Ac/L2/Energy/Forward",
		"/Ac/L3/Energy/Forward",
		"/Ac/L1/Energy/Reverse",
		"/Ac/L2/Energy/Reverse",
		"/Ac/L3/Energy/Reverse",
	}

	for _, s := range basicPaths {
		log.Debug("Registering dbus basic path: ", s)
		a.dbusConn.Export(objectpath(s), s, "com.victronenergy.BusItem")
		a.dbusConn.Export(introspect.Introspectable(intro), s, "org.freedesktop.DBus.Introspectable")
	}

	for _, s := range updatingPaths {
		log.Debug("Registering dbus update path: ", s)
		a.dbusConn.Export(objectpath(s), s, "com.victronenergy.BusItem")
		a.dbusConn.Export(introspect.Introspectable(intro), s, "org.freedesktop.DBus.Introspectable")
	}

	return nil
}

// InitializeValues sets up the initial DBus values
func (a *App) InitializeValues() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Basic device information
	a.values[0]["/Connected"] = dbus.MakeVariant(1)
	a.values[1]["/Connected"] = dbus.MakeVariant("1")

	a.values[0]["/CustomName"] = dbus.MakeVariant("Grid meter")
	a.values[1]["/CustomName"] = dbus.MakeVariant("Grid meter")

	a.values[0]["/DeviceInstance"] = dbus.MakeVariant(30)
	a.values[1]["/DeviceInstance"] = dbus.MakeVariant("30")

	a.values[0]["/DeviceType"] = dbus.MakeVariant(71)
	a.values[1]["/DeviceType"] = dbus.MakeVariant("71")

	a.values[0]["/ErrorCode"] = dbus.MakeVariantWithSignature(0, dbus.SignatureOf(123))
	a.values[1]["/ErrorCode"] = dbus.MakeVariant("0")

	a.values[0]["/FirmwareVersion"] = dbus.MakeVariant(2)
	a.values[1]["/FirmwareVersion"] = dbus.MakeVariant("2")

	a.values[0]["/Mgmt/Connection"] = dbus.MakeVariant("/dev/ttyUSB0")
	a.values[1]["/Mgmt/Connection"] = dbus.MakeVariant("/dev/ttyUSB0")

	a.values[0]["/Mgmt/ProcessName"] = dbus.MakeVariant("/opt/color-control/dbus-cgwacs/dbus-cgwacs")
	a.values[1]["/Mgmt/ProcessName"] = dbus.MakeVariant("/opt/color-control/dbus-cgwacs/dbus-cgwacs")

	a.values[0]["/Mgmt/ProcessVersion"] = dbus.MakeVariant("1.8.0")
	a.values[1]["/Mgmt/ProcessVersion"] = dbus.MakeVariant("1.8.0")

	a.values[0]["/Position"] = dbus.MakeVariantWithSignature(0, dbus.SignatureOf(123))
	a.values[1]["/Position"] = dbus.MakeVariant("0")

	a.values[0]["/ProductId"] = dbus.MakeVariant(45058)
	a.values[1]["/ProductId"] = dbus.MakeVariant("45058")

	a.values[0]["/ProductName"] = dbus.MakeVariant("Grid meter")
	a.values[1]["/ProductName"] = dbus.MakeVariant("Grid meter")

	a.values[0]["/Serial"] = dbus.MakeVariant("BP98305081235")
	a.values[1]["/Serial"] = dbus.MakeVariant("BP98305081235")

	// Initialize power values
	a.values[0]["/Ac/L1/Power"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L1/Power"] = dbus.MakeVariant("0 W")
	a.values[0]["/Ac/L2/Power"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L2/Power"] = dbus.MakeVariant("0 W")
	a.values[0]["/Ac/L3/Power"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L3/Power"] = dbus.MakeVariant("0 W")

	// Initialize voltage values
	a.values[0]["/Ac/L1/Voltage"] = dbus.MakeVariant(230)
	a.values[1]["/Ac/L1/Voltage"] = dbus.MakeVariant("230 V")
	a.values[0]["/Ac/L2/Voltage"] = dbus.MakeVariant(230)
	a.values[1]["/Ac/L2/Voltage"] = dbus.MakeVariant("230 V")
	a.values[0]["/Ac/L3/Voltage"] = dbus.MakeVariant(230)
	a.values[1]["/Ac/L3/Voltage"] = dbus.MakeVariant("230 V")

	// Initialize current values
	a.values[0]["/Ac/L1/Current"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L1/Current"] = dbus.MakeVariant("0 A")
	a.values[0]["/Ac/L2/Current"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L2/Current"] = dbus.MakeVariant("0 A")
	a.values[0]["/Ac/L3/Current"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L3/Current"] = dbus.MakeVariant("0 A")

	// Initialize energy values
	a.values[0]["/Ac/L1/Energy/Forward"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L1/Energy/Forward"] = dbus.MakeVariant("0 kWh")
	a.values[0]["/Ac/L2/Energy/Forward"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L2/Energy/Forward"] = dbus.MakeVariant("0 kWh")
	a.values[0]["/Ac/L3/Energy/Forward"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L3/Energy/Forward"] = dbus.MakeVariant("0 kWh")

	a.values[0]["/Ac/L1/Energy/Reverse"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L1/Energy/Reverse"] = dbus.MakeVariant("0 kWh")
	a.values[0]["/Ac/L2/Energy/Reverse"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L2/Energy/Reverse"] = dbus.MakeVariant("0 kWh")
	a.values[0]["/Ac/L3/Energy/Reverse"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/L3/Energy/Reverse"] = dbus.MakeVariant("0 kWh")
}

// UpdateVariant updates a DBus value and emits the change
func (a *App) UpdateVariant(value float64, unit string, path string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	emit := make(map[string]dbus.Variant)
	emit["Text"] = dbus.MakeVariant(fmt.Sprintf("%.2f", value) + unit)
	emit["Value"] = dbus.MakeVariant(float64(value))
	a.values[0][objectpath(path)] = emit["Value"]
	a.values[1][objectpath(path)] = emit["Text"]
	a.dbusConn.Emit(dbus.ObjectPath(path), "com.victronenergy.BusItem.PropertiesChanged", emit)
}
