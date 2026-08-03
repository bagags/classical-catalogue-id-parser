package main

import (
	"fmt"
	"io"
	"math"
	"sort"
)

const wilsonZ95 = 1.959963984540054

type fieldMetrics struct {
	RepresentativeTotal int
	Valid               int
	Invalid             int
	Uncertain           int
	Skipped             int
	Unreviewed          int
	Precision           float64
	WilsonLower         float64
	WilsonUpper         float64
	BoundLower          float64
	BoundUpper          float64
	HasPrecision        bool
	HasBounds           bool
}

func calculateFieldMetrics(items []sampleItem, latest map[string]judgment, field string) fieldMetrics {
	var result fieldMetrics
	for _, item := range items {
		if item.Field != field || item.Cohort != cohortRepresentative {
			continue
		}
		result.RepresentativeTotal++
		value, reviewed := latest[item.ID]
		if !reviewed {
			result.Unreviewed++
			continue
		}
		switch value.Decision {
		case decisionValid:
			result.Valid++
		case decisionInvalid:
			result.Invalid++
		case decisionUncertain:
			result.Uncertain++
		case decisionSkip:
			result.Skipped++
		}
	}
	decided := result.Valid + result.Invalid
	if decided > 0 {
		result.HasPrecision = true
		result.Precision = float64(result.Valid) / float64(decided)
		result.WilsonLower, result.WilsonUpper = wilsonInterval(result.Valid, decided)
	}
	boundDenominator := result.Valid + result.Invalid + result.Uncertain + result.Unreviewed
	if boundDenominator > 0 {
		result.HasBounds = true
		result.BoundLower = float64(result.Valid) / float64(boundDenominator)
		result.BoundUpper = float64(result.Valid+result.Uncertain+result.Unreviewed) / float64(boundDenominator)
	}
	return result
}

func wilsonInterval(successes, trials int) (float64, float64) {
	if trials <= 0 {
		return 0, 0
	}
	n := float64(trials)
	p := float64(successes) / n
	zSquared := wilsonZ95 * wilsonZ95
	denominator := 1 + zSquared/n
	center := (p + zSquared/(2*n)) / denominator
	margin := wilsonZ95 * math.Sqrt(p*(1-p)/n+zSquared/(4*n*n)) / denominator
	return max(0, center-margin), min(1, center+margin)
}

func printSummary(output io.Writer, value evaluation, top int) {
	reviewed := len(value.LatestJudgments)
	fmt.Fprintf(output, "Review completion: %d/%d (%.1f%%)\n", reviewed, len(value.Items), percentage(reviewed, len(value.Items)))
	for _, field := range []string{fieldNumber, fieldTitle} {
		for _, cohort := range []string{cohortRepresentative, cohortDiagnostic} {
			total, complete := completionCounts(value.Items, value.LatestJudgments, field, cohort)
			fmt.Fprintf(output, "  %s/%s: %d/%d (%.1f%%)\n", field, cohort, complete, total, percentage(complete, total))
		}
	}

	fmt.Fprintln(output, "\nRepresentative precision (diagnostic cohort excluded):")
	for _, field := range []string{fieldNumber, fieldTitle} {
		metrics := calculateFieldMetrics(value.Items, value.LatestJudgments, field)
		fmt.Fprintf(output, "  %s: representative=%d valid=%d invalid=%d uncertain=%d skip=%d unreviewed=%d\n",
			field, metrics.RepresentativeTotal, metrics.Valid, metrics.Invalid,
			metrics.Uncertain, metrics.Skipped, metrics.Unreviewed)
		if metrics.HasPrecision {
			fmt.Fprintf(output, "    decided precision: %.2f%% (%d/%d); 95%% Wilson interval: %.2f%%–%.2f%%\n",
				100*metrics.Precision, metrics.Valid, metrics.Valid+metrics.Invalid,
				100*metrics.WilsonLower, 100*metrics.WilsonUpper)
		} else {
			fmt.Fprintln(output, "    decided precision: n/a (no valid or invalid judgments)")
		}
		if metrics.HasBounds {
			fmt.Fprintf(output, "    outcome bounds: %.2f%%–%.2f%% (uncertain/unreviewed treated as invalid/valid; skips excluded)\n",
				100*metrics.BoundLower, 100*metrics.BoundUpper)
		} else {
			fmt.Fprintln(output, "    outcome bounds: n/a (only skipped items)")
		}
	}

	fmt.Fprintln(output, "\nDiagnostic cohort (exploratory counts; not prevalence estimates):")
	for _, field := range []string{fieldNumber, fieldTitle} {
		counts := decisionCounts(value.Items, value.LatestJudgments, field, cohortDiagnostic)
		fmt.Fprintf(output, "  %s: valid=%d invalid=%d uncertain=%d skip=%d unreviewed=%d\n",
			field, counts[decisionValid], counts[decisionInvalid], counts[decisionUncertain], counts[decisionSkip], counts["unreviewed"])
	}

	printDecisionBreakdowns(output, value, decisionInvalid, top)
	printDecisionBreakdowns(output, value, decisionUncertain, top)
}

