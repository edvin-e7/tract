package main

import "testing"

// TestResolveAddrBareDefaultIsLoopback is the check that was missing while the
// bug was live. The README and the startup warning both claimed the token-less
// mode was "for localhost", and nothing ever asserted that the address agreed —
// so a wildcard default sat under a localhost promise for months and was found
// by measuring a running process, not by the suite.
func TestResolveAddrBareDefaultIsLoopback(t *testing.T) {
	t.Setenv("TRACT_ADDR", "")
	t.Setenv("PORT", "")
	got := resolveAddr()
	if !isLoopback(got) {
		t.Fatalf("bare default resolved to %q, which is reachable from the network; "+
			"a token-less instance started with no env must not be", got)
	}
}

// TestResolveAddrHonoursExplicitIntent pins the two paths that MUST stay
// network-open: a PaaS-injected PORT (the proxy connects from outside the
// container) and an explicit TRACT_ADDR. Hardening the bare default is only
// correct if asking for the old behaviour still works.
func TestResolveAddrHonoursExplicitIntent(t *testing.T) {
	t.Run("PORT injected by the host", func(t *testing.T) {
		t.Setenv("TRACT_ADDR", "")
		t.Setenv("PORT", "3000")
		if got := resolveAddr(); got != ":3000" {
			t.Fatalf("PORT=3000 -> %q, want \":3000\" (wildcard, as a PaaS requires)", got)
		}
	})
	t.Run("TRACT_ADDR wins over PORT", func(t *testing.T) {
		t.Setenv("TRACT_ADDR", "0.0.0.0:9999")
		t.Setenv("PORT", "3000")
		if got := resolveAddr(); got != "0.0.0.0:9999" {
			t.Fatalf("TRACT_ADDR -> %q, want \"0.0.0.0:9999\"", got)
		}
	})
}

// TestIsLoopback covers the conflation that caused the original defect: a bare
// ":8080" reads like "the local port" and IS the wildcard. Anything the function
// cannot positively identify as loopback must count as open — an unknown address
// is not a safe one.
func TestIsLoopback(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"localhost:8080", true},
		{"[::1]:8080", true},
		{":8080", false},        // wildcard — the bug
		{"0.0.0.0:8080", false}, // wildcard, spelled out
		{"192.168.1.21:8080", false},
		{"[::]:8080", false},
		{"/tmp/tract.sock", false}, // not host:port; unknown is not safe
		{"", false},
	} {
		if got := isLoopback(tc.addr); got != tc.want {
			t.Errorf("isLoopback(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}
