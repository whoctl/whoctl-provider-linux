package pkgtools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoBackend(t *testing.T, opts Options, name string) RepoBackend {
	t.Helper()
	b, err := FindRepos(opts, name)
	if err != nil {
		t.Fatalf("FindRepos(%q): %v", name, err)
	}
	return b
}

func listRepos(t *testing.T, b RepoBackend) map[string]Repository {
	t.Helper()
	repos, err := b.List(context.Background())
	if err != nil {
		t.Fatalf("%s.List: %v", b.Name(), err)
	}
	out := make(map[string]Repository, len(repos))
	for _, r := range repos {
		out[r.Name] = r
	}
	return out
}

// writableRoot copies the fixture tree into a temp directory, so the write
// tests never touch the real machine or the fixtures themselves.
func writableRoot(t *testing.T) Options {
	t.Helper()
	root := t.TempDir()
	err := filepath.WalkDir("testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("testdata", path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copying the fixture tree: %v", err)
	}
	return Options{Root: root}
}

func readFile(t *testing.T, opts Options, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(opts.Root, rel))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(data)
}

// --- apk ---------------------------------------------------------------

func TestAPKReposReadsURLsAndSkipsProse(t *testing.T) {
	repos := listRepos(t, repoBackend(t, fixtureOptions(), "apk"))
	if len(repos) != 4 {
		t.Fatalf("got %d repositories, want 4: %v", len(repos), repos)
	}
	// A commented-out URL is a disabled repository; a commented-out sentence
	// is a comment.
	testing_ := repos["https://dl-cdn.alpinelinux.org/alpine/edge/testing"]
	if testing_.Enabled || testing_.Tag != "testing" {
		t.Errorf("the disabled tagged repository = %+v", testing_)
	}
	for name := range repos {
		if strings.Contains(name, " ") {
			t.Errorf("a prose comment was read as a repository: %q", name)
		}
	}
	// apk also takes local paths.
	if _, ok := repos["/media/cdrom/apks"]; !ok {
		t.Errorf("a local path repository was dropped: %v", repos)
	}
}

func TestAPKReposEnableKeepsPosition(t *testing.T) {
	opts := writableRoot(t)
	b := repoBackend(t, opts, "apk")
	const url = "https://dl-cdn.alpinelinux.org/alpine/edge/testing"

	changed, err := b.Apply(context.Background(), Repository{Name: url, URLs: []string{url}, Enabled: true, Tag: "testing"})
	if err != nil || !changed {
		t.Fatalf("Apply = %v, %v", changed, err)
	}
	lines := strings.Split(strings.TrimSpace(readFile(t, opts, "etc/apk/repositories")), "\n")
	// apk queries repositories in file order, so enabling one must not move it
	// to the end of the file.
	if lines[3] != "@testing "+url {
		t.Errorf("line 4 = %q, want the repository enabled in place\n%v", lines[3], lines)
	}

	// Applying the same thing again changes nothing.
	changed, err = b.Apply(context.Background(), Repository{Name: url, URLs: []string{url}, Enabled: true, Tag: "testing"})
	if err != nil || changed {
		t.Errorf("re-Apply = %v, %v; want no change", changed, err)
	}
}

func TestAPKReposDeleteLeavesTheRestAlone(t *testing.T) {
	opts := writableRoot(t)
	b := repoBackend(t, opts, "apk")
	if err := b.Delete(context.Background(), "https://dl-cdn.alpinelinux.org/alpine/v3.24/main"); err != nil {
		t.Fatal(err)
	}
	content := readFile(t, opts, "etc/apk/repositories")
	if strings.Contains(content, "v3.24/main") {
		t.Errorf("the repository is still there:\n%s", content)
	}
	for _, keep := range []string{"# The main Alpine repositories.", "v3.24/community", "prose and not a repository"} {
		if !strings.Contains(content, keep) {
			t.Errorf("%q was lost:\n%s", keep, content)
		}
	}
}

// --- apt ---------------------------------------------------------------

