package transport

import "testing"

// One client's pass over every vault it syncs fetches the device log and membership of each, so
// the burst must carry such a pass; the sustained rate is what bounds an address.
func TestTheBurstCarriesAPassOverManyVaults(t *testing.T) {
	cfg := withDefaults(Config{})
	if cfg.RatePerSecond != 20 || cfg.Burst != 200 {
		t.Fatalf("rate %v, burst %d; want 20 a second and 200 at once", cfg.RatePerSecond, cfg.Burst)
	}
	if cfg := withDefaults(Config{RatePerSecond: 5, Burst: 7}); cfg.RatePerSecond != 5 || cfg.Burst != 7 {
		t.Fatalf("explicit limits were replaced: %+v", cfg)
	}
	if cfg := withDefaults(Config{Burst: 9}); cfg.RatePerSecond != 20 || cfg.Burst != 9 {
		t.Fatalf("a burst alone keeps the default rate: %+v", cfg)
	}
}
