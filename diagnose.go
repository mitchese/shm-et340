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
	"time"

	"github.com/godbus/dbus/v5"
)

// RunDiagnostics runs all diagnostic checks and returns an exit code.
func RunDiagnostics(cfg Config) int {
	fmt.Println("=== shm-et340 Diagnostic Report ===")
	fmt.Println()

	allPassed := true

	// Check 1: Network interfaces
	if !checkNetworkInterfaces() {
		allPassed = false
	}

	// Check 2: Multicast reception
	if !checkMulticastReception(cfg) {
		allPassed = false
	}

	// Check 3: D-Bus connection
	if !checkDBusConnection() {
		allPassed = false
	}

	// Check 4: D-Bus name available
	if !checkDBusNameAvailable(cfg.DBusName) {
		allPassed = false
	}

	fmt.Println()
	if allPassed {
		fmt.Println("All checks passed.")
		return 0
	}
	fmt.Println("Some checks failed. See details above.")
	return 1
}

func checkNetworkInterfaces() bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Printf("[FAIL] List network interfaces\n")
		fmt.Printf("       Error: %v\n", err)
		return false
	}

	found := false
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, _ := iface.Addrs()
		addrStrs := make([]string, 0, len(addrs))
		for _, a := range addrs {
			addrStrs = append(addrStrs, a.String())
		}

		supportsMulticast := iface.Flags&net.FlagMulticast != 0
		status := "PASS"
		if !supportsMulticast {
			status = "WARN"
		}

		fmt.Printf("[%s] Interface %s supports multicast\n", status, iface.Name)
		fmt.Printf("       addresses: %v, flags: %v\n", addrStrs, iface.Flags)

		if supportsMulticast {
			found = true
		}
	}

	if !found {
		fmt.Printf("[FAIL] No UP non-loopback interface supports multicast\n")
		return false
	}
	return true
}

func checkMulticastReception(cfg Config) bool {
	fmt.Printf("[....] Receive multicast packet within 10 seconds\n")

	addr, err := net.ResolveUDPAddr("udp4", cfg.MulticastAddress)
	if err != nil {
		fmt.Printf("\r[FAIL] Receive multicast packet within 10 seconds\n")
		fmt.Printf("       Could not resolve address %s: %v\n", cfg.MulticastAddress, err)
		return false
	}

	var ifi *net.Interface
	if cfg.Interface != "" {
		ifi, err = net.InterfaceByName(cfg.Interface)
		if err != nil {
			fmt.Printf("\r[FAIL] Receive multicast packet within 10 seconds\n")
			fmt.Printf("       Interface %s not found: %v\n", cfg.Interface, err)
			return false
		}
	}

	conn, err := net.ListenMulticastUDP("udp4", ifi, addr)
	if err != nil {
		fmt.Printf("\r[FAIL] Receive multicast packet within 10 seconds\n")
		fmt.Printf("       Could not join multicast group: %v\n", err)
		return false
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	buf := make([]byte, 2048)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		fmt.Printf("\r[FAIL] Receive multicast packet within 10 seconds\n")
		fmt.Printf("       No packet received. This usually means:\n")
		fmt.Printf("       1. IGMP snooping on your switch is blocking multicast traffic\n")
		fmt.Printf("       2. The SMA meter is not on the same network/VLAN\n")
		fmt.Printf("       3. The SMA meter is powered off or misconfigured\n")
		fmt.Printf("       Try: Disable IGMP snooping on your managed switch, or\n")
		fmt.Printf("       enable the IGMP querier on your router.\n")
		return false
	}

	fmt.Printf("\r[PASS] Receive multicast packet within 10 seconds\n")
	fmt.Printf("       Packet size: %d bytes\n", n)
	if n >= 24 {
		serial := binary.BigEndian.Uint32(buf[20:24])
		fmt.Printf("       SMA serial: %d\n", serial)
	}
	return true
}

func checkDBusConnection() bool {
	conn, err := dbus.SystemBus()
	if err != nil {
		fmt.Printf("[FAIL] Connect to system D-Bus\n")
		fmt.Printf("       Error: %v\n", err)
		fmt.Printf("       Hint: Is this running on Venus OS? Is dbus-daemon running?\n")
		return false
	}
	conn.Close()
	fmt.Printf("[PASS] Connect to system D-Bus\n")
	return true
}

func checkDBusNameAvailable(name string) bool {
	conn, err := dbus.SystemBus()
	if err != nil {
		fmt.Printf("[FAIL] D-Bus name available (%s)\n", name)
		fmt.Printf("       Could not connect to system bus: %v\n", err)
		return false
	}
	defer conn.Close()

	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil {
		fmt.Printf("[FAIL] D-Bus name available (%s)\n", name)
		fmt.Printf("       Error requesting name: %v\n", err)
		return false
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		fmt.Printf("[FAIL] D-Bus name available (%s)\n", name)
		fmt.Printf("       Name already taken — is another instance of shm-et340 running?\n")
		return false
	}

	// Release the name so the actual app can claim it
	conn.ReleaseName(name)
	fmt.Printf("[PASS] D-Bus name available\n")
	return true
}
