package main

import (
	"net"
	"testing"
)

func TestResolveMDNSBindingWithSelectsAdvertisedInterface(t *testing.T) {
	ifaces := []net.Interface{
		{Name: "uplink0", Flags: net.FlagUp | net.FlagMulticast, Index: 1},
		{Name: "spaxel-ap", Flags: net.FlagUp | net.FlagMulticast, Index: 2},
	}

	addrs := func(iface net.Interface) ([]net.Addr, error) {
		ip := map[string]string{
			"uplink0":   "10.0.0.20/24",
			"spaxel-ap": "192.168.50.1/24",
		}[iface.Name]
		parsedIP, network, err := net.ParseCIDR(ip)
		if err != nil {
			return nil, err
		}
		network.IP = parsedIP
		return []net.Addr{network}, nil
	}

	iface, ip, err := resolveMDNSBindingWith(
		"192.168.50.1",
		net.ParseIP("192.168.50.1"),
		ifaces,
		nil,
		addrs,
	)
	if err != nil {
		t.Fatalf("resolveMDNSBindingWith returned error: %v", err)
	}
	if iface == nil || iface.Name != "spaxel-ap" {
		t.Fatalf("selected interface = %v, want spaxel-ap", iface)
	}
	if got := ip.String(); got != "192.168.50.1" {
		t.Fatalf("selected IP = %s, want 192.168.50.1", got)
	}
}

func TestResolveMDNSBindingWithResolvesLocalHostname(t *testing.T) {
	ifaces := []net.Interface{{Name: "spaxel-ap", Flags: net.FlagUp, Index: 7}}
	lookup := func(host string) ([]net.IP, error) {
		if host != "mothership.local" {
			t.Fatalf("lookup host = %s, want mothership.local", host)
		}
		return []net.IP{net.ParseIP("192.168.50.1")}, nil
	}
	addrs := func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{&net.IPAddr{IP: net.ParseIP("192.168.50.1")}}, nil
	}

	iface, ip, err := resolveMDNSBindingWith("mothership.local", nil, ifaces, lookup, addrs)
	if err != nil {
		t.Fatalf("resolveMDNSBindingWith returned error: %v", err)
	}
	if iface.Name != "spaxel-ap" || ip.String() != "192.168.50.1" {
		t.Fatalf("binding = %s/%s, want spaxel-ap/192.168.50.1", iface.Name, ip)
	}
}

func TestResolveMDNSBindingWithRejectsNonLocalAddress(t *testing.T) {
	ifaces := []net.Interface{{Name: "spaxel-ap", Flags: net.FlagUp, Index: 7}}
	addrs := func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{&net.IPAddr{IP: net.ParseIP("192.168.50.1")}}, nil
	}

	_, _, err := resolveMDNSBindingWith(
		"10.0.0.20",
		net.ParseIP("10.0.0.20"),
		ifaces,
		nil,
		addrs,
	)
	if err == nil {
		t.Fatal("resolveMDNSBindingWith succeeded for an address not present locally")
	}
}
