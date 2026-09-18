package minecraft

import "testing"

func TestIsPrerelease(t *testing.T) {
	prereleases := []string{"26.3-rc-1", "26.3-pre-2", "26.3-snapshot-10", "1.21.9-pre2", "24w14a", "1.19-experimental"}
	for _, v := range prereleases {
		if !IsPrerelease(v) {
			t.Errorf("IsPrerelease(%q) = false, want true", v)
		}
	}

	releases := []string{"26.3", "26.1.2", "1.21.11", "1.8.9"}
	for _, v := range releases {
		if IsPrerelease(v) {
			t.Errorf("IsPrerelease(%q) = true, want false", v)
		}
	}
}

func TestFilterVersions(t *testing.T) {
	versions := []VersionInfo{
		{ID: "26.3", Type: "release"},
		{ID: "26.3-rc-1", Type: "snapshot"},
		{ID: "1.21.11", Type: "release"},
	}

	if got := filterVersions(versions, true); len(got) != 3 {
		t.Errorf("filterVersions(includeSnapshots=true) returned %d versions, want 3", len(got))
	}

	releases := filterVersions(versions, false)
	if len(releases) != 2 {
		t.Fatalf("filterVersions(includeSnapshots=false) returned %d versions, want 2", len(releases))
	}
	for _, v := range releases {
		if v.Type != "release" {
			t.Errorf("filterVersions kept non-release %q", v.ID)
		}
	}
}

func TestRegistryHasAllFlavors(t *testing.T) {
	r := NewRegistry()
	for _, flavor := range []string{"vanilla", "paper", "purpur"} {
		p, ok := r.GetProvider(flavor)
		if !ok {
			t.Fatalf("provider %q not registered", flavor)
		}
		if p.Flavor() != flavor {
			t.Errorf("provider %q reports flavor %q", flavor, p.Flavor())
		}
	}

	if _, ok := r.GetProvider("forge"); ok {
		t.Error("unknown flavor should not resolve")
	}

	if len(r.GetAllFlavors()) != 3 {
		t.Errorf("GetAllFlavors returned %v, want 3 flavors", r.GetAllFlavors())
	}
}
