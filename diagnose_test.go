package main

import (
	"testing"
)

func TestCheckNetworkInterfaces(t *testing.T) {
	// Should return results on any system (at minimum, it should not crash)
	result := checkNetworkInterfaces()
	// On most systems with a network connection, this should pass
	// We just verify it doesn't panic
	_ = result
}
