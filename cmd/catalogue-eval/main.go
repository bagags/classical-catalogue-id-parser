// Command catalogue-eval creates and reviews a local precision evaluation for
// catalogue references emitted from MusicBrainz relation numbers and titles.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

type usageError struct {
	message string
}

func (err usageError) Error() string { return err.message }

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, time.Now))
}

func run(args []string, input io.Reader, output, errorOutput io.Writer, now func() time.Time) int {
	if len(args) == 0 {
		printRootUsage(errorOutput)
		return 2
	}
	var err error
	switch args[0] {
	case "sample":
		err = runSample(args[1:], output, errorOutput, now)
	case "review":
		err = runReview(args[1:], input, output, errorOutput, now)
	case "summary":
		err = runSummary(args[1:], output, errorOutput)
	case "help", "-h", "--help":
		printRootUsage(output)
		return 0
	default:
		fmt.Fprintf(errorOutput, "unknown command %q\n", args[0])
		printRootUsage(errorOutput)
		return 2
	}
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}
	var invalidUsage usageError
	if errors.As(err, &invalidUsage) {
		fmt.Fprintln(errorOutput, invalidUsage.Error())
		return 2
	}
	fmt.Fprintln(errorOutput, "catalogue-eval:", err)
	return 1
}

func runSample(args []string, output, errorOutput io.Writer, now func() time.Time) error {
	flags := flag.NewFlagSet("sample", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	inputPath := flags.String("input", "", "MusicBrainz catalogue-references JSONL path (required)")
	outPath := flags.String("out", ".catalogue-eval", "new local evaluation directory")
	representativeQuota := flags.Int("representative-per-field", 75, "representative outputs per field")
	diagnosticQuota := flags.Int("diagnostic-per-field", 25, "diagnostic outputs per field")
	seed := flags.Int64("seed", 1, "deterministic sampling seed")
	flags.Usage = func() {
		fmt.Fprintln(errorOutput, "usage: catalogue-eval sample -input PATH [-out .catalogue-eval] [-representative-per-field 75] [-diagnostic-per-field 25] [-seed 1]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usageError{message: "sample: " + err.Error()}
	}
	if flags.NArg() != 0 {
		return usageError{message: "sample: unexpected positional arguments"}
	}
	if *inputPath == "" {
		return usageError{message: "sample: -input PATH is required"}
	}
	if *outPath == "" {
		return usageError{message: "sample: -out must not be empty"}
	}
	if *representativeQuota < 0 || *diagnosticQuota < 0 {
		return usageError{message: "sample: sampling quotas must not be negative"}
	}
	if err := requireOutputAbsent(*outPath); err != nil {
		return err
	}

	candidates, err := buildCandidates(*inputPath)
	if err != nil {
		return err
	}
	selected := make([]sampleItem, 0, 2*(*representativeQuota+*diagnosticQuota))
	for _, field := range []string{fieldNumber, fieldTitle} {
		selected = append(selected, selectSample(candidates.ByField[field], *representativeQuota, *diagnosticQuota, *seed)...)
	}
	if err := writeEvaluation(*outPath, *inputPath, candidates, selected, *representativeQuota, *diagnosticQuota, *seed, now()); err != nil {
		return err
	}
	numberCounts := countSelected(selected, fieldNumber)
	titleCounts := countSelected(selected, fieldTitle)
	fmt.Fprintf(output, "Read %d catalogue relations and %d unique work titles.\n",
		candidates.Populations[fieldNumber].Inputs, candidates.Populations[fieldTitle].Inputs)
	fmt.Fprintf(output, "Candidate outputs: number=%d title=%d.\n",
		candidates.Populations[fieldNumber].Candidates, candidates.Populations[fieldTitle].Candidates)
	fmt.Fprintf(output, "Wrote %d review items to %s (number %d+%d, title %d+%d representative+diagnostic).\n",
		len(selected), *outPath,
		numberCounts.Representative, numberCounts.Diagnostic,
		titleCounts.Representative, titleCounts.Diagnostic)
	return nil
}

func countSelected(items []sampleItem, field string) cohortCounts {
	var result cohortCounts
	for _, item := range items {
		if item.Field != field {
			continue
		}
		if item.Cohort == cohortRepresentative {
			result.Representative++
		} else if item.Cohort == cohortDiagnostic {
			result.Diagnostic++
		}
	}
	return result
}

func runReview(args []string, input io.Reader, output, errorOutput io.Writer, now func() time.Time) error {
	flags := flag.NewFlagSet("review", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	directory := flags.String("dir", ".catalogue-eval", "evaluation directory")
	idPrefix := flags.String("id", "", "unambiguous item ID prefix to review or correct")
	flags.Usage = func() {
		fmt.Fprintln(errorOutput, "usage: catalogue-eval review [-dir .catalogue-eval] [-id ID_PREFIX]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usageError{message: "review: " + err.Error()}
	}
	if flags.NArg() != 0 {
		return usageError{message: "review: unexpected positional arguments"}
	}
	if *directory == "" {
		return usageError{message: "review: -dir must not be empty"}
	}
	return reviewEvaluation(*directory, *idPrefix, input, output, now)
}

func runSummary(args []string, output, errorOutput io.Writer) error {
	flags := flag.NewFlagSet("summary", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	directory := flags.String("dir", ".catalogue-eval", "evaluation directory")
	top := flags.Int("top", 10, "maximum rows per breakdown")
	flags.Usage = func() {
		fmt.Fprintln(errorOutput, "usage: catalogue-eval summary [-dir .catalogue-eval] [-top 10]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usageError{message: "summary: " + err.Error()}
	}
	if flags.NArg() != 0 {
		return usageError{message: "summary: unexpected positional arguments"}
	}
	if *directory == "" {
		return usageError{message: "summary: -dir must not be empty"}
	}
	if *top < 1 {
		return usageError{message: "summary: -top must be positive"}
	}
	value, err := loadEvaluation(*directory)
	if err != nil {
		return err
	}
	printSummary(output, value, *top)
	return nil
}

func printRootUsage(output io.Writer) {
	fmt.Fprintln(output, "usage: catalogue-eval <sample|review|summary> [options]")
	fmt.Fprintln(output, "run 'catalogue-eval <command> -h' for command options")
}
