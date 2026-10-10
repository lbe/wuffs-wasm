package wuffs

import (
	"os"
	"testing"
)

const benchFixturesHint = "run make bench-fixtures from repo root (network required)"

const (
	benchMHibiscusPNG      = "testdata/bench/m/hibiscus.regular.png"
	benchMHibiscusJPEG     = "testdata/bench/m/hibiscus.regular.jpeg"
	benchMHibiscusWEBP     = "testdata/bench/m/hibiscus.regular.lossless.webp"
	benchMHibiscusGIF      = "testdata/bench/m/hibiscus.regular.gif"
	benchLHarvestersPNG    = "testdata/bench/l/harvesters.png"
	benchLHarvestersJPEG   = "testdata/bench/l/harvesters.jpeg"
	benchLHarvestersWEBPLL = "testdata/bench/l/harvesters.lossless.webp"
	benchLHarvestersWEBPLY = "testdata/bench/l/harvesters.lossy.webp"
	benchLHarvestersGIF    = "testdata/bench/l/harvesters.gif"
)

var benchFixturePaths = []string{
	benchMHibiscusPNG,
	benchMHibiscusJPEG,
	benchMHibiscusWEBP,
	benchMHibiscusGIF,
	benchLHarvestersPNG,
	benchLHarvestersJPEG,
	benchLHarvestersWEBPLL,
	benchLHarvestersWEBPLY,
	benchLHarvestersGIF,
}

func benchFixturesOK() bool {
	for _, p := range benchFixturePaths {
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return true
}

func requireBenchFixtures(tb testing.TB) {
	if !benchFixturesOK() {
		tb.Fatal(benchFixturesHint)
	}
}

func skipBenchFixturesIfMissing(tb testing.TB) {
	if !benchFixturesOK() {
		tb.Skip(benchFixturesHint)
	}
}
