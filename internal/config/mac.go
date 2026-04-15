package config

import (
	"net"
	"strings"
)

// preferredInterfaces lists NIC names to prefer, in order.
var preferredInterfaces = []string{"en0", "eth0", "wlan0", "en1", "eth1"}

// getPrimaryMAC returns the MAC address of the primary network interface.
// It prefers well-known physical interfaces (en0, eth0, wlan0) and skips
// loopback, virtual, and inactive interfaces. Returns "unknown" if no
// valid NIC is found.
func getPrimaryMAC() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "unknown"
	}

	// First pass: try preferred interfaces in order
	for _, name := range preferredInterfaces {
		for _, iface := range ifaces {
			if iface.Name == name {
				if mac := validMAC(iface); mac != "" {
					return mac
				}
			}
		}
	}

	// Second pass: first valid non-virtual interface
	for _, iface := range ifaces {
		if isVirtual(iface.Name) {
			continue
		}
		if mac := validMAC(iface); mac != "" {
			return mac
		}
	}

	return "unknown"
}

// validMAC returns the formatted MAC address if the interface is up,
// not loopback, and has a hardware address. Returns "" otherwise.
func validMAC(iface net.Interface) string {
	if iface.Flags&net.FlagLoopback != 0 {
		return ""
	}
	if iface.Flags&net.FlagUp == 0 {
		return ""
	}
	mac := iface.HardwareAddr
	if len(mac) == 0 {
		return ""
	}
	return mac.String()
}

// isVirtual returns true for interface names that are typically virtual.
func isVirtual(name string) bool {
	virtual := []string{"lo", "veth", "docker", "br-", "bridge", "virbr", "vmnet", "vbox", "utun", "awdl", "llw"}
	lower := strings.ToLower(name)
	for _, prefix := range virtual {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}