func TestAptReposReadsBothFormats(t *testing.T) {
	repos := listRepos(t, repoBackend(t, fixtureOptions(), "apt"))

	docker := repos["docker"]
	if docker.Suite != "bookworm" || docker.URLs[0] != "https://download.docker.com/linux/debian" {
		t.Errorf("docker = %+v", docker)
	}
	if docker.Options["signed-by"] != "/etc/apt/keyrings/docker.asc" || docker.Options["arch"] != "amd64" {
		t.Errorf("docker options = %v", docker.Options)
	}
	// The main sources.list is named after its file, like everything else.
	if repos["sources"].Suite != "bookworm" {
		t.Errorf("sources = %+v", repos["sources"])
	}
	// The deb822 file is read and marked, so `get` shows it and `apply` can
	// refuse it.
	if got := repos["modern"].Options["format"]; got != "deb822" {
		t.Errorf("modern format = %q", got)
	}
	// A commented-out source line is a disabled repository, not an absent one.
	// It has to be, or `enabled: false` would make the object disappear and
	// there would be no way to turn it back on.
	disabled, ok := repos["disabled"]
	if !ok {
		t.Fatalf("a commented-out source was dropped instead of read as disabled: %v", repos)
	}
	if disabled.Enabled || disabled.Suite != "bookworm" {
		t.Errorf("disabled = %+v, want it off with its suite intact", disabled)
	}
}

func TestAptReposRefusesToRewriteDeb822(t *testing.T) {
	opts := writableRoot(t)
	b := repoBackend(t, opts, "apt")
	_, err := b.Apply(context.Background(), Repository{
		Name: "modern", URLs: []string{"http://deb.debian.org/debian"}, Suite: "trixie", Enabled: true,
	})
	if err == nil {
		t.Fatal("rewriting a deb822 stanza must be refused, not attempted")
	}
	if !strings.Contains(err.Error(), "deb822") {
		t.Errorf("err = %v, want it to name the format", err)
	}
}

