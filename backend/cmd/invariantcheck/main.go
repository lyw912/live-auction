package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"live-auction/backend/internal/config"
	"live-auction/backend/internal/invariant"
)

func main() {
	var auctionID string
	var roomID string
	var format string
	var outPath string
	var maxDetails int
	var failOnWarn bool
	var timeout time.Duration
	flag.StringVar(&auctionID, "auction", "", "limit checks to one auction id")
	flag.StringVar(&roomID, "room", "", "limit checks to one room id")
	flag.StringVar(&format, "format", "json", "output format: json or markdown")
	flag.StringVar(&outPath, "out", "", "write report to file instead of stdout")
	flag.IntVar(&maxDetails, "max-details", 20, "maximum detail rows per check")
	flag.BoolVar(&failOnWarn, "fail-on-warn", false, "exit non-zero when warnings are present")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "database check timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cfg := config.Load()
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fail("open postgres: %v", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		fail("ping postgres: %v", err)
	}

	checker := invariant.NewChecker(db)
	report, err := checker.Run(ctx, invariant.Options{
		AuctionID:  auctionID,
		RoomID:     roomID,
		MaxDetails: maxDetails,
	})
	if err != nil {
		fail("run invariant checks: %v", err)
	}

	var output *os.File
	if outPath == "" {
		output = os.Stdout
	} else {
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil && filepath.Dir(outPath) != "." {
			fail("create output directory: %v", err)
		}
		output, err = os.Create(outPath)
		if err != nil {
			fail("create output file: %v", err)
		}
		defer output.Close()
	}

	switch strings.ToLower(format) {
	case "json":
		err = invariant.WriteJSON(output, report)
	case "markdown", "md":
		err = invariant.WriteMarkdown(output, report)
	default:
		fail("unknown format %q", format)
	}
	if err != nil {
		fail("write report: %v", err)
	}

	if report.Status == invariant.StatusFail || (failOnWarn && report.Status == invariant.StatusWarn) {
		os.Exit(1)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "invariantcheck: "+format+"\n", args...)
	os.Exit(2)
}
