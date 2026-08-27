package main

import "testing"

func TestFixtureRequiresImmutablePreviousChart(t *testing.T) {
	valid := fixture{
		PreviousVersion:     "2.12.0",
		PreviousChart:       "oci://ghcr.io/pascalebeier/charts/hitkeep:2.12.0",
		PreviousChartDigest: "sha256:4fac3c9a02f7257f4290d8178272fe19b155068028333199cac923c36ab7db51",
	}
	if err := valid.chartValid(); err != nil {
		t.Fatalf("valid chart fixture rejected: %v", err)
	}

	for _, invalid := range []fixture{
		{},
		{PreviousChart: valid.PreviousChart},
		{PreviousChart: valid.PreviousChart, PreviousChartDigest: "sha256:not-a-digest"},
		{PreviousChart: "oci://ghcr.io/pascalebeier/charts/hitkeep:2.11.0", PreviousChartDigest: valid.PreviousChartDigest},
	} {
		if err := invalid.chartValid(); err == nil {
			t.Fatalf("invalid chart fixture accepted: %#v", invalid)
		}
	}
}