func TestAptReposWritesASortedOptionLine(t *testing.T) {
	opts := writableRoot(t)
	b := repoBackend(t, opts, "apt")
	repo := Repository{
		Name: "example", URLs: []string{"https://deb.example.com/debian"}, Suite: "bookworm",
		Components: []string{"main"}, Types: []string{"deb"}, Enabled: true,
		Options: map[string]string{"signed-by": "/k.asc", "arch": "amd64"},
	}
	if _, err := b.Apply(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(readFile(t, opts, "etc/apt/sources.list.d/example.list"))
	want := "deb [arch=amd64 signed-by=/k.asc] https://deb.example.com/debian bookworm main"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	// Sorted options mean a re-apply is byte-identical rather than a reshuffle.
	changed, err := b.Apply(context.Background(), repo)
	if err != nil || changed {
		t.Errorf("re-Apply = %v, %v; want no change", changed, err)
	}
}

func TestAptReposNeverDeletesTheDistributionsOwnList(t *testing.T) {
	opts := writableRoot(t)
	if err := repoBackend(t, opts, "apt").Delete(context.Background(), "sources"); err == nil {
		t.Fatal("deleting /etc/apt/sources.list must be refused")
	}
}

// --- dnf ---------------------------------------------------------------

func TestDnfReposReadsSectionsAcrossFiles(t *testing.T) {
	repos := listRepos(t, repoBackend(t, fixtureOptions(), "dnf"))
	if len(repos) != 2 {
		t.Fatalf("got %d repositories, want 2: %v", len(repos), repos)
	}
	if !repos["fedora"].Enabled || repos["fedora-debuginfo"].Enabled {
		t.Errorf("enabled flags = %v, %v", repos["fedora"].Enabled, repos["fedora-debuginfo"].Enabled)
	}
	if repos["fedora"].Metalink == "" {
		t.Errorf("the metalink was dropped: %+v", repos["fedora"])
	}
}

func TestDnfReposKeepUnmodelledKeys(t *testing.T) {
	opts := writableRoot(t)
	b := repoBackend(t, opts, "dnf")
	enabled := false
	if _, err := b.Apply(context.Background(), Repository{
		Name: "fedora", Enabled: enabled, DisplayName: "Fedora $releasever - $basearch",
		Metalink: "https://mirrors.fedoraproject.org/metalink?repo=fedora-$releasever",
	}); err != nil {
		t.Fatal(err)
	}
	content := readFile(t, opts, "etc/yum.repos.d/fedora.repo")
	// countme and skip_if_unavailable are not modelled and must survive.
	for _, keep := range []string{"countme=1", "skip_if_unavailable=False", "[fedora-debuginfo]"} {
		if !strings.Contains(content, keep) {
			t.Errorf("%q was lost:\n%s", keep, content)
		}
	}
	if !strings.Contains(content, "enabled=0") {
		t.Errorf("the change was not applied:\n%s", content)
	}
}

func TestDnfReposLeaveGPGCheckAloneWhenOmitted(t *testing.T) {
	opts := writableRoot(t)
	b := repoBackend(t, opts, "dnf")
	// A nil GPGCheck must not become gpgcheck=0: turning off signature
	// verification as a side effect of an unrelated change would be a
	// security regression.
	if _, err := b.Apply(context.Background(), Repository{
		Name: "fedora", Enabled: true, DisplayName: "renamed",
		Metalink: "https://mirrors.fedoraproject.org/metalink?repo=fedora-$releasever",
	}); err != nil {
		t.Fatal(err)
	}
	if content := readFile(t, opts, "etc/yum.repos.d/fedora.repo"); !strings.Contains(content, "gpgcheck=1") {
		t.Errorf("gpgcheck was changed:\n%s", content)
	}
}

func TestDnfReposDeleteRemovesAFileItEmpties(t *testing.T) {
	opts := writableRoot(t)
	b := repoBackend(t, opts, "dnf")
	ctx := context.Background()
	if err := b.Delete(ctx, "fedora"); err != nil {
		t.Fatal(err)
	}
	if content := readFile(t, opts, "etc/yum.repos.d/fedora.repo"); strings.Contains(content, "[fedora]\n") {
		t.Errorf("the section is still there:\n%s", content)
	}
	if err := b.Delete(ctx, "fedora-debuginfo"); err != nil {
		t.Fatal(err)
	}
	// The file held nothing else, so it goes rather than staying as a stub.
	if _, err := os.Stat(filepath.Join(opts.Root, "etc/yum.repos.d/fedora.repo")); !os.IsNotExist(err) {
		t.Errorf("the emptied file was left behind: %v", err)
	}
}

// --- pacman ------------------------------------------------------------

func TestPacmanReposSkipTheOptionsSection(t *testing.T) {
	repos := listRepos(t, repoBackend(t, fixtureOptions(), "pacman"))
	if _, ok := repos["options"]; ok {
		// [options] configures pacman itself; treating it as a repository
		// would let a manifest rewrite the machine's package settings.
		t.Fatalf("[options] was listed as a repository: %v", repos)
	}
	if len(repos) != 3 {
		t.Fatalf("got %d repositories, want core, extra and multilib: %v", len(repos), repos)
	}
	if repos["multilib"].Enabled {
		t.Errorf("a commented-out section must read as disabled")
	}
	if repos["core"].Include != "/etc/pacman.d/mirrorlist" {
		t.Errorf("core = %+v", repos["core"])
	}
}

func TestPacmanReposRefuseToTouchOptions(t *testing.T) {
	opts := writableRoot(t)
	_, err := repoBackend(t, opts, "pacman").Apply(context.Background(), Repository{
		Name: "options", URLs: []string{"https://example.com"}, Enabled: true,
	})
	if err == nil {
		t.Fatal("writing to [options] must be refused")
	}
}

func TestPacmanReposEnableAndPreserveTheRest(t *testing.T) {
	opts := writableRoot(t)
	b := repoBackend(t, opts, "pacman")
	if _, err := b.Apply(context.Background(), Repository{
		Name: "multilib", Include: "/etc/pacman.d/mirrorlist", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	content := readFile(t, opts, "etc/pacman.conf")
	if !strings.Contains(content, "\n[multilib]\nInclude = /etc/pacman.d/mirrorlist") {
		t.Errorf("multilib was not enabled in place:\n%s", content)
	}
	for _, keep := range []string{"[options]", "HoldPkg     = pacman glibc", "# /etc/pacman.conf"} {
		if !strings.Contains(content, keep) {
			t.Errorf("%q was lost:\n%s", keep, content)
		}
	}
}

func TestPacmanReposNeedAServerOrAnInclude(t *testing.T) {
	opts := writableRoot(t)
	_, err := repoBackend(t, opts, "pacman").Apply(context.Background(), Repository{Name: "empty", Enabled: true})
	if err == nil {
		t.Fatal("a section pointing nowhere must be refused")
	}
}
