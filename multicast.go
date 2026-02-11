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
	"net"
	"time"

	log "github.com/sirupsen/logrus"
)

// MulticastHandler is the callback for received packets.
type MulticastHandler func(src *net.UDPAddr, n int, b []byte)

// MulticastListener joins a multicast group and delivers packets to a handler.
type MulticastListener struct {
	Address   string           // e.g. "239.12.255.254:9522"
	Interface string           // optional NIC name (e.g. "eth0")
	Handler   MulticastHandler
}

// Listen joins the multicast group and reads packets until the context is cancelled.
// On network errors it reconnects with exponential backoff (1s → 30s cap).
func (ml *MulticastListener) Listen(ctx context.Context) error {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		err := ml.listenOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			log.Warnf("Multicast listener error: %v — reconnecting in %v", err, backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
	}
}

// listenOnce opens a UDP socket, joins the multicast group, and reads packets.
// Returns an error on socket/read failure; caller handles reconnection.
func (ml *MulticastListener) listenOnce(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp4", ml.Address)
	if err != nil {
		return err
	}

	var ifi *net.Interface
	if ml.Interface != "" {
		ifi, err = net.InterfaceByName(ml.Interface)
		if err != nil {
			return err
		}
	}

	conn, err := net.ListenMulticastUDP("udp4", ifi, addr)
	if err != nil {
		return err
	}

	// Close the connection when the context is cancelled
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	if err := conn.SetReadBuffer(2048); err != nil {
		log.Warnf("Failed to set read buffer: %v", err)
	}

	buf := make([]byte, 2048)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			// If context was cancelled, this is expected
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		// Copy packet data to prevent races (buf is reused)
		pkt := make([]byte, n)
		copy(pkt, buf[:n])

		ml.Handler(src, n, pkt)
	}
}
