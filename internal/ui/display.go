package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"appcleaner/internal/matcher"
)

// ANSI color codes (falls back gracefully if not in TTY)
var (
	ColorReset  = "\033[0m"
	ColorBold   = "\033[1m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[90m"
)

func init() {
	// Disable colors if NO_COLOR env is set or TERM is dumb
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		DisableColors()
	}
}

// DisableColors removes ANSI codes.
func DisableColors() {
	ColorReset = ""
	ColorBold = ""
	ColorRed = ""
	ColorGreen = ""
	ColorYellow = ""
	ColorBlue = ""
	ColorCyan = ""
	ColorGray = ""
}

// FormatBytes formats byte counts into human-readable strings.
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// PrintBanner outputs a header banner.
func PrintBanner(w io.Writer) {
	fmt.Fprintf(w, "%s%s=== macOS Uninstalled Application Support Cleaner ===%s\n\n", ColorBold, ColorCyan, ColorReset)
}

// PrintCandidates outputs the numbered list of uninstalled apps and their support files.
func PrintCandidates(w io.Writer, candidates []*matcher.Candidate, showDetails bool) {
	if len(candidates) == 0 {
		fmt.Fprintf(w, "%sNo orphaned application support folders found.%s\n", ColorGreen, ColorReset)
		return
	}

	fmt.Fprintf(w, "%sFound %d orphaned support folder(s) for uninstalled applications:%s\n", ColorBold, len(candidates), ColorReset)
	fmt.Fprintln(w, strings.Repeat("-", 76))

	var totalBytes int64
	for i, c := range candidates {
		totalBytes += c.Size
		fmt.Fprintf(w, "%s[%d] %s%s%s\n", ColorBold, i+1, ColorYellow, c.AppName, ColorReset)
		fmt.Fprintf(w, "    Path:   %s%s%s\n", ColorCyan, c.Path, ColorReset)
		fmt.Fprintf(w, "    Size:   %s%s%s (%d files)\n", ColorGreen, FormatBytes(c.Size), ColorReset, c.FileCount)
		fmt.Fprintf(w, "    Reason: %s%s%s\n", ColorGray, c.Reason, ColorReset)

		if showDetails && len(c.SampleFiles) > 0 {
			fmt.Fprintf(w, "    Sample Files:\n")
			for _, f := range c.SampleFiles {
				fmt.Fprintf(w, "      • %s\n", f)
			}
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, strings.Repeat("-", 76))
	fmt.Fprintf(w, "%sTotal reclaimable disk space:%s %s%s%s\n\n",
		ColorBold, ColorReset, ColorBold+ColorGreen, FormatBytes(totalBytes), ColorReset)
}

// PromptConfirmation prompts the user for action: y (all), s (select), d (details), n (cancel).
func PromptConfirmation(r io.Reader, w io.Writer, total int) (action string, selected []int) {
	scanner := bufio.NewScanner(r)

	for {
		fmt.Fprintf(w, "%sConfirm deletion:%s\n", ColorBold, ColorReset)
		fmt.Fprintf(w, "  %s[y]%s Delete all %d orphaned folders\n", ColorGreen, ColorReset, total)
		fmt.Fprintf(w, "  %s[s]%s Select specific folders to delete (e.g. 1, 3-5)\n", ColorYellow, ColorReset)
		fmt.Fprintf(w, "  %s[d]%s Toggle detailed file preview\n", ColorCyan, ColorReset)
		fmt.Fprintf(w, "  %s[n]%s Cancel and exit without deleting (default)\n", ColorRed, ColorReset)
		fmt.Fprintf(w, "\n%sEnter your choice [y/s/d/N]:%s ", ColorBold, ColorReset)

		if !scanner.Scan() {
			// EOF or interrupt
			return "n", nil
		}

		choice := strings.ToLower(strings.TrimSpace(scanner.Text()))
		switch choice {
		case "y", "yes":
			all := make([]int, total)
			for i := 0; i < total; i++ {
				all[i] = i
			}
			return "y", all
		case "d", "details":
			return "d", nil
		case "s", "select":
			fmt.Fprintf(w, "\n%sEnter folder numbers to delete (e.g. '1, 3' or '1-%d') or 'all':%s ", ColorBold, total, ColorReset)
			if !scanner.Scan() {
				return "n", nil
			}
			input := strings.TrimSpace(scanner.Text())
			indices, err := ParseSelection(input, total)
			if err != nil {
				fmt.Fprintf(w, "%sError: %v%s\n\n", ColorRed, err, ColorReset)
				continue
			}
			if len(indices) == 0 {
				fmt.Fprintf(w, "%sNo valid folders selected.%s\n\n", ColorYellow, ColorReset)
				continue
			}
			return "s", indices
		case "n", "no", "q", "quit", "":
			return "n", nil
		default:
			fmt.Fprintf(w, "%sInvalid choice %q. Please choose [y], [s], [d], or [n].%s\n\n", ColorRed, choice, ColorReset)
		}
	}
}

// ParseSelection parses user input string like "1, 3, 5-7" into zero-based slice of indices.
func ParseSelection(input string, max int) ([]int, error) {
	input = strings.TrimSpace(input)
	if strings.ToLower(input) == "all" {
		res := make([]int, max)
		for i := 0; i < max; i++ {
			res[i] = i
		}
		return res, nil
	}

	selectedSet := make(map[int]struct{})
	tokens := strings.Split(input, ",")

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		if strings.Contains(token, "-") {
			parts := strings.Split(token, "-")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid range format: %q", token)
			}
			start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil || start > end {
				return nil, fmt.Errorf("invalid range numbers in: %q", token)
			}
			for i := start; i <= end; i++ {
				if i < 1 || i > max {
					return nil, fmt.Errorf("index %d out of bounds (1..%d)", i, max)
				}
				selectedSet[i-1] = struct{}{}
			}
		} else {
			val, err := strconv.Atoi(token)
			if err != nil {
				return nil, fmt.Errorf("invalid number: %q", token)
			}
			if val < 1 || val > max {
				return nil, fmt.Errorf("index %d out of bounds (1..%d)", val, max)
			}
			selectedSet[val-1] = struct{}{}
		}
	}

	var res []int
	for idx := range selectedSet {
		res = append(res, idx)
	}
	sort.Ints(res)
	return res, nil
}
