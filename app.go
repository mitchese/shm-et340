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
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	log "github.com/sirupsen/logrus"
)

// App represents the main application.
type App struct {
	config       Config
	dbusConn     *dbus.Conn
	values       map[int]map[objectpath]dbus.Variant
	mu           sync.RWMutex
	lastEmit     time.Time
	lastReceived time.Time
	serialSet    bool
	stale        bool
}

var globalApp *App

// GetApp returns the global application instance (needed by D-Bus method handlers).
func GetApp() *App {
	return globalApp
}

// NewApp creates a new application instance.
func NewApp(cfg Config) (*App, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system bus: %w", err)
	}

	app := &App{
		config:   cfg,
		dbusConn: conn,
		values:   make(map[int]map[objectpath]dbus.Variant),
	}

	app.values[0] = make(map[objectpath]dbus.Variant) // VALUE variant
	app.values[1] = make(map[objectpath]dbus.Variant) // STRING variant

	return app, nil
}

// Run starts the application and blocks until the context is cancelled.
func (a *App) Run(ctx context.Context) error {
	a.InitializeValues()

	if err := a.RegisterDBusPaths(); err != nil {
		return fmt.Errorf("failed to register D-Bus paths: %w", err)
	}

	log.Info("Successfully connected to D-Bus and registered as a meter. Commencing reading of the SMA meter")

	// Start stale data watchdog
	go a.staleWatchdog(ctx)

	// Start multicast listener (blocks until ctx cancelled or fatal error)
	listener := &MulticastListener{
		Address:   a.config.MulticastAddress,
		Interface: a.config.Interface,
		Handler:   a.HandleMessage,
	}

	return listener.Listen(ctx)
}

// Shutdown closes the D-Bus connection.
func (a *App) Shutdown() {
	if a.dbusConn != nil {
		a.dbusConn.Close()
	}
}

// staleWatchdog periodically checks whether data has gone stale.
func (a *App) staleWatchdog(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.mu.RLock()
			lastRecv := a.lastReceived
			wasStale := a.stale
			a.mu.RUnlock()

			// No data received yet — nothing to mark stale
			if lastRecv.IsZero() {
				continue
			}

			isStale := time.Since(lastRecv) > a.config.StaleTimeout

			if isStale && !wasStale {
				log.Warn("No meter data received for ", a.config.StaleTimeout, " — marking stale")
				a.markStale()
			} else if !isStale && wasStale {
				log.Info("Meter data resumed — marking connected")
				a.markConnected()
			}
		}
	}
}

// markStale zeros out power/current and sets /Connected to 0.
func (a *App) markStale() {
	a.mu.Lock()
	a.stale = true

	a.values[0]["/Connected"] = dbus.MakeVariant(0)
	a.values[1]["/Connected"] = dbus.MakeVariant("0")

	// Zero out power and current values
	zeroPaths := []struct {
		path string
		unit string
	}{
		{"/Ac/Power", "W"}, {"/Ac/Current", "A"},
		{"/Ac/L1/Power", "W"}, {"/Ac/L1/Current", "A"},
		{"/Ac/L2/Power", "W"}, {"/Ac/L2/Current", "A"},
		{"/Ac/L3/Power", "W"}, {"/Ac/L3/Current", "A"},
	}

	changed := make(map[string]map[string]dbus.Variant)
	for _, zp := range zeroPaths {
		a.values[0][objectpath(zp.path)] = dbus.MakeVariant(0.0)
		a.values[1][objectpath(zp.path)] = dbus.MakeVariant("0.00 " + zp.unit)
		changed[zp.path] = map[string]dbus.Variant{
			"Value": dbus.MakeVariant(0.0),
			"Text":  dbus.MakeVariant("0.00 " + zp.unit),
		}
	}
	changed["/Connected"] = map[string]dbus.Variant{
		"Value": dbus.MakeVariant(0),
		"Text":  dbus.MakeVariant("0"),
	}
	a.mu.Unlock()

	a.emitItemsChanged(changed)
}

// markConnected sets /Connected back to 1.
func (a *App) markConnected() {
	a.mu.Lock()
	a.stale = false
	a.values[0]["/Connected"] = dbus.MakeVariant(1)
	a.values[1]["/Connected"] = dbus.MakeVariant("1")
	a.mu.Unlock()

	a.emitItemsChanged(map[string]map[string]dbus.Variant{
		"/Connected": {
			"Value": dbus.MakeVariant(1),
			"Text":  dbus.MakeVariant("1"),
		},
	})
}
