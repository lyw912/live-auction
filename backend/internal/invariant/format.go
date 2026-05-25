package invariant

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func WriteJSON(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func WriteMarkdown(w io.Writer, report Report) error {
	if _, err := fmt.Fprintf(w, "# Auction Invariant Verifier\n\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- status: %s\n- generated_at: %s\n- auction_id: %s\n- room_id: %s\n- passed: %d\n- warned: %d\n- failed: %d\n- skipped: %d\n\n",
		report.Status,
		report.GeneratedAt.Format("2006-01-02T15:04:05.000Z07:00"),
		emptyDash(report.Scope.AuctionID),
		emptyDash(report.Scope.RoomID),
		report.Summary.Passed,
		report.Summary.Warned,
		report.Summary.Failed,
		report.Summary.Skipped,
	); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| Check | Severity | Status | Count |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---:|---:|---:|"); err != nil {
		return err
	}
	for _, check := range report.Checks {
		if _, err := fmt.Fprintf(w, "| %s | %s | %s | %d |\n", escapeCell(check.Name), check.Severity, check.Status, check.Count); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\n## Non-Passing Details"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	wrote := false
	for _, check := range report.Checks {
		if check.Status == StatusPass || check.Status == StatusSkip {
			continue
		}
		wrote = true
		if _, err := fmt.Fprintf(w, "### %s\n\n%s\n\n", check.Name, check.Description); err != nil {
			return err
		}
		for _, detail := range check.Details {
			parts := make([]string, 0, len(detail))
			for _, key := range sortedKeys(detail) {
				parts = append(parts, fmt.Sprintf("%s=%v", key, detail[key]))
			}
			if _, err := fmt.Fprintf(w, "- %s\n", strings.Join(parts, ", ")); err != nil {
				return err
			}
		}
		if len(check.Details) == 0 {
			if _, err := fmt.Fprintln(w, "- violation count exceeded detail capture or no row details were available"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	if !wrote {
		_, err := fmt.Fprintln(w, "All checks passed.")
		return err
	}
	return nil
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func escapeCell(value string) string {
	return strings.ReplaceAll(value, "|", "/")
}
