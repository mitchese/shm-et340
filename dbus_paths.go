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
	"strings"

	"github.com/godbus/dbus/v5"
	log "github.com/sirupsen/logrus"
)

// objectpath is a string type whose methods are exported as D-Bus methods.
type objectpath string

// GetValue returns the current value for this path.
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

// GetText returns the text representation for this path.
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

// SetValue sets the value for this path.
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

// GetItems returns all path values (called on the App at root "/").
func (a *App) GetItems() (map[string]map[string]dbus.Variant, *dbus.Error) {
	log.Debug("GetItems() called on root")
	a.mu.RLock()
	defer a.mu.RUnlock()

	items := make(map[string]map[string]dbus.Variant)

	for path, valueVariant := range a.values[0] {
		pathStr := string(path)
		textVariant, ok := a.values[1][path]
		if !ok {
			textVariant = dbus.MakeVariant("")
		}

		items[pathStr] = map[string]dbus.Variant{
			"Value": valueVariant,
			"Text":  textVariant,
		}
	}

	return items, nil
}
