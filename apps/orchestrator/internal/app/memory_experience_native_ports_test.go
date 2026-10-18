package app

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExperienceNativeAddressesRemainReservedAcrossChildHandoff(t *testing.T) {
	addresses := newExperienceNativeAddresses(t)
	all := []string{addresses.Extension, addresses.HTTP, addresses.Broker, addresses.BrokerHTTP}
	assertReserved := func(ports []string) {
		t.Helper()
		for _, address := range ports {
			listener, err := net.Listen("tcp4", address)
			if err == nil {
				listener.Close()
				t.Fatalf("frozen endpoint was available to another fixture: %s", address)
			}
		}
	}
	assertReserved(all)
	if addresses.reserved == nil || len(addresses.reserved.listeners) != len(all) {
		t.Fatal("all four distinct endpoints must remain reserved after freezing")
	}
	for cycle := 0; cycle < 2; cycle++ {
		assertReserved(all)
		// These are the allocations that previously could steal a frozen endpoint.
		authority, recipient := httptest.NewServer(http.NotFoundHandler()), httptest.NewServer(http.NotFoundHandler())
		assertReserved(all)
		for _, ports := range [][]string{{addresses.Extension}, {addresses.HTTP}, {addresses.Broker, addresses.BrokerHTTP}} {
			var children []net.Listener
			if err := addresses.reserved.handoff(func() error {
				for _, address := range ports {
					listener, err := net.Listen("tcp4", address)
					if err != nil {
						return err
					}
					children = append(children, listener)
				}
				return nil
			}, ports...); err != nil {
				t.Fatal("child could not bind its exact frozen endpoints", err)
			}
			assertReserved(all)
			for _, listener := range children {
				listener.Close()
			}
			if err := addresses.reserved.reclaim(ports...); err != nil {
				t.Fatal(err)
			}
			assertReserved(all)
		}
		authority.Close()
		recipient.Close()
		assertReserved(all)
	}
}
