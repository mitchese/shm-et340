package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/dmichael/go-multicast/multicast"
	"github.com/godbus/dbus/introspect"
	"github.com/godbus/dbus/v5"
	log "github.com/sirupsen/logrus"
)

const (
	address = "239.12.255.254:9522"
)

var conn, err = dbus.SystemBus()

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

func (f objectpath) GetValue() (dbus.Variant, *dbus.Error) {
	log.Debug("GetValue() called for ", f)
	log.Debug("...returning ", victronValues[0][f])
	return victronValues[0][f], nil
}
func (f objectpath) GetText() (string, *dbus.Error) {
	log.Debug("GetText() called for ", f)
	log.Debug("...returning ", victronValues[1][f])
	// Why does this end up ""SOMEVAL"" ... trim it I guess
	return strings.Trim(victronValues[1][f].String(), "\""), nil
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

func main() {
	// Need to implement following paths:
	// https://github.com/victronenergy/venus/wiki/dbus#grid-meter
	// also in system.py
	victronValues[0]["/Connected"] = dbus.MakeVariant(1)
	victronValues[1]["/Connected"] = dbus.MakeVariant("1")

	victronValues[0]["/CustomName"] = dbus.MakeVariant("Grid meter")
	victronValues[1]["/CustomName"] = dbus.MakeVariant("Grid meter")

	victronValues[0]["/DeviceInstance"] = dbus.MakeVariant(30)
	victronValues[1]["/DeviceInstance"] = dbus.MakeVariant("30")

	// also in system.py
	victronValues[0]["/DeviceType"] = dbus.MakeVariant(71)
	victronValues[1]["/DeviceType"] = dbus.MakeVariant("71")

	victronValues[0]["/ErrorCode"] = dbus.MakeVariantWithSignature(0, dbus.SignatureOf(123))
	victronValues[1]["/ErrorCode"] = dbus.MakeVariant("0")

	victronValues[0]["/FirmwareVersion"] = dbus.MakeVariant(2)
	victronValues[1]["/FirmwareVersion"] = dbus.MakeVariant("2")

	// also in system.py
	victronValues[0]["/Mgmt/Connection"] = dbus.MakeVariant("/dev/ttyUSB0")
	victronValues[1]["/Mgmt/Connection"] = dbus.MakeVariant("/dev/ttyUSB0")

	victronValues[0]["/Mgmt/ProcessName"] = dbus.MakeVariant("/opt/color-control/dbus-cgwacs/dbus-cgwacs")
	victronValues[1]["/Mgmt/ProcessName"] = dbus.MakeVariant("/opt/color-control/dbus-cgwacs/dbus-cgwacs")

	victronValues[0]["/Mgmt/ProcessVersion"] = dbus.MakeVariant("1.8.0")
	victronValues[1]["/Mgmt/ProcessVersion"] = dbus.MakeVariant("1.8.0")

	victronValues[0]["/Position"] = dbus.MakeVariantWithSignature(0, dbus.SignatureOf(123))
	victronValues[1]["/Position"] = dbus.MakeVariant("0")

	// also in system.py
	victronValues[0]["/ProductId"] = dbus.MakeVariant(45058)
	victronValues[1]["/ProductId"] = dbus.MakeVariant("45058")

	// also in system.py
	victronValues[0]["/ProductName"] = dbus.MakeVariant("Grid meter")
	victronValues[1]["/ProductName"] = dbus.MakeVariant("Grid meter")

	victronValues[0]["/Serial"] = dbus.MakeVariant("BP98305081235")
	victronValues[1]["/Serial"] = dbus.MakeVariant("BP98305081235")

	// Provide some initial values... note that the values must be a valid formt otherwise dbus_systemcalc.py exits like this:
	//@400000005ecc11bf3782b374   File "/opt/victronenergy/dbus-systemcalc-py/dbus_systemcalc.py", line 386, in _handletimertick
	//@400000005ecc11bf37aa251c     self._updatevalues()
	//@400000005ecc11bf380e74cc   File "/opt/victronenergy/dbus-systemcalc-py/dbus_systemcalc.py", line 678, in _updatevalues
	//@400000005ecc11bf383ab4ec     c = _safeadd(c, p, pvpower)
	//@400000005ecc11bf386c9674   File "/opt/victronenergy/dbus-systemcalc-py/sc_utils.py", line 13, in safeadd
	//@400000005ecc11bf387b28ec     return sum(values) if values else None
	//@400000005ecc11bf38b2bb7c TypeError: unsupported operand type(s) for +: 'int' and 'unicode'
	//
	victronValues[0]["/Ac/L1/Power"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L1/Power"] = dbus.MakeVariant("0 W")
	victronValues[0]["/Ac/L2/Power"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L2/Power"] = dbus.MakeVariant("0 W")
	victronValues[0]["/Ac/L3/Power"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L3/Power"] = dbus.MakeVariant("0 W")

	victronValues[0]["/Ac/L1/Voltage"] = dbus.MakeVariant(230)
	victronValues[1]["/Ac/L1/Voltage"] = dbus.MakeVariant("230 V")
	victronValues[0]["/Ac/L2/Voltage"] = dbus.MakeVariant(230)
	victronValues[1]["/Ac/L2/Voltage"] = dbus.MakeVariant("230 V")
	victronValues[0]["/Ac/L3/Voltage"] = dbus.MakeVariant(230)
	victronValues[1]["/Ac/L3/Voltage"] = dbus.MakeVariant("230 V")

	victronValues[0]["/Ac/L1/Current"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L1/Current"] = dbus.MakeVariant("0 A")
	victronValues[0]["/Ac/L2/Current"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L2/Current"] = dbus.MakeVariant("0 A")
	victronValues[0]["/Ac/L3/Current"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L3/Current"] = dbus.MakeVariant("0 A")

	victronValues[0]["/Ac/L1/Energy/Forward"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L1/Energy/Forward"] = dbus.MakeVariant("0 kWh")
	victronValues[0]["/Ac/L2/Energy/Forward"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L2/Energy/Forward"] = dbus.MakeVariant("0 kWh")
	victronValues[0]["/Ac/L3/Energy/Forward"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L3/Energy/Forward"] = dbus.MakeVariant("0 kWh")

	victronValues[0]["/Ac/L1/Energy/Reverse"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L1/Energy/Reverse"] = dbus.MakeVariant("0 kWh")
	victronValues[0]["/Ac/L2/Energy/Reverse"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L2/Energy/Reverse"] = dbus.MakeVariant("0 kWh")
	victronValues[0]["/Ac/L3/Energy/Reverse"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L3/Energy/Reverse"] = dbus.MakeVariant("0 kWh")

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

	defer conn.Close()

	// Some of the victron stuff requires it be called grid.cgwacs... using the only known valid value (from the simulator)
	// This can _probably_ be changed as long as it matches com.victronenergy.grid.cgwacs_*
	reply, err := conn.RequestName("com.victronenergy.grid.cgwacs_ttyUSB0_di30_mb1",
		dbus.NameFlagDoNotQueue)
	if err != nil {
		log.Panic("Something went horribly wrong in the dbus connection")
		panic(err)
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		log.Panic("name cgwacs_ttyUSB0_di30_mb1 already taken on dbus.")
		os.Exit(1)
	}

	for i, s := range basicPaths {
		log.Debug("Registering dbus basic path #", i, ": ", s)
		conn.Export(objectpath(s), s, "com.victronenergy.BusItem")
		conn.Export(introspect.Introspectable(intro), s, "org.freedesktop.DBus.Introspectable")
	}

	for i, s := range updatingPaths {
		log.Debug("Registering dbus update path #", i, ": ", s)
		conn.Export(objectpath(s), s, "com.victronenergy.BusItem")
		conn.Export(introspect.Introspectable(intro), s, "org.freedesktop.DBus.Introspectable")
	}

	log.Info("Successfully connected to dbus and registered as a meter... Commencing reading of the SMA meter")

	multicast.Listen(address, msgHandler)
	// This is a forever loop^^
	panic("Error: We terminated.... how did we ever get here?")
}

func msgHandler(src *net.UDPAddr, n int, b []byte) {
	// This function will be called with every datagram sent by the SMA meter

	// 0-28: SMA/SUSyID/SN/Uptime
	log.Debug("----------------------")
	log.Debug("Received datagram from meter")
	log.Debug("Uid: ", binary.BigEndian.Uint32(b[4:8]))
	log.Debug("Serial: ", binary.BigEndian.Uint32(b[20:24]))

	//              ...buy....                                 ...sell...  both in 0.1W, converted to W
	powertot := ((float32(binary.BigEndian.Uint32(b[32:36])) - float32(binary.BigEndian.Uint32(b[52:56]))) / 10.0)

	// in watt seconds, convert to kWh
	bezugtot := float64(binary.BigEndian.Uint64(b[40:48])) / 3600.0 / 1000.0
	einsptot := float64(binary.BigEndian.Uint64(b[60:68])) / 3600.0 / 1000.0

	log.Debug("Total W: ", powertot)
	log.Debug("Total Buy kWh: ", bezugtot)
	log.Debug("Total Sell kWh: ", einsptot)

	log.Info(fmt.Sprintf("Meter update received: %.2f kWh bought and %.2f kWh sold, %.1f W currently flowing", bezugtot, einsptot, powertot))
	updateVariant(float64(powertot), "W", "/Ac/Power")
	updateVariant(float64(einsptot), "kWh", "/Ac/Energy/Reverse")
	updateVariant(float64(bezugtot), "kWh", "/Ac/Energy/Forward")

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

	// L1
	updateVariant(float64(L1.power), "W", "/Ac/L1/Power")
	updateVariant(float64(L1.voltage), "V", "/Ac/L1/Voltage")
	updateVariant(float64(L1.a), "A", "/Ac/L1/Current")
	updateVariant(L1.forward, "kWh", "/Ac/L1/Energy/Forward")
	updateVariant(L1.reverse, "kWh", "/Ac/L1/Energy/Reverse")

	// L2
	updateVariant(float64(L2.power), "W", "/Ac/L2/Power")
	updateVariant(float64(L2.voltage), "V", "/Ac/L2/Voltage")
	updateVariant(float64(L2.a), "A", "/Ac/L2/Current")
	updateVariant(L2.forward, "kWh", "/Ac/L2/Energy/Forward")
	updateVariant(L2.reverse, "kWh", "/Ac/L2/Energy/Reverse")

	// L3
	updateVariant(float64(L3.power), "W", "/Ac/L3/Power")
	updateVariant(float64(L3.voltage), "V", "/Ac/L3/Voltage")
	updateVariant(float64(L3.a), "A", "/Ac/L3/Current")
	updateVariant(L3.forward, "kWh", "/Ac/L3/Energy/Forward")
	updateVariant(L3.reverse, "kWh", "/Ac/L3/Energy/Reverse")

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

func updateVariant(value float64, unit string, path string) {
	emit := make(map[string]dbus.Variant)
	emit["Text"] = dbus.MakeVariant(fmt.Sprintf("%.2f", value) + unit)
	emit["Value"] = dbus.MakeVariant(float64(value))
	victronValues[0][objectpath(path)] = emit["Value"]
	victronValues[1][objectpath(path)] = emit["Text"]
	conn.Emit(dbus.ObjectPath(path), "com.victronenergy.BusItem.PropertiesChanged", emit)
}
