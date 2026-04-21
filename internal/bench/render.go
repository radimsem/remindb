package bench

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

func renderResults(w io.Writer, results []scenarioResult) error {
	if len(results) == 0 {
		_, err := fmt.Fprintln(w, "no scenarios ran")
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "scenario\tnaive (tok)\tremindb (tok)\tsaved"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(tw, strings.Repeat("-", 8)+"\t"+strings.Repeat("-", 11)+"\t"+strings.Repeat("-", 13)+"\t"+strings.Repeat("-", 5)); err != nil {
		return err
	}

	var totalNaive, totalRemindb int
	for _, r := range results {
		if _, err := fmt.Fprintf(tw, "%s\t~%d\t~%d\t%s\n", r.name, r.naiveTok, r.remindbTok, savedPct(r.naiveTok, r.remindbTok)); err != nil {
			return err
		}

		totalNaive += r.naiveTok
		totalRemindb += r.remindbTok
	}

	if len(results) > 1 {
		if _, err := fmt.Fprintf(tw, "total\t~%d\t~%d\t%s\n", totalNaive, totalRemindb, savedPct(totalNaive, totalRemindb)); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func savedPct(naive, remindb int) string {
	if naive <= 0 {
		return "n/a"
	}

	pct := 100.0 * (1.0 - float64(remindb)/float64(naive))
	return fmt.Sprintf("%+.1f%%", pct)
}
