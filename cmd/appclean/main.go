package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"appcleaner/internal/appindex"
	"appcleaner/internal/cleaner"
	"appcleaner/internal/cliindex"
	"appcleaner/internal/matcher"
	"appcleaner/internal/scanner"
	"appcleaner/internal/ui"
)

func main() {
	var (
		flagPaths       string
		flagCaches      bool
		flagContainers  bool
		flagXcode       bool
		flagPrefs       bool
		flagDryRun      bool
		flagYes         bool
		flagTrash       bool
		flagVerbose     bool
		flagWorkers     int
		flagMinSize     string
		flagExclude     string
		flagNoSpotlight bool
		flagCliDirs     string
		flagNoCli       bool
	)

	flag.StringVar(&flagPaths, "path", "", "Custom target directory or directories (comma-separated)")
	flag.StringVar(&flagPaths, "p", "", "Alias for -path")
	flag.BoolVar(&flagCaches, "caches", true, "Include Caches directories (~/Library/Caches, /Library/Caches)")
	flag.BoolVar(&flagContainers, "containers", true, "Include unused Containers (~/Library/Containers, ~/Library/Group Containers)")
	flag.BoolVar(&flagXcode, "xcode", true, "Include Xcode cleanups (DerivedData, iOS DeviceSupport, CoreSimulator)")
	flag.BoolVar(&flagPrefs, "prefs", false, "Include Preferences directory (~/Library/Preferences)")
	flag.BoolVar(&flagDryRun, "dry-run", false, "Preview uninstalled apps and support files without deleting")
	flag.BoolVar(&flagDryRun, "n", false, "Alias for -dry-run")
	flag.BoolVar(&flagYes, "yes", false, "Skip confirmation prompt and delete automatically")
	flag.BoolVar(&flagYes, "y", false, "Alias for -yes")
	flag.BoolVar(&flagTrash, "trash", false, "Move to macOS Trash instead of permanently deleting")
	flag.BoolVar(&flagVerbose, "verbose", false, "Show detailed logging during scanning")
	flag.BoolVar(&flagVerbose, "v", false, "Alias for -verbose")
	flag.IntVar(&flagWorkers, "workers", runtime.NumCPU()*2, "Number of concurrent worker goroutines")
	flag.IntVar(&flagWorkers, "w", runtime.NumCPU()*2, "Alias for -workers")
	flag.StringVar(&flagMinSize, "min-size", "0", "Minimum folder size to consider (e.g. 100KB, 1MB, 1GB)")
	flag.StringVar(&flagExclude, "exclude", "", "Comma-separated list of folder names or patterns to exclude")
	flag.BoolVar(&flagNoSpotlight, "no-spotlight", false, "Disable Spotlight (mdfind) integration")
	flag.StringVar(&flagCliDirs, "cli-dirs", "", "Additional directories containing custom CLI binaries (comma-separated)")
	flag.BoolVar(&flagNoCli, "no-cli", false, "Disable CLI tool detection and protection")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: appclean [options]\n\n")
		fmt.Fprintf(os.Stderr, "Concurrent macOS CLI tool to find and delete orphaned Application Support,\n")
		fmt.Fprintf(os.Stderr, "Caches, Containers, and Xcode build/simulator artifacts.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  appclean -dry-run               # Preview all reclaimable folders without deleting\n")
		fmt.Fprintf(os.Stderr, "  appclean                        # Scan and interactively confirm deletion\n")
		fmt.Fprintf(os.Stderr, "  appclean -xcode=false           # Skip Xcode DerivedData / Simulator cleanup\n")
		fmt.Fprintf(os.Stderr, "  appclean -trash                 # Move folders to Trash instead of removing\n")
		fmt.Fprintf(os.Stderr, "  appclean -min-size 10MB         # Only show folders larger than 10MB\n")
	}

	flag.Parse()

	ui.PrintBanner(os.Stdout)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logVerbose := func(format string, args ...any) {
		if flagVerbose {
			fmt.Printf(ui.ColorGray+"[INFO] "+format+ui.ColorReset+"\n", args...)
		}
	}

	minSizeBytes := parseSize(flagMinSize)

	// Step 1: Concurrently discover installed GUI applications and CLI tools
	fmt.Printf("%s[*] Concurrently discovering installed macOS applications and CLI tools...%s\n", ui.ColorCyan, ui.ColorReset)

	var (
		appIdx *appindex.Index
		cliIdx *cliindex.Index
		appErr error
		cliErr error
		idxWg  sync.WaitGroup
	)

	idxWg.Add(1)
	go func() {
		defer idxWg.Done()
		appIdx, appErr = appindex.ScanInstalledApps(ctx, appindex.ScanConfig{
			Workers:      flagWorkers,
			UseSpotlight: !flagNoSpotlight,
			LogVerbose:   logVerbose,
		})
	}()

	if !flagNoCli {
		idxWg.Add(1)
		go func() {
			defer idxWg.Done()
			var extraDirs []string
			if flagCliDirs != "" {
				for _, d := range strings.Split(flagCliDirs, ",") {
					if d = strings.TrimSpace(d); d != "" {
						extraDirs = append(extraDirs, d)
					}
				}
			}
			cliIdx, cliErr = cliindex.ScanInstalledCLITools(ctx, cliindex.ScanConfig{
				ExtraDirs:  extraDirs,
				LogVerbose: logVerbose,
			})
		}()
	}

	idxWg.Wait()

	if appErr != nil {
		fmt.Fprintf(os.Stderr, "%sError discovering installed applications: %v%s\n", ui.ColorRed, appErr, ui.ColorReset)
		os.Exit(1)
	}
	if cliErr != nil {
		logVerbose("Warning scanning CLI tools: %v", cliErr)
	}

	cliCount := 0
	if cliIdx != nil {
		cliCount = cliIdx.TotalCount()
	}
	fmt.Printf("%s[+] Indexed %d GUI applications and %d CLI tools.%s\n\n", ui.ColorGreen, len(appIdx.Apps), cliCount, ui.ColorReset)

	// Step 2: Initialize Matcher with installed apps, CLI tools, and exclusions
	var excludes []string
	if flagExclude != "" {
		excludes = strings.Split(flagExclude, ",")
	}
	m := matcher.NewMatcher(appIdx, cliIdx, excludes)

	// Step 3: Concurrently scan target support, cache, container, and Xcode directories
	fmt.Printf("%s[*] Scanning Application Support, Caches, Containers, and Xcode folders...%s\n", ui.ColorCyan, ui.ColorReset)
	var targetDirs []string
	if flagPaths != "" {
		for _, p := range strings.Split(flagPaths, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				targetDirs = append(targetDirs, p)
			}
		}
	}

	scanRes, err := scanner.ScanTargets(ctx, m, scanner.ScanTargetConfig{
		TargetDirs:        targetDirs,
		IncludeCaches:     flagCaches,
		IncludeContainers: flagContainers,
		IncludeXcode:      flagXcode,
		IncludePrefs:      flagPrefs,
		MinSizeBytes:      minSizeBytes,
		Workers:           flagWorkers,
		LogVerbose:        logVerbose,
		PermissionWarn: func(path string, err error) {
			logVerbose("Permission warning on %s: %v", path, err)
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError scanning support directories: %v%s\n", ui.ColorRed, err, ui.ColorReset)
		os.Exit(1)
	}

	if len(scanRes.Warnings) > 0 {
		fmt.Printf("\n%s[!] Warning: Some directories could not be accessed:%s\n", ui.ColorYellow, ui.ColorReset)
		for _, w := range scanRes.Warnings {
			fmt.Printf("    • %s\n", w)
		}
		fmt.Printf("    %sTip: To scan protected user directories, grant Full Disk Access to your Terminal in:%s\n", ui.ColorGray, ui.ColorReset)
		fmt.Printf("    %sSystem Settings > Privacy & Security > Full Disk Access%s\n\n", ui.ColorGray, ui.ColorReset)
	}

	candidates := scanRes.Candidates
	if len(candidates) == 0 {
		fmt.Printf("%s[✓] No orphaned folders found. Everything is clean!%s\n", ui.ColorGreen, ui.ColorReset)
		return
	}

	// Sort candidates largest first so biggest space savers appear at the top
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Size > candidates[j].Size
	})

	// Step 4: Display candidates
	showDetails := false
	ui.PrintCandidates(os.Stdout, candidates, showDetails)

	if flagDryRun {
		fmt.Printf("%s[DRY RUN] Preview mode enabled. No files were deleted.%s\n", ui.ColorYellow, ui.ColorReset)
		return
	}

	// Step 5: Confirmation prompt
	var toDelete []*matcher.Candidate
	if flagYes {
		toDelete = candidates
	} else {
		for {
			action, selected := ui.PromptConfirmation(os.Stdin, os.Stdout, len(candidates))
			if action == "d" {
				showDetails = !showDetails
				fmt.Println()
				ui.PrintCandidates(os.Stdout, candidates, showDetails)
				continue
			}
			if action == "n" {
				fmt.Printf("\n%sOperation cancelled. No files were deleted.%s\n", ui.ColorYellow, ui.ColorReset)
				return
			}
			if action == "y" || action == "s" {
				for _, idx := range selected {
					toDelete = append(toDelete, candidates[idx])
				}
				break
			}
		}
	}

	if len(toDelete) == 0 {
		fmt.Printf("%sNo folders selected for deletion.%s\n", ui.ColorYellow, ui.ColorReset)
		return
	}

	// Step 6: Concurrently delete/trash confirmed folders
	mode := cleaner.ModePermanent
	actionVerb := "Deleting"
	if flagTrash {
		mode = cleaner.ModeTrash
		actionVerb = "Moving to Trash"
	}

	fmt.Printf("\n%s[*] %s %d folder(s) concurrently using %d workers...%s\n",
		ui.ColorCyan, actionVerb, len(toDelete), flagWorkers, ui.ColorReset)

	res := cleaner.DeleteCandidates(ctx, toDelete, cleaner.CleanOptions{
		Mode:    mode,
		DryRun:  false,
		Workers: flagWorkers,
		OnProgress: func(completed, total int, cand *matcher.Candidate, err error) {
			if err != nil {
				fmt.Printf("%s[%d/%d] FAILED: %s: %v%s\n",
					ui.ColorRed, completed, total, cand.Path, err, ui.ColorReset)
			} else {
				fmt.Printf("%s[%d/%d] Removed: %s%s (%s)\n",
					ui.ColorGreen, completed, total, ui.ColorReset, cand.Path, ui.FormatBytes(cand.Size))
			}
		},
	})

	// Step 7: Summary report
	fmt.Println()
	fmt.Println(strings.Repeat("=", 76))
	if res.FailedCount > 0 {
		fmt.Printf("%sCompleted with %d error(s).%s\n", ui.ColorYellow, res.FailedCount, ui.ColorReset)
	} else {
		fmt.Printf("%sCleanup successfully finished!%s\n", ui.ColorGreen+ui.ColorBold, ui.ColorReset)
	}
	fmt.Printf("• Folders removed: %d of %d\n", res.SuccessCount, len(toDelete))
	fmt.Printf("• Total space reclaimed: %s%s%s\n", ui.ColorBold+ui.ColorGreen, ui.FormatBytes(res.TotalFreedBytes), ui.ColorReset)
	fmt.Println(strings.Repeat("=", 76))
}

func parseSize(s string) int64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" || s == "0" {
		return 0
	}

	var multiplier int64 = 1
	var numStr string

	if strings.HasSuffix(s, "GB") || strings.HasSuffix(s, "G") {
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(strings.TrimSuffix(s, "GB"), "G")
	} else if strings.HasSuffix(s, "MB") || strings.HasSuffix(s, "M") {
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(strings.TrimSuffix(s, "MB"), "M")
	} else if strings.HasSuffix(s, "KB") || strings.HasSuffix(s, "K") {
		multiplier = 1024
		numStr = strings.TrimSuffix(strings.TrimSuffix(s, "KB"), "K")
	} else if strings.HasSuffix(s, "B") {
		multiplier = 1
		numStr = strings.TrimSuffix(s, "B")
	} else {
		numStr = s
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil {
		return 0
	}
	return int64(val * float64(multiplier))
}
