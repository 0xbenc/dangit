package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/0xbenc/dangit/internal/fsutil"
	"github.com/0xbenc/dangit/internal/termstyle"
)

// runTheme dispatches theme subcommands. Only `import` is supported; dangit has
// no in-app editor or export (see README).
func (r runner) runTheme(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(r.stderr, themeUsage)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(r.stdout, themeUsage)
		return 0
	case "import":
		return r.runThemeImport(args[1:])
	default:
		fmt.Fprintf(r.stderr, "dangit: unknown theme command %q\n", args[0])
		fmt.Fprint(r.stderr, themeUsage)
		return 2
	}
}

// runThemeImport loads a portable .theme file and writes it as the active theme
// config, backing up the previous one.
func (r runner) runThemeImport(args []string) int {
	f, positional, err := parseFlags(args)
	if err != nil {
		return r.usageErr(err)
	}
	if f.help {
		fmt.Fprint(r.stdout, themeUsage)
		return 0
	}
	if len(positional) != 1 {
		fmt.Fprintln(r.stderr, "dangit: theme import needs exactly one PATH")
		return 2
	}
	src := positional[0]

	data, err := os.ReadFile(src)
	if err != nil {
		fmt.Fprintf(r.stderr, "dangit: read %s: %v\n", src, err)
		return 1
	}
	cfg, meta, err := termstyle.ImportTheme(data)
	if err != nil {
		fmt.Fprintf(r.stderr, "dangit: %s is not a valid theme: %v\n", src, err)
		return 1
	}
	for _, w := range meta.Warnings {
		fmt.Fprintf(r.stderr, "dangit: note: %s\n", w)
	}

	dest, err := termstyle.ThemeConfigPath(f.themeFile, r.env)
	if err != nil {
		fmt.Fprintf(r.stderr, "dangit: %v\n", err)
		return 1
	}
	result, err := fsutil.AtomicWriteFile(dest, formatThemeConfig(cfg), fsutil.WriteOptions{
		Backup:       true,
		BackupPrefix: "dangit-theme-backup",
		Mode:         0o600,
	})
	if err != nil {
		fmt.Fprintf(r.stderr, "dangit: write %s: %v\n", dest, err)
		return 1
	}
	if result.Changed {
		msg := "Theme imported to " + result.Path + "."
		if result.BackupPath != "" {
			msg += " Backup: " + result.BackupPath + "."
		}
		fmt.Fprintln(r.stderr, msg)
	} else {
		fmt.Fprintln(r.stderr, "Theme unchanged.")
	}
	return 0
}

// formatThemeConfig serializes a ThemeConfig to dangit's theme.conf text. The
// chosen base palette is persisted only when it differs from the default so
// configs stay lean; terminal is implicit.
func formatThemeConfig(cfg termstyle.ThemeConfig) []byte {
	var b strings.Builder
	b.WriteString("# dangit theme config\n")
	b.WriteString("# Imported with `dangit theme import`.\n\n")
	if base := strings.TrimSpace(cfg.BaseName); base != "" {
		if t, ok := termstyle.BuiltinTheme(base); ok && t.Name != "terminal" {
			b.WriteString("theme = ")
			b.WriteString(t.Name)
			b.WriteString("\n\n")
		}
	}
	for _, role := range termstyle.Roles() {
		spec := strings.TrimSpace(cfg.Specs[role])
		if spec == "" {
			continue
		}
		b.WriteString(string(role))
		b.WriteString(" = ")
		b.WriteString(spec)
		b.WriteString("\n")
	}
	return []byte(b.String())
}
