package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0xbenc/dangit/internal/scan"
)

// flags is the parsed union of every dangit flag. Commands read the subset they
// care about; unknown flags are rejected during parsing.
type flags struct {
	timeout     time.Duration
	timeoutSet  bool
	noNetwork   bool
	json        bool
	plain       bool
	noColor     bool
	noAltScreen bool
	intro       bool
	noIntro     bool
	themeFile   string
	yes         bool
	message     string
	messageSet  bool
	help        bool
}

// parseFlags parses args into flags plus positional arguments. Unknown options
// and missing values are errors (mapped to exit code 2 by callers).
func parseFlags(args []string) (flags, []string, error) {
	var f flags
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positional = append(positional, args[i+1:]...)
			return f, positional, nil
		case a == "-h" || a == "--help":
			f.help = true
		case a == "-t" || a == "--timeout" || a == "--timeout-secs":
			val, err := nextValue(args, &i, a)
			if err != nil {
				return f, nil, err
			}
			d, err := parseTimeout(val)
			if err != nil {
				return f, nil, err
			}
			f.timeout, f.timeoutSet = d, true
		case strings.HasPrefix(a, "--timeout=") || strings.HasPrefix(a, "--timeout-secs="):
			d, err := parseTimeout(a[strings.IndexByte(a, '=')+1:])
			if err != nil {
				return f, nil, err
			}
			f.timeout, f.timeoutSet = d, true
		case a == "--no-network":
			f.noNetwork = true
		case a == "--json":
			f.json = true
		case a == "--plain":
			f.plain = true
		case a == "--no-color":
			f.noColor = true
		case a == "--no-alt-screen":
			f.noAltScreen = true
		case a == "--intro":
			f.intro = true
		case a == "--no-intro":
			f.noIntro = true
		case a == "--theme-file":
			val, err := nextValue(args, &i, a)
			if err != nil {
				return f, nil, err
			}
			f.themeFile = val
		case strings.HasPrefix(a, "--theme-file="):
			f.themeFile = a[strings.IndexByte(a, '=')+1:]
		case a == "-y" || a == "--yes":
			f.yes = true
		case a == "-m" || a == "--message":
			val, err := nextValue(args, &i, a)
			if err != nil {
				return f, nil, err
			}
			f.message, f.messageSet = val, true
		case strings.HasPrefix(a, "--message="):
			f.message, f.messageSet = a[strings.IndexByte(a, '=')+1:], true
		case a != "-" && strings.HasPrefix(a, "-"):
			return f, nil, fmt.Errorf("unknown option: %s", a)
		default:
			positional = append(positional, a)
		}
	}
	return f, positional, nil
}

func nextValue(args []string, i *int, opt string) (string, error) {
	*i++
	if *i >= len(args) {
		return "", fmt.Errorf("missing value for %s", opt)
	}
	return args[*i], nil
}

func parseTimeout(s string) (time.Duration, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid timeout %q: want a positive integer of seconds", s)
	}
	return time.Duration(n) * time.Second, nil
}

// resolveTimeout applies precedence: -t flag > DANGIT_TIMEOUT_SECS > default.
func (r runner) resolveTimeout(f flags) (time.Duration, error) {
	if f.timeoutSet {
		return f.timeout, nil
	}
	if v := strings.TrimSpace(r.envValue("DANGIT_TIMEOUT_SECS")); v != "" {
		d, err := parseTimeout(v)
		if err != nil {
			return 0, fmt.Errorf("DANGIT_TIMEOUT_SECS: %w", err)
		}
		return d, nil
	}
	return scan.DefaultTimeout, nil
}

// singlePath returns the single optional PATH argument, defaulting to ".".
func singlePath(positional []string) (string, error) {
	switch len(positional) {
	case 0:
		return ".", nil
	case 1:
		return positional[0], nil
	default:
		return "", fmt.Errorf("expected at most one PATH, got %d", len(positional))
	}
}

// validateDir ensures path exists and is a directory.
func validateDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}
