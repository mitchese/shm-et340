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
    <method name="GetItems">
      <arg direction="out" type="a{sa{sv}}" name="values"/>
    </method>
	</interface>` + introspect.IntrospectDataString + `</node> `

type objectpath string

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

var globalApp *App

func GetApp() *App {
	return globalApp
}

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

func (a *App) HandleMessage(src *net.UDPAddr, n int, b []byte) {

	if n < 500 {
		log.Debug("Received packet is probably too small. Size: ", n)
		log.Debug("Serial: ", binary.BigEndian.Uint32(b[20:24]))
		return
	}

	// 0-28: SMA/SUSyID/SN/Uptime
	if a.config.SMASusyID > 0 && uint32(a.config.SMASusyID) != binary.BigEndian.Uint32(b[20:24]) {
		log.Debugf("Oops, I was told to only listen for updates from %d, but this update is from %d", a.config.SMASusyID, binary.BigEndian.Uint32(b[20:24]))
		return
	}
	log.Debug("----------------------")
	log.Debug("Received datagram from meter")

	// There are some broadcast packets caught by the multicast listener, that the meter is sending to 9522.
	// See https://github.com/mitchese/shm-et340/issues/2
	if binary.BigEndian.Uint16(b[16:18]) != 24681 {
		log.Debug("This is a broadcast packet, not from the meter")
		return
	}

	changedItems := make(map[string]map[string]dbus.Variant)

	update := func(path, unit string, value float64, precision int) {
		a.mu.Lock()
		defer a.mu.Unlock()

		formatString := fmt.Sprintf("%%.%df%%s", precision)
		textValue := fmt.Sprintf(formatString, value, unit)

		currentValue, valueExists := a.values[0][objectpath(path)]

		// Only update and add to batch if the value has actually changed
		if !valueExists || currentValue.Value() != value {
			a.values[0][objectpath(path)] = dbus.MakeVariant(value)
			a.values[1][objectpath(path)] = dbus.MakeVariant(textValue)

			// Add the changed properties to our batch map.
			changedItems[path] = map[string]dbus.Variant{
				"Value": dbus.MakeVariant(value),
				"Text":  dbus.MakeVariant(textValue),
			}
		}
	}

	powertot := ((float32(binary.BigEndian.Uint32(b[32:36])) - float32(binary.BigEndian.Uint32(b[52:56]))) / 10.0)
	bezugtot := float64(binary.BigEndian.Uint64(b[40:48])) / 3600.0 / 1000.0
	einsptot := float64(binary.BigEndian.Uint64(b[60:68])) / 3600.0 / 1000.0
	L1 := decodePhaseChunk(b[164:308])
	L2 := decodePhaseChunk(b[308:452])
	L3 := decodePhaseChunk(b[452:596])

	// --- Use the new helper to batch updates with correct formatting ---
	// Using 1 decimal for power, 2 for energy/voltage/current is a safe bet.
	// First, Totals
	update("/Ac/Power", "W", float64(powertot), 1)
	update("/Ac/Energy/Reverse", "kWh", einsptot, 2)
	update("/Ac/Energy/Forward", "kWh", bezugtot, 2)
	totalCurrent := L1.a + L2.a + L3.a
	totalVoltage := (L1.voltage + L2.voltage + L3.voltage) / 3.0
	update("/Ac/Current", "A", float64(totalCurrent), 2)
	update("/Ac/Voltage", "V", float64(totalVoltage), 2)

	// Update L1 values
	update("/Ac/L1/Power", "W", float64(L1.power), 1)
	update("/Ac/L1/Voltage", "V", float64(L1.voltage), 2)
	update("/Ac/L1/Current", "A", float64(L1.a), 2)
	update("/Ac/L1/Energy/Forward", "kWh", L1.forward, 2)
	update("/Ac/L1/Energy/Reverse", "kWh", L1.reverse, 2)

	// Update L2 values
	update("/Ac/L2/Power", "W", float64(L2.power), 1)
	update("/Ac/L2/Voltage", "V", float64(L2.voltage), 2)
	update("/Ac/L2/Current", "A", float64(L2.a), 2)
	update("/Ac/L2/Energy/Forward", "kWh", L2.forward, 2)
	update("/Ac/L2/Energy/Reverse", "kWh", L2.reverse, 2)

	// Update L3 values
	update("/Ac/L3/Power", "W", float64(L3.power), 1)
	update("/Ac/L3/Voltage", "V", float64(L3.voltage), 2)
	update("/Ac/L3/Current", "A", float64(L3.a), 2)
	update("/Ac/L3/Energy/Forward", "kWh", L3.forward, 2)
	update("/Ac/L3/Energy/Reverse", "kWh", L3.reverse, 2)

	// finally, post the updates
	a.emitItemsChanged(changedItems)

	log.Info(fmt.Sprintf("Meter update received and published to D-Bus: %.1f W", powertot))
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
		log.Info("Received shutdown signal, cleaning up...")
		app.Shutdown()
		os.Exit(0)
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

	// this is in watt seconds ... chagne to kilo(1000)watthour(3600)s:
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

func (a *App) RegisterDBusPaths() error {
	paths := []dbus.ObjectPath{
		// Basic Paths whch never change
		"/Connected", "/CustomName", "/DeviceInstance", "/DeviceType",
		"/ErrorCode", "/FirmwareVersion", "/Mgmt/Connection", "/Mgmt/ProcessName",
		"/Mgmt/ProcessVersion", "/ProductName", "/Serial",
		// Updating Paths, which change every time the meter sends a packet
		"/Ac/L1/Power", "/Ac/L2/Power", "/Ac/L3/Power",
		"/Ac/L1/Voltage", "/Ac/L2/Voltage", "/Ac/L3/Voltage",
		"/Ac/L1/Current", "/Ac/L2/Current", "/Ac/L3/Current",
		"/Ac/L1/Energy/Forward", "/Ac/L2/Energy/Forward", "/Ac/L3/Energy/Forward",
		"/Ac/L1/Energy/Reverse", "/Ac/L2/Energy/Reverse", "/Ac/L3/Energy/Reverse",
		"/Ac/Current", "/Ac/Voltage", "/Ac/Power", "/Ac/Energy/Forward", "/Ac/Energy/Reverse",
	}

	a.dbusConn.Export(a, "/", "com.victronenergy.BusItem")

	a.dbusConn.Export(introspect.Introspectable(intro), "/", "org.freedesktop.DBus.Introspectable")

	for _, p := range paths {
		log.Debug("Exporting dbus path: ", p)
		a.dbusConn.Export(objectpath(p), p, "com.victronenergy.BusItem")
	}

	// only after all paths are exported, request the name
	log.Infof("All paths exported. Requesting name %s on D-Bus...", a.config.DBusName)
	reply, err := a.dbusConn.RequestName(a.config.DBusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("failed to request DBus name: %w", err)
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("name %s already taken on dbus", a.config.DBusName)
	}

	log.Info("Successfully acquired D-Bus name.")
	return nil
}

func (a *App) emitItemsChanged(items map[string]map[string]dbus.Variant) {
	if len(items) > 0 {
		a.dbusConn.Emit("/", "com.victronenergy.BusItem.ItemsChanged", items)
	}
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

	// these used to be in the old demo, but have been removed. Not sure what they did, but they may be useful in the future
	//a.values[0]["/Position"] = dbus.MakeVariantWithSignature(0, dbus.SignatureOf(123))
	///a.values[1]["/Position"] = dbus.MakeVariant("0")
	//a.values[0]["/ProductId"] = dbus.MakeVariant(45058)
	//a.values[1]["/ProductId"] = dbus.MakeVariant("45058")

	a.values[0]["/ProductName"] = dbus.MakeVariant("Grid meter")
	a.values[1]["/ProductName"] = dbus.MakeVariant("Grid meter")

	a.values[0]["/Serial"] = dbus.MakeVariant("BP98305081235")
	a.values[1]["/Serial"] = dbus.MakeVariant("BP98305081235")

	// Initialize power values
	a.values[0]["/Ac/L1/Power"] = dbus.MakeVariant(1.0)
	a.values[1]["/Ac/L1/Power"] = dbus.MakeVariant("1 W")
	a.values[0]["/Ac/L2/Power"] = dbus.MakeVariant(1.0)
	a.values[1]["/Ac/L2/Power"] = dbus.MakeVariant("1 W")
	a.values[0]["/Ac/L3/Power"] = dbus.MakeVariant(1.0)
	a.values[1]["/Ac/L3/Power"] = dbus.MakeVariant("1 W")

	// Initialize voltage values
	a.values[0]["/Ac/L1/Voltage"] = dbus.MakeVariant(230)
	a.values[1]["/Ac/L1/Voltage"] = dbus.MakeVariant("230 V")
	a.values[0]["/Ac/L2/Voltage"] = dbus.MakeVariant(230)
	a.values[1]["/Ac/L2/Voltage"] = dbus.MakeVariant("230 V")
	a.values[0]["/Ac/L3/Voltage"] = dbus.MakeVariant(230)
	a.values[1]["/Ac/L3/Voltage"] = dbus.MakeVariant("230 V")

	// Initialize current values
	a.values[0]["/Ac/L1/Current"] = dbus.MakeVariant(1.0)
	a.values[1]["/Ac/L1/Current"] = dbus.MakeVariant("1 A")
	a.values[0]["/Ac/L2/Current"] = dbus.MakeVariant(1.0)
	a.values[1]["/Ac/L2/Current"] = dbus.MakeVariant("1 A")
	a.values[0]["/Ac/L3/Current"] = dbus.MakeVariant(1.0)
	a.values[1]["/Ac/L3/Current"] = dbus.MakeVariant("1 A")

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

	// Initialize total values
	a.values[0]["/Ac/Current"] = dbus.MakeVariant(1.0)
	a.values[1]["/Ac/Current"] = dbus.MakeVariant("1 A")
	a.values[0]["/Ac/Voltage"] = dbus.MakeVariant(230)
	a.values[1]["/Ac/Voltage"] = dbus.MakeVariant("230 V")
	a.values[0]["/Ac/Power"] = dbus.MakeVariant(1.0)
	a.values[1]["/Ac/Power"] = dbus.MakeVariant("1 W")
	a.values[0]["/Ac/Energy/Forward"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/Energy/Forward"] = dbus.MakeVariant("0 kWh")
	a.values[0]["/Ac/Energy/Reverse"] = dbus.MakeVariant(0.0)
	a.values[1]["/Ac/Energy/Reverse"] = dbus.MakeVariant("0 kWh")
}

func (a *App) GetItems() (map[string]map[string]dbus.Variant, *dbus.Error) {
	log.Debug("GetItems() called on root")
	a.mu.RLock()
	defer a.mu.RUnlock()

	items := make(map[string]map[string]dbus.Variant)

	// Iterate over all known paths
	for path, valueVariant := range a.values[0] {
		pathStr := string(path)
		textVariant, ok := a.values[1][path]
		if !ok {
			// This case should ideally not happen if InitializeValues is correct
			textVariant = dbus.MakeVariant("")
		}

		items[pathStr] = map[string]dbus.Variant{
			"Value": valueVariant,
			"Text":  textVariant,
		}
	}

	return items, nil
}
