package server

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/log"
)

// selectLocalDir opens the interactive picker (same UX as leap-cli's `leap push`
// model-path prompt) for choosing a dataset volume's local directory:
//   - type a path; matching subdirectories are listed as you go
//   - ↑↓ to move, TAB to descend into the highlighted directory
//   - ENTER selects the highlighted directory, or the typed path if none matches
//
// The default is pre-filled, so ENTER on an untouched prompt keeps it. A path
// that doesn't exist yet is accepted verbatim (the caller creates it via
// MkdirAll). The advanced "localdir:containerdir" form is passed through
// untouched once ':' is typed — only the local side gets completion.
func selectLocalDir(title, defaultPath string) (string, error) {
	onInputChange := func(cursor int, filter string) (string, []string, int, string, string) {
		if strings.Contains(filter, ":") {
			return title, nil, cursor, "", "container path — press ENTER to accept"
		}
		dir, prefix := splitForListing(filter)
		return title, listDirs(dir, prefix), cursor, "", "dir: " + dir
	}

	onEnter := func(options []string, cursor int, filter string) (*string, string, string, string) {
		trimmed := strings.TrimSpace(filter)
		if trimmed == "" {
			res := defaultPath
			return &res, filter, "", ""
		}
		if !strings.Contains(filter, ":") && cursor >= 0 && cursor < len(options) {
			dir, _ := splitForListing(filter)
			res := strings.TrimRight(filepath.Join(dir, options[cursor]), string(os.PathSeparator))
			return &res, filter, "", ""
		}
		// Accept the typed value (may not exist yet, or is a local:container form).
		return &trimmed, filter, "", ""
	}

	onTab := func(options []string, cursor int, filter string) string {
		if cursor < 0 || cursor >= len(options) {
			return filter
		}
		dir, _ := splitForListing(filter)
		// Keep the trailing separator so the next listing shows the dir's children.
		return filepath.Join(dir, options[cursor]) + string(os.PathSeparator)
	}

	return log.InteractiveSelectTea(defaultPath, onInputChange, onEnter, onTab)
}

// splitForListing turns the typed path into the directory to list and the
// name-prefix to match. A trailing separator means "list this dir's children".
func splitForListing(filter string) (dir, prefix string) {
	p := expandHome(filter)
	if p == "" {
		return ".", ""
	}
	if strings.HasSuffix(p, string(os.PathSeparator)) {
		return filepath.Clean(p), ""
	}
	return filepath.Dir(p), filepath.Base(p)
}

// listDirs returns the subdirectories of dir whose name contains prefix
// (case-insensitive), each with a trailing separator so TAB descends into them.
func listDirs(dir, prefix string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	lp := strings.ToLower(prefix)
	out := []string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue // dataset volumes mount directories
		}
		if lp != "" && !strings.Contains(strings.ToLower(e.Name()), lp) {
			continue
		}
		out = append(out, e.Name()+string(os.PathSeparator))
	}
	return out
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + strings.TrimPrefix(p, "~")
		}
	}
	return p
}
