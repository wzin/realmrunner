package minecraft

import (
	"os"
	"testing"
)

// Live tests hit the real upstream APIs. They are opt-in so normal test runs
// stay offline, but they are the only thing that catches an upstream API being
// sunset (as happened with PaperMC's v2 API).
func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv("REALMRUNNER_LIVE_TESTS") != "1" {
		t.Skip("set REALMRUNNER_LIVE_TESTS=1 to run tests against upstream APIs")
	}
}

func TestLiveProvidersListVersions(t *testing.T) {
	requireLive(t)

	for _, p := range []Provider{
		&VanillaProvider{fetcher: NewVersionFetcher()},
		&PaperProvider{},
		&PurpurProvider{},
	} {
		t.Run(p.Flavor(), func(t *testing.T) {
			versions, err := p.GetVersions(false)
			if err != nil {
				t.Fatalf("GetVersions: %v", err)
			}
			if len(versions) == 0 {
				t.Fatal("no versions returned")
			}

			var has26 bool
			for _, v := range versions {
				if len(v.ID) >= 2 && v.ID[:2] == "26" {
					has26 = true
					break
				}
			}
			if !has26 {
				t.Errorf("no 26.x version offered by %s", p.Flavor())
			}
		})
	}
}

func TestLiveVanillaJavaRequirement(t *testing.T) {
	requireLive(t)

	vf := NewVersionFetcher()
	major, err := vf.GetJavaMajor("26.3")
	if err != nil {
		t.Fatalf("GetJavaMajor: %v", err)
	}
	if major < 25 {
		t.Errorf("Mojang reports Java %d for 26.3, expected at least 25", major)
	}
}

func TestLivePaperHasBuildsFor26(t *testing.T) {
	requireLive(t)

	p := &PaperProvider{}
	build, err := p.latestBuild("26.3")
	if err != nil {
		t.Fatalf("latestBuild: %v", err)
	}
	if _, ok := build.Downloads["server:default"]; !ok {
		t.Errorf("Paper build %d has no server:default download", build.ID)
	}
}
