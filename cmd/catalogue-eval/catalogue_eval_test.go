package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	catalogue "github.com/bagags/classical-catalogue-id-parser"
)

func TestBuildCandidatesDeduplicatesTitlesAndKeepsRelations(t *testing.T) {
	path := writeSource(t, []map[string]any{
		{
			"series_id": "series-b", "series_name": "B catalogue", "work_id": "work-1",
			"work_title": "Suite BWV 1 and K 2", "number": "BWV 1 and K 2", "ignored": true,
		},
		{
			"series_id": "series-a", "series_name": "A catalogue", "series_disambiguation": "early",
			"work_id": "work-1", "work_title": "Suite BWV 1 and K 2", "number": "Hob. XVI:52",
		},
		{
			"series_id": "series-c", "series_name": "C catalogue", "work_id": "work-2",
			"work_title": "A title without a reference", "number": "42",
		},
	})

	result, err := buildCandidates(path)
	if err != nil {
		t.Fatal(err)
	}
	numbers := result.Populations[fieldNumber]
	if numbers != (populationStats{Inputs: 3, ZeroOutputInputs: 1, MultiOutputInputs: 1, Candidates: 3}) {
		t.Fatalf("number population = %+v", numbers)
	}
	titles := result.Populations[fieldTitle]
	if titles != (populationStats{Inputs: 2, ZeroOutputInputs: 1, MultiOutputInputs: 1, Candidates: 2}) {
		t.Fatalf("title population = %+v", titles)
	}
	if len(result.ByField[fieldNumber]) != 3 || len(result.ByField[fieldTitle]) != 2 {
		t.Fatalf("candidate counts = number %d title %d", len(result.ByField[fieldNumber]), len(result.ByField[fieldTitle]))
	}
	for _, item := range result.ByField[fieldTitle] {
		if item.WorkID != "work-1" || len(item.Contexts) != 2 {
			t.Fatalf("title candidate context = %+v", item)
		}
		if item.Contexts[0].SeriesID != "series-a" || item.Contexts[1].SeriesID != "series-b" {
			t.Fatalf("title contexts are not stably sorted: %+v", item.Contexts)
		}
		if !item.Shape.MultiOutput {
			t.Fatal("title multi-output shape was not retained")
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256(data)
	if result.SourceHash != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("source hash = %s", result.SourceHash)
	}
}

func TestBuildCandidatesRejectsMalformedAndConflictingRows(t *testing.T) {
	valid := map[string]any{
		"series_id": "series-1", "series_name": "Catalogue", "work_id": "work-1",
		"work_title": "Work BWV 1", "number": "BWV 1",
	}
	tests := map[string]struct {
		lines []string
		want  string
	}{
		"malformed JSON": {lines: []string{"{"}, want: "decode JSON"},
		"blank line":     {lines: []string{""}, want: "blank lines"},
		"missing field":  {lines: []string{marshalLine(t, withoutKey(valid, "number"))}, want: "required field number"},
		"wrong type":     {lines: []string{marshalLine(t, withValue(valid, "number", 12))}, want: "cannot unmarshal number"},
		"conflicting title": {
			lines: []string{marshalLine(t, valid), marshalLine(t, withValue(valid, "work_title", "Different title"))},
			want:  "conflicting with",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.jsonl")
			if err := os.WriteFile(path, []byte(strings.Join(test.lines, "\n")+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := buildCandidates(path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestSamplingIsDeterministicAndCohortsDoNotOverlap(t *testing.T) {
	candidates := make([]sampleItem, 30)
	for index := range candidates {
		symbol := "BWV"
		if index%5 == 0 {
			symbol = "K"
		}
		candidates[index] = syntheticItem(fieldNumber, index+1, symbol, "1", index%4 == 0)
	}
	first := selectSample(candidates, 7, 5, 99)
	second := selectSample(candidates, 7, 5, 99)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same seed produced different samples")
	}
	if len(first) != 12 {
		t.Fatalf("sample length = %d", len(first))
	}
	seen := make(map[string]string)
	for index, item := range first {
		wantCohort := cohortRepresentative
		if index >= 7 {
			wantCohort = cohortDiagnostic
		}
		if item.Cohort != wantCohort {
			t.Fatalf("item %d cohort = %q", index, item.Cohort)
		}
		if previous, exists := seen[item.ID]; exists {
			t.Fatalf("item %s occurs in both %s and %s", item.ID, previous, item.Cohort)
		}
		seen[item.ID] = item.Cohort
	}
	third := selectSample(candidates, 7, 5, 100)
	if reflect.DeepEqual(itemIDs(first[:7]), itemIDs(third[:7])) {
		t.Fatal("different seeds unexpectedly produced the same representative sample")
	}

	exhausted := selectSample(candidates[:4], 3, 3, 1)
	if len(exhausted) != 4 || countSelected(exhausted, fieldNumber) != (cohortCounts{Representative: 3, Diagnostic: 1}) {
		t.Fatalf("exhausted sample = %+v", exhausted)
	}
}

func TestDiagnosticsCycleAcrossRareSymbols(t *testing.T) {
	var candidates []sampleItem
	line := 1
	for _, countAndSymbol := range []struct {
		count  int
		symbol string
	}{{10, "A"}, {2, "B"}, {1, "C"}} {
		for range countAndSymbol.count {
			candidates = append(candidates, syntheticItem(fieldNumber, line, countAndSymbol.symbol, "1", false))
			line++
		}
	}
	selected := selectDiagnostics(candidates, 3, 1)
	if got := []string{selected[0].Output.Symbol, selected[1].Output.Symbol, selected[2].Output.Symbol}; !reflect.DeepEqual(got, []string{"C", "B", "A"}) {
		t.Fatalf("diagnostic symbol order = %v", got)
	}
}

func TestReviewResumesAndCorrectionAppendsReplacement(t *testing.T) {
	directory, items := writeTestEvaluation(t, 2)
	first := judgment{
		ArtifactVersion: artifactVersion, ItemID: items[0].ID,
		Timestamp: "2026-08-04T00:00:00Z", Decision: decisionInvalid, Reason: "identifier",
	}
	if err := appendJudgment(filepath.Join(directory, "judgments.jsonl"), first); err != nil {
		t.Fatal(err)
	}
	fixed := func() time.Time { return time.Date(2026, 8, 4, 1, 2, 3, 4, time.UTC) }
	var output bytes.Buffer
	if err := reviewEvaluation(directory, "", strings.NewReader("valid\n"), &output, fixed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), items[1].ID) || strings.Contains(output.String(), "Raw text: "+items[0].RawText) {
		t.Fatalf("resume output did not select the unreviewed item:\n%s", output.String())
	}

	output.Reset()
	prefix := items[0].ID[:len(fieldNumber)+1+10]
	if err := reviewEvaluation(directory, prefix, strings.NewReader("valid\n"), &output, fixed); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadEvaluation(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.LatestJudgments[items[0].ID].Decision; got != decisionValid {
		t.Fatalf("latest replacement decision = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(directory, "judgments.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := bytes.Count(data, []byte("\n")); lines != 3 {
		t.Fatalf("judgment line count = %d, want 3", lines)
	}
}

func TestReviewIDPrefixMustBeUnambiguous(t *testing.T) {
	directory, items := writeTestEvaluation(t, 2)
	value, err := loadEvaluation(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewItems(value, "number-"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous prefix error = %v", err)
	}
	if matches, err := reviewItems(value, items[0].ID); err != nil || len(matches) != 1 {
		t.Fatalf("exact ID matches = %d, error = %v", len(matches), err)
	}
}

func TestShowReviewItemDisplaysMatchSpan(t *testing.T) {
	references := catalogue.Parse("Suite BWV 1 and K 2")
	if len(references) != 2 {
		t.Fatalf("parse fixture = %#v", references)
	}
	item := newSampleItem(fieldTitle, "Suite BWV 1 and K 2", references, 1, 0, "work-1", "Suite BWV 1 and K 2",
		[]relationContext{{SeriesID: "series-1", SeriesName: "K catalogue", Number: "K 2"}})
	var output bytes.Buffer
	showReviewItem(&output, item, 1, 1)
	if !strings.Contains(output.String(), `Match: "K 2" (bytes [16, 19))`) {
		t.Fatalf("review item output missing match span:\n%s", output.String())
	}
}

func TestJudgmentReasonValidation(t *testing.T) {
	base := judgment{ArtifactVersion: artifactVersion, ItemID: "number-item", Timestamp: "2026-08-04T00:00:00Z"}
	tests := []struct {
		name     string
		decision string
		reason   string
		note     string
		valid    bool
	}{
		{name: "valid", decision: decisionValid, valid: true},
		{name: "invalid reason", decision: decisionInvalid, reason: "symbol", valid: true},
		{name: "invalid missing", decision: decisionInvalid},
		{name: "invalid wrong set", decision: decisionInvalid, reason: "context"},
		{name: "uncertain reason", decision: decisionUncertain, reason: "context", valid: true},
		{name: "uncertain wrong set", decision: decisionUncertain, reason: "symbol"},
		{name: "other missing note", decision: decisionInvalid, reason: "other"},
		{name: "other note", decision: decisionUncertain, reason: "other", note: "needs a specialist", valid: true},
		{name: "skip", decision: decisionSkip, valid: true},
		{name: "valid with reason", decision: decisionValid, reason: "symbol"},
		{name: "unknown decision", decision: "maybe"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			value.Decision, value.Reason, value.Note = test.decision, test.reason, test.note
			err := validateJudgment(value)
			if (err == nil) != test.valid {
				t.Fatalf("validateJudgment() error = %v, valid = %v", err, test.valid)
			}
		})
	}
}

func TestWilsonBoundsAndDiagnosticExclusion(t *testing.T) {
	lower, upper := wilsonInterval(5, 10)
	if math.Abs(lower-0.23659309051256405) > 1e-12 || math.Abs(upper-0.763406909487436) > 1e-12 {
		t.Fatalf("Wilson interval = %.15f–%.15f", lower, upper)
	}
	items := []sampleItem{
		{ID: "valid", Field: fieldNumber, Cohort: cohortRepresentative},
		{ID: "invalid", Field: fieldNumber, Cohort: cohortRepresentative},
		{ID: "uncertain", Field: fieldNumber, Cohort: cohortRepresentative},
		{ID: "unreviewed", Field: fieldNumber, Cohort: cohortRepresentative},
		{ID: "skip", Field: fieldNumber, Cohort: cohortRepresentative},
		{ID: "diagnostic-invalid", Field: fieldNumber, Cohort: cohortDiagnostic},
	}
	latest := map[string]judgment{
		"valid":              {Decision: decisionValid},
		"invalid":            {Decision: decisionInvalid},
		"uncertain":          {Decision: decisionUncertain},
		"skip":               {Decision: decisionSkip},
		"diagnostic-invalid": {Decision: decisionInvalid},
	}
	metrics := calculateFieldMetrics(items, latest, fieldNumber)
	if metrics.Valid != 1 || metrics.Invalid != 1 || metrics.RepresentativeTotal != 5 || metrics.Precision != 0.5 {
		t.Fatalf("representative metrics = %+v", metrics)
	}
	if metrics.BoundLower != 0.25 || metrics.BoundUpper != 0.75 {
		t.Fatalf("outcome bounds = %.2f–%.2f", metrics.BoundLower, metrics.BoundUpper)
	}
}

func TestArtifactValidationRejectsUnknownJudgmentIDAndValue(t *testing.T) {
	directory, items := writeTestEvaluation(t, 1)
	path := filepath.Join(directory, "judgments.jsonl")
	bad := judgment{ArtifactVersion: artifactVersion, ItemID: "number-unknown", Timestamp: "2026-08-04T00:00:00Z", Decision: decisionValid}
	encoded, _ := json.Marshal(bad)
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadEvaluation(directory); err == nil || !strings.Contains(err.Error(), "unknown item ID") {
		t.Fatalf("unknown ID error = %v", err)
	}
	bad.ItemID = items[0].ID
	bad.Decision = "maybe"
	encoded, _ = json.Marshal(bad)
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadEvaluation(directory); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("invalid decision error = %v", err)
	}
}

func TestWriteEvaluationRefusesOverwrite(t *testing.T) {
	directory, _ := writeTestEvaluation(t, 1)
	marker := filepath.Join(directory, "judgments.jsonl")
	before, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	candidates := candidateSet{SourceHash: strings.Repeat("a", 64), Populations: map[string]populationStats{fieldNumber: {}, fieldTitle: {}}}
	err = writeEvaluation(directory, "input.jsonl", candidates, nil, 0, 0, 1, time.Now())
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("overwrite error = %v", err)
	}
	after, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("overwrite attempt changed judgments")
	}
}

func TestCLIExitCodes(t *testing.T) {
	var output, errorOutput bytes.Buffer
	if code := run(nil, strings.NewReader(""), &output, &errorOutput, time.Now); code != 2 {
		t.Fatalf("missing command exit code = %d", code)
	}
	output.Reset()
	errorOutput.Reset()
	if code := run([]string{"summary", "-not-a-flag"}, strings.NewReader(""), &output, &errorOutput, time.Now); code != 2 {
		t.Fatalf("invalid flag exit code = %d", code)
	}
	output.Reset()
	errorOutput.Reset()
	if code := run([]string{"help"}, strings.NewReader(""), &output, &errorOutput, time.Now); code != 0 {
		t.Fatalf("help exit code = %d", code)
	}
	output.Reset()
	errorOutput.Reset()
	if code := run([]string{"summary", "-dir", filepath.Join(t.TempDir(), "missing")}, strings.NewReader(""), &output, &errorOutput, time.Now); code != 1 {
		t.Fatalf("data failure exit code = %d", code)
	}
	directory, _ := writeTestEvaluation(t, 1)
	output.Reset()
	errorOutput.Reset()
	if code := run([]string{"review", "-dir", directory}, strings.NewReader("quit\n"), &output, &errorOutput, time.Now); code != 0 {
		t.Fatalf("intentional quit exit code = %d", code)
	}
}

func writeSource(t *testing.T, rows []map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "catalogue-references.jsonl")
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	for _, row := range rows {
		if err := encoder.Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func marshalLine(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func withoutKey(source map[string]any, key string) map[string]any {
	result := make(map[string]any, len(source))
	for name, value := range source {
		if name != key {
			result[name] = value
		}
	}
	return result
}

func withValue(source map[string]any, key string, value any) map[string]any {
	result := withoutKey(source, key)
	result[key] = value
	return result
}

func syntheticItem(field string, line int, symbol, identifier string, marker bool) sampleItem {
	reference := catalogue.Reference{Symbol: symbol, Identifier: identifier}
	if marker {
		reference.Marker = "anh"
		reference.Identifier = "anh" + identifier
	}
	context := relationContext{SeriesID: "series-" + symbol, SeriesName: symbol + " catalogue", Number: symbol + " " + identifier}
	references := []catalogue.Reference{reference}
	item := newSampleItem(field, context.Number, references, 0, line, "work-"+string(rune(line+64)), "Work "+identifier, []relationContext{context})
	if field == fieldTitle {
		item.SourceLine = 0
		item.ID = candidateID(item)
	}
	return item
}

func itemIDs(items []sampleItem) []string {
	ids := make([]string, len(items))
	for index, item := range items {
		ids[index] = item.ID
	}
	return ids
}

func writeTestEvaluation(t *testing.T, count int) (string, []sampleItem) {
	t.Helper()
	items := make([]sampleItem, count)
	for index := range items {
		items[index] = syntheticItem(fieldNumber, index+1, "BWV", string(rune('1'+index)), false)
		items[index].Cohort = cohortRepresentative
	}
	directory := filepath.Join(t.TempDir(), "evaluation")
	candidates := candidateSet{
		SourceHash: strings.Repeat("a", 64),
		Populations: map[string]populationStats{
			fieldNumber: {Inputs: count, Candidates: count},
			fieldTitle:  {},
		},
	}
	if err := writeEvaluation(directory, "catalogue-references.jsonl", candidates, items, count, 0, 1, time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	return directory, items
}
