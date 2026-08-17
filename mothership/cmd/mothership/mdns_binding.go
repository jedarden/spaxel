package main

import (
	"fmt"
	"net"
	"net/url"
)

// resolveMDNSBinding finds the local interface and IPv4 address represented by
// the URL that is handed to nodes for node-reachable HTTP/OTA traffic. The
// HashiCorp mDNS server binds to one interface when Config.Iface is nil, and
// its service records otherwise come from the host name lookup. On a
// multi-homed host those defaults can select the uplink instead of the LAN
// interface where the nodes live.
func resolveMDNSBinding(advertisedBaseURL string) (*net.Interface, net.IP, error) {
	parsed, err := url.Parse(advertisedBaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse advertised base URL: %w", err)
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, nil, fmt.Errorf("advertised base URL has no host")
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, fmt.Errorf("list network interfaces: %w", err)
	}
	lookup := func(name string) ([]net.IP, error) {
		return net.LookupIP(name)
	}
	return resolveMDNSBindingWith(host, net.ParseIP(host), ifaces, lookup,
		func(iface net.Interface) ([]net.Addr, error) { return iface.Addrs() })
}

// resolveMDNSBindingWith contains the selection logic separately from system
// lookups so the important multi-interface behavior can be tested without
// depending on the machine running the test.
func resolveMDNSBindingWith(
	host string,
	directIP net.IP,
	ifaces []net.Interface,
	lookup func(string) ([]net.IP, error),
	addrs func(net.Interface) ([]net.Addr, error),
) (*net.Interface, net.IP, error) {
	candidates := make([]net.IP, 0, 1)
	if directIP != nil {
		candidates = append(candidates, directIP)
	} else {
		ips, err := lookup(host)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve %s: %w", host, err)
		}
		candidates = append(candidates, ips...)
	}

	for _, candidate := range candidates {
		candidate4 := candidate.To4()
		if candidate4 == nil {
			continue
		}
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			ifaceAddrs, err := addrs(iface)
			if err != nil {
				continue
			}
			for _, addr := range ifaceAddrs {
				if ip := ipFromNetAddr(addr); ip != nil && ip.Equal(candidate4) {
					selected := iface
					return &selected, candidate4, nil
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("%s does not resolve to an IPv4 address on a local interface", host)
}

func ipFromNetAddr(addr net.Addr) net.IP {
	switch typed := addr.(type) {
	case *net.IPNet:
		return typed.IP.To4()
	case *net.IPAddr:
		return typed.IP.To4()
	default:
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			return nil
		}
		return ip.To4()
	}
}