func completionCounts(items []sampleItem, latest map[string]judgment, field, cohort string) (total, complete int) {
	for _, item := range items {
		if item.Field != field || item.Cohort != cohort {
			continue
		}
		total++
		if _, ok := latest[item.ID]; ok {
			complete++
		}
	}
	return total, complete
}

func decisionCounts(items []sampleItem, latest map[string]judgment, field, cohort string) map[string]int {
	counts := map[string]int{
		decisionValid: 0, decisionInvalid: 0, decisionUncertain: 0,
		decisionSkip: 0, "unreviewed": 0,
	}
	for _, item := range items {
		if item.Field != field || item.Cohort != cohort {
			continue
		}
		if value, ok := latest[item.ID]; ok {
			counts[value.Decision]++
		} else {
			counts["unreviewed"]++
		}
	}
	return counts
}

func printDecisionBreakdowns(output io.Writer, value evaluation, decision string, top int) {
	reasons := make(map[string]int)
	symbols := make(map[string]int)
	shapes := make(map[string]int)
	cohorts := make(map[string]int)
	total := 0
	for _, item := range value.Items {
		judgment, ok := value.LatestJudgments[item.ID]
		if !ok || judgment.Decision != decision {
			continue
		}
		total++
		reasons[judgment.Reason]++
		symbols[item.Output.Symbol]++
		shapes[item.Shape.Bucket]++
		cohorts[item.Field+"/"+item.Cohort]++
	}
	fmt.Fprintf(output, "\n%s breakdowns (all cohorts, n=%d):\n", titleCase(decision), total)
	printTopCounts(output, "reason", reasons, top)
	printTopCounts(output, "symbol", symbols, top)
	printTopCounts(output, "marker/identifier shape", shapes, top)
	printTopCounts(output, "cohort", cohorts, top)
}

type namedCount struct {
	Name  string
	Count int
}

func printTopCounts(output io.Writer, label string, values map[string]int, top int) {
	entries := make([]namedCount, 0, len(values))
	for name, count := range values {
		entries = append(entries, namedCount{Name: name, Count: count})
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Count != entries[right].Count {
			return entries[left].Count > entries[right].Count
		}
		return entries[left].Name < entries[right].Name
	})
	fmt.Fprintf(output, "  %s:", label)
	if len(entries) == 0 {
		fmt.Fprintln(output, " none")
		return
	}
	fmt.Fprintln(output)
	for index, entry := range entries {
		if index == top {
			break
		}
		fmt.Fprintf(output, "    %s: %d\n", entry.Name, entry.Count)
	}
}

func percentage(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return 100 * float64(numerator) / float64(denominator)
}

func titleCase(value string) string {
	if value == "" {
		return value
	}
	return string(value[0]-('a'-'A')) + value[1:]
}
