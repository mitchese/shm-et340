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
	"github.com/godbus/dbus/v5/introspect"
	log "github.com/sirupsen/logrus"
)

const introXML = `
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

const introXMLPath = `
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

// dbusPaths lists all D-Bus object paths to register.
var dbusPaths = []dbus.ObjectPath{
	// Static device info
	"/Connected", "/CustomName", "/DeviceInstance", "/DeviceType",
	"/ErrorCode", "/FirmwareVersion", "/Mgmt/Connection", "/Mgmt/ProcessName",
	"/Mgmt/ProcessVersion", "/ProductName", "/ProductId", "/RefreshTime", "/Role", "/Position", "/Serial",
	// Dynamic measurement paths
	"/Ac/L1/Power", "/Ac/L2/Power", "/Ac/L3/Power",
	"/Ac/L1/Voltage", "/Ac/L2/Voltage", "/Ac/L3/Voltage",
	"/Ac/L1/Current", "/Ac/L2/Current", "/Ac/L3/Current",
	"/Ac/L1/Energy/Forward", "/Ac/L2/Energy/Forward", "/Ac/L3/Energy/Forward",
	"/Ac/L1/Energy/Reverse", "/Ac/L2/Energy/Reverse", "/Ac/L3/Energy/Reverse",
	"/Ac/Current", "/Ac/Voltage", "/Ac/Power", "/Ac/Energy/Forward", "/Ac/Energy/Reverse",
}

// RegisterDBusPaths exports all object paths and requests the service name.
func (a *App) RegisterDBusPaths() error {
	if err := a.dbusConn.Export(a, "/", "com.victronenergy.BusItem"); err != nil {
		return fmt.Errorf("failed to export root: %w", err)
	}

	if err := a.dbusConn.Export(introspect.Introspectable(introXML), "/", "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("failed to export introspection: %w", err)
	}

	for _, p := range dbusPaths {
		log.Debug("Exporting D-Bus path: ", p)
		if err := a.dbusConn.Export(objectpath(p), p, "com.victronenergy.BusItem"); err != nil {
			return fmt.Errorf("failed to export path %s: %w", p, err)
		}
		if err := a.dbusConn.Export(introspect.Introspectable(introXMLPath), p, "org.freedesktop.DBus.Introspectable"); err != nil {
			return fmt.Errorf("failed to export introspection for path %s: %w", p, err)
		}
	}

	log.Infof("All paths exported. Requesting name %s on D-Bus...", a.config.DBusName)
	reply, err := a.dbusConn.RequestName(a.config.DBusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("failed to request D-Bus name: %w", err)
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("name %s already taken on D-Bus", a.config.DBusName)
	}

	log.Info("Successfully acquired D-Bus name.")
	return nil
}

// emitItemsChanged emits changes to D-Bus, logging any errors.
func (a *App) emitItemsChanged(items map[string]map[string]dbus.Variant) {
	if len(items) == 0 {
		return
	}
	if err := a.dbusConn.Emit("/", "com.victronenergy.BusItem.ItemsChanged", items); err != nil {
		log.Warnf("Failed to emit ItemsChanged: %v", err)
	}
}
