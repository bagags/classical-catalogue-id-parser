package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	catalogue "github.com/bagags/classical-catalogue-id-parser"
)

func reviewEvaluation(directory, idPrefix string, input io.Reader, output io.Writer, now func() time.Time) error {
	evaluation, err := loadEvaluation(directory)
	if err != nil {
		return err
	}
	items, err := reviewItems(evaluation, idPrefix)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(output, "No unreviewed items remain.")
		return nil
	}

	reader := bufio.NewReader(input)
	for index, item := range items {
		showReviewItem(output, item, index+1, len(items))
		decision, reason, note, quit, err := promptJudgment(reader, output)
		if err != nil {
			return err
		}
		if quit {
			fmt.Fprintln(output, "Review stopped; all prior judgments are saved.")
			return nil
		}
		value := judgment{
			ArtifactVersion: artifactVersion,
			ItemID:          item.ID,
			Timestamp:       now().UTC().Format(time.RFC3339Nano),
			Decision:        decision,
			Reason:          reason,
			Note:            note,
		}
		if err := appendJudgment(filepath.Join(directory, "judgments.jsonl"), value); err != nil {
			return err
		}
		fmt.Fprintf(output, "Saved %s for %s.\n", decision, item.ID)
		if idPrefix != "" {
			return nil
		}
	}
	return nil
}

func reviewItems(value evaluation, idPrefix string) ([]sampleItem, error) {
	if idPrefix == "" {
		items := make([]sampleItem, 0)
		for _, item := range value.Items {
			if _, reviewed := value.LatestJudgments[item.ID]; !reviewed {
				items = append(items, item)
			}
		}
		return items, nil
	}
	matches := make([]sampleItem, 0, 1)
	for _, item := range value.Items {
		if strings.HasPrefix(item.ID, idPrefix) {
			matches = append(matches, item)
		}
	}
	if len(matches) == 0 {
		return nil, usageError{message: fmt.Sprintf("review: ID prefix %q does not match a sample item", idPrefix)}
	}
	if len(matches) > 1 {
		ids := make([]string, len(matches))
		for index, item := range matches {
			ids[index] = item.ID
		}
		sort.Strings(ids)
		return nil, usageError{message: fmt.Sprintf("review: ID prefix %q is ambiguous (%s)", idPrefix, strings.Join(ids, ", "))}
	}
	return matches, nil
}

func showReviewItem(output io.Writer, item sampleItem, index, total int) {
	fmt.Fprintf(output, "\n[%d/%d] %s  field=%s cohort=%s\n", index, total, item.ID, item.Field, item.Cohort)
	fmt.Fprintf(output, "Raw text: %s\n", item.RawText)
	if matches := catalogue.ParseMatches(item.RawText); item.OutputIndex < len(matches) {
		match := matches[item.OutputIndex]
		fmt.Fprintf(output, "Match: %q (bytes [%d, %d))\n", item.RawText[match.Start:match.End], match.Start, match.End)
	}
	fmt.Fprintf(output, "Parsed: symbol=%q marker=%q identifier=%q\n", item.Output.Symbol, item.Output.Marker, item.Output.Identifier)
	fmt.Fprintf(output, "Work: %s\n", item.WorkTitle)
	fmt.Fprintf(output, "Work MBID: %s\n", item.WorkID)
	fmt.Fprintln(output, "Catalogue relations:")
	for _, context := range item.Contexts {
		disambiguation := ""
		if context.SeriesDisambiguation != "" {
			disambiguation = " (" + context.SeriesDisambiguation + ")"
		}
		fmt.Fprintf(output, "  - %s%s | series MBID %s | number %s\n", context.SeriesName, disambiguation, context.SeriesID, context.Number)
	}
}

func promptJudgment(reader *bufio.Reader, output io.Writer) (decision, reason, note string, quit bool, err error) {
	for {
		answer, eof, readErr := promptLine(reader, output, "Decision [valid/invalid/uncertain/skip/quit]: ")
		if readErr != nil {
			return "", "", "", false, readErr
		}
		if eof && answer == "" {
			return "", "", "", true, nil
		}
		decision = strings.ToLower(strings.TrimSpace(answer))
		switch decision {
		case "quit", "q":
			return "", "", "", true, nil
		case decisionValid, decisionSkip:
			return decision, "", "", false, nil
		case decisionInvalid:
			reason, note, quit, err = promptReason(reader, output, invalidReasons, "false-positive/symbol/marker/identifier/duplicate/other")
			if err != nil || quit {
				return "", "", "", quit, err
			}
			return decision, reason, note, false, nil
		case decisionUncertain:
			reason, note, quit, err = promptReason(reader, output, uncertainReasons, "ambiguity/context/expertise/other")
			if err != nil || quit {
				return "", "", "", quit, err
			}
			return decision, reason, note, false, nil
		default:
			fmt.Fprintln(output, "Enter valid, invalid, uncertain, skip, or quit.")
		}
	}
}

func promptReason(reader *bufio.Reader, output io.Writer, allowed map[string]bool, choices string) (reason, note string, quit bool, err error) {
	for {
		answer, eof, readErr := promptLine(reader, output, "Reason ["+choices+"]: ")
		if readErr != nil {
			return "", "", false, readErr
		}
		if eof && answer == "" {
			return "", "", true, nil
		}
		reason = strings.ToLower(strings.TrimSpace(answer))
		if reason == "quit" || reason == "q" {
			return "", "", true, nil
		}
		if !allowed[reason] {
			fmt.Fprintf(output, "Choose one of %s.\n", choices)
			continue
		}
		if reason != "other" {
			return reason, "", false, nil
		}
		for {
			answer, eof, readErr = promptLine(reader, output, "Note (required for other): ")
			if readErr != nil {
				return "", "", false, readErr
			}
			if eof && answer == "" {
				return "", "", true, nil
			}
			note = strings.TrimSpace(answer)
			if note != "" {
				return reason, note, false, nil
			}
			fmt.Fprintln(output, "A note is required when the reason is other.")
		}
	}
}

func promptLine(reader *bufio.Reader, output io.Writer, prompt string) (string, bool, error) {
	fmt.Fprint(output, prompt)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", false, fmt.Errorf("read review input: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), errors.Is(err, io.EOF), nil
}
