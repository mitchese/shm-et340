package main

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestMulticastListener_ContextCancellation(t *testing.T) {
	ml := &MulticastListener{
		Address: "239.12.255.254:9522",
		Handler: func(src *net.UDPAddr, n int, b []byte) {},
	}

	// Immediately cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ml.Listen(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Listen with cancelled ctx returned %v, want context.Canceled", err)
	}
}
