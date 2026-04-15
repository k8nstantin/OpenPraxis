package peer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/grandcat/zeroconf"
)

const serviceType = "_agentmemory._tcp"

// Discovery handles mDNS service registration and browsing.
type Discovery struct {
	registry *Registry
	server   *zeroconf.Server
	nodeID   string
	port     int
	onFound  func(nodeID, ip string, port int) // callback when peer discovered
	onLost   func(nodeID string)               // callback when peer lost
}

// NewDiscovery creates a peer discovery service.
func NewDiscovery(registry *Registry, nodeID string, port int) *Discovery {
	return &Discovery{
		registry: registry,
		nodeID:   nodeID,
		port:     port,
	}
}

// SetCallbacks sets callbacks for peer events.
func (d *Discovery) SetCallbacks(onFound func(string, string, int), onLost func(string)) {
	d.onFound = onFound
	d.onLost = onLost
}

// Start registers this node and begins browsing for peers.
func (d *Discovery) Start(ctx context.Context) error {
	// Register this node
	hostname, _ := os.Hostname()
	server, err := zeroconf.Register(
		d.nodeID,       // instance name
		serviceType,    // service type
		"local.",       // domain
		d.port,         // port
		[]string{"id=" + d.nodeID}, // TXT records
		nil,            // interfaces (all)
	)
	if err != nil {
		return fmt.Errorf("register mDNS: %w", err)
	}
	d.server = server
	slog.Info("mDNS registered", "node_id", d.nodeID, "port", d.port, "hostname", hostname)

	// Browse for peers
	go d.browse(ctx)

	return nil
}

// Stop shuts down mDNS registration.
func (d *Discovery) Stop() {
	if d.server != nil {
		d.server.Shutdown()
	}
}

func (d *Discovery) browse(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		resolver, err := zeroconf.NewResolver(nil)
		if err != nil {
			slog.Error("mDNS resolver error", "error", err)
			time.Sleep(5 * time.Second)
			continue
		}

		entries := make(chan *zeroconf.ServiceEntry)
		go func() {
			for entry := range entries {
				d.handleEntry(entry)
			}
		}()

		browseCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := resolver.Browse(browseCtx, serviceType, "local.", entries); err != nil {
			slog.Error("mDNS browse error", "error", err)
		}
		cancel()

		// Re-browse periodically
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
		}
	}
}

func (d *Discovery) handleEntry(entry *zeroconf.ServiceEntry) {
	// Extract node ID from TXT records
	nodeID := ""
	for _, txt := range entry.Text {
		if len(txt) > 3 && txt[:3] == "id=" {
			nodeID = txt[3:]
		}
	}
	if nodeID == "" || nodeID == d.nodeID {
		return // skip self
	}

	// Get IP address
	var ip string
	if len(entry.AddrIPv4) > 0 {
		ip = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		ip = entry.AddrIPv6[0].String()
	}
	port := entry.Port

	// Register peer
	d.registry.Add(nodeID, ip, port)
	slog.Info("mDNS discovered peer", "node_id", nodeID, "ip", ip, "port", port)

	if d.onFound != nil {
		d.onFound(nodeID, ip, port)
	}
}

// ConnectStatic attempts to connect to statically configured peers.
func (d *Discovery) ConnectStatic(ctx context.Context, staticPeers []string) {
	for _, addr := range staticPeers {
		go func(a string) {
			// Parse host:port
			host, portStr := a, "8766"
			if idx := len(a) - 1; idx > 0 {
				for i := len(a) - 1; i >= 0; i-- {
					if a[i] == ':' {
						host = a[:i]
						portStr = a[i+1:]
						break
					}
				}
			}
			port, _ := strconv.Atoi(portStr)

			// Try to reach the peer
			d.registry.Add("static-"+host, host, port)
			slog.Info("static peer added", "host", host, "port", port)

			if d.onFound != nil {
				d.onFound("static-"+host, host, port)
			}
		}(addr)
	}
}
