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
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
)

func main() {
	cfg := ParseConfig()

	if cfg.ShowVersion {
		fmt.Println("shm-et340 version", Version)
		os.Exit(0)
	}

	ll, err := log.ParseLevel(cfg.LogLevel)
	if err != nil {
		ll = log.InfoLevel
	}
	log.SetLevel(ll)

	if cfg.IsEnergyMeter {
		log.Info("SMA Energy Meter 1.0 mode (NOT SHM 2.0)")
	}

	if cfg.DiagnoseMode {
		os.Exit(RunDiagnostics(cfg))
	}

	app, err := NewApp(cfg)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}
	globalApp = app

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("Received shutdown signal, cleaning up...")
		cancel()
	}()

	if err := app.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("Application error: %v", err)
	}

	app.Shutdown()
}
