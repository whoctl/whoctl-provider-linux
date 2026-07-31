package pkgtools

import (
	"context"
	"strings"

	"github.com/whoctl/whoctl-sdk-go/sysexec"
)

// dnfBackend drives dnf, falling back to yum on the older Red Hat family.
//
// This is the one backend that reads through a command. The other three keep
// their databases as flat text, but rpm's is a binary store (sqlite today,
// Berkeley DB before that) with no stable on-disk format to parse, so `rpm -qa`
// with an explicit query format is both the supported interface and the only
// one that will still work after the next rpm release.
type dnfBackend struct{ opts Options }

func (b *dnfBackend) Name() string { return "dnf" }

// Binary prefers dnf and accepts yum, so RHEL 7 and its descendants are not
// reported as having no package manager at all.
func (b *dnfBackend) Binary() string {
	if sysexec.Which("dnf") != "" {
		return "dnf"
	}
	return "yum"
}

func (b *dnfBackend) SupportsVersionPinning() bool { return true }

// rpmQueryFormat asks rpm for exactly the fields Package holds, tab-separated.
// %{VERSION}-%{RELEASE} is the version a manifest pins, matching what
// `dnf install name-version` expects back.
const rpmQueryFormat = `%{NAME}\t%{VERSION}-%{RELEASE}\t%{ARCH}\t%{SUMMARY}\t%{VENDOR}\n`

func (b *dnfBackend) Installed(ctx context.Context) ([]Package, error) {
	if sysexec.Which("rpm") == "" {
		return nil, nil
	}
	out, err := b.opts.Runner.Run(ctx, "rpm", "-qa", "--qf", rpmQueryFormat)
	if err != nil {
		return nil, err
	}
	return parseRPMQuery(out), nil
}

func parseRPMQuery(out string) []Package {
	var pkgs []Package
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		p := Package{Name: fields[0], Version: fields[1]}
		if len(fields) > 2 {
			p.Architecture = fields[2]
		}
		if len(fields) > 3 {
			p.Description = fields[3]
		}
		// rpm prints "(none)" for an unset vendor, which is not an origin.
		if len(fields) > 4 && fields[4] != "(none)" {
			p.Origin = fields[4]
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

func (b *dnfBackend) Install(ctx context.Context, name, version string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	target := name
	if version != "" {
		// dnf pins by joining with a hyphen, the same shape rpm reports.
		target = name + "-" + version
	}
	_, err := b.opts.Runner.Run(ctx, b.Binary(), "install", "-y", target)
	return err
}

func (b *dnfBackend) Remove(ctx context.Context, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	_, err := b.opts.Runner.Run(ctx, b.Binary(), "remove", "-y", name)
	return err
}
