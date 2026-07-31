package pkgtools

import (
	"context"
	"testing"
)

// fixtureOptions points the file-reading backends at testdata instead of the
// real machine, the same way the rest of the provider's tests do.
func fixtureOptions() Options { return Options{Root: "testdata"} }

func backend(t *testing.T, name string) Backend {
	t.Helper()
	b, err := Find(fixtureOptions(), name)
	if err != nil {
		t.Fatalf("Find(%q): %v", name, err)
	}
	return b
}

func installedByName(t *testing.T, b Backend) map[string]Package {
	t.Helper()
	pkgs, err := b.Installed(context.Background())
	if err != nil {
		t.Fatalf("%s.Installed: %v", b.Name(), err)
	}
	out := make(map[string]Package, len(pkgs))
	for _, p := range pkgs {
		out[p.Name] = p
	}
	return out
}

func TestAPKReadsItsDatabase(t *testing.T) {
	pkgs := installedByName(t, backend(t, "apk"))
	if len(pkgs) != 3 {
		t.Fatalf("got %d packages, want 3: %v", len(pkgs), pkgs)
	}
	curl := pkgs["curl"]
	if curl.Version != "8.21.0-r0" || curl.Architecture != "x86_64" || curl.Origin != "curl" {
		t.Errorf("curl = %+v", curl)
	}
	if curl.Description != "URL retrieval utility and library" {
		t.Errorf("description = %q", curl.Description)
	}
	// The last record has no trailing blank line and still has to come out.
	if _, ok := pkgs["tree"]; !ok {
		t.Errorf("the final record was dropped: %v", pkgs)
	}
}

func TestDpkgSkipsWhatIsNotActuallyInstalled(t *testing.T) {
	pkgs := installedByName(t, backend(t, "apt"))
	for _, absent := range []string{"neverinstalled", "purged"} {
		if _, ok := pkgs[absent]; ok {
			// dpkg keeps records for packages it merely knows about; only
			// "install ok installed" means the files are on disk.
			t.Errorf("%q is not installed but was reported", absent)
		}
	}
	if got := pkgs["bash"].Version; got != "5.2.37-2+b9" {
		t.Errorf("bash version = %q", got)
	}
	if got := pkgs["tree"].Version; got != "2.2.1-1" {
		t.Errorf("tree version = %q", got)
	}
}

func TestDpkgIgnoresContinuationLines(t *testing.T) {
	// The bash record's multi-line Description contains a line that reads like
	// "Version: 9.9.9-not-a-field". It is indented, so it belongs to the
	// description and must not overwrite the real Version field.
	pkgs := installedByName(t, backend(t, "apt"))
	if got := pkgs["bash"].Version; got == "9.9.9-not-a-field" {
		t.Errorf("a continuation line was read as a field: version = %q", got)
	}
}

func TestPacmanReadsOneDirectoryPerPackage(t *testing.T) {
	pkgs := installedByName(t, backend(t, "pacman"))
	if len(pkgs) != 2 {
		t.Fatalf("got %d packages, want 2: %v", len(pkgs), pkgs)
	}
	// The version comes from the desc file, not from the directory name, which
	// carries a stale one in the fixture on purpose.
	if got := pkgs["bash"].Version; got != "5.3.15-1" {
		t.Errorf("bash version = %q, want the value from desc", got)
	}
	if got := pkgs["tree"].Architecture; got != "x86_64" {
		t.Errorf("tree architecture = %q", got)
	}
}

func TestPacmanSkipsADirectoryWithoutADescFile(t *testing.T) {
	// testdata holds an empty "broken" directory: a half-written entry is not
	// a reason to fail the whole listing.
	if _, err := backend(t, "pacman").Installed(context.Background()); err != nil {
		t.Errorf("Installed: %v", err)
	}
}

func TestMissingDatabaseIsNotAnError(t *testing.T) {
	// Every backend exists on every machine, so three of the four always read a
	// database that is not there. That is "nothing installed by this manager",
	// not a failure — the manager's absence is reported by Available instead.
	for _, name := range []string{"apk", "apt", "pacman"} {
		b, err := Find(Options{Root: "testdata/nonexistent"}, name)
		if err != nil {
			t.Fatal(err)
		}
		pkgs, err := b.Installed(context.Background())
		if err != nil {
			t.Errorf("%s.Installed with no database: %v", name, err)
		}
		if len(pkgs) != 0 {
			t.Errorf("%s reported %d packages with no database", name, len(pkgs))
		}
	}
}

func TestParseRPMQuery(t *testing.T) {
	out := "curl\t8.18.0-7.fc44\tx86_64\tA utility for getting files\tFedora Project\n" +
		"gpg-pubkey\tc6e7f081-66b6dccf\t(none)\tgpg(Fedora)\t(none)\n"
	pkgs := parseRPMQuery(out)
	if len(pkgs) != 2 {
		t.Fatalf("got %d packages, want 2", len(pkgs))
	}
	if pkgs[0].Version != "8.18.0-7.fc44" || pkgs[0].Origin != "Fedora Project" {
		t.Errorf("curl = %+v", pkgs[0])
	}
	// rpm prints "(none)" for an unset vendor, which is not an origin.
	if pkgs[1].Origin != "" {
		t.Errorf("origin = %q, want it empty for an unset vendor", pkgs[1].Origin)
	}
}

func TestOnlyPacmanRefusesAVersion(t *testing.T) {
	for name, want := range map[string]bool{"apk": true, "apt": true, "dnf": true, "pacman": false} {
		if got := backend(t, name).SupportsVersionPinning(); got != want {
			t.Errorf("%s.SupportsVersionPinning() = %v, want %v", name, got, want)
		}
	}
}

func TestPacmanInstallRejectsAVersion(t *testing.T) {
	err := backend(t, "pacman").Install(context.Background(), "tree", "2.3.2-1")
	if err == nil {
		t.Fatal("pacman must refuse a pinned version rather than install another one")
	}
}

func TestValidateNameRejectsWhatWouldReachArgv(t *testing.T) {
	for _, bad := range []string{"", "  ", "curl=1.2", "curl 8", "curl;rm -rf /", "a|b", "$(id)"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("ValidateName(%q) accepted it", bad)
		}
	}
	for _, good := range []string{"curl", "openssh-server", "python3.12", "lib32-glibc", "g++"} {
		if err := ValidateName(good); err != nil {
			t.Errorf("ValidateName(%q): %v", good, err)
		}
	}
}
