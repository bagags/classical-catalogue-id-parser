package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	catalogue "github.com/bagags/classical-catalogue-id-parser"
)

const maximumJSONLLine = 16 * 1024 * 1024

type candidateSet struct {
	ByField     map[string][]sampleItem
	Populations map[string]populationStats
	SourceHash  string
}

type accumulatedWork struct {
	Title     string
	FirstLine int
	Contexts  []relationContext
}

func requireOutputAbsent(outPath string) error {
	if _, err := os.Lstat(outPath); err == nil {
		return fmt.Errorf("output %q already exists; refusing to overwrite judgments", outPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect output %q: %w", outPath, err)
	}
	return nil
}

func buildCandidates(inputPath string) (candidateSet, error) {
	input, err := os.Open(inputPath)
	if err != nil {
		return candidateSet{}, fmt.Errorf("open input: %w", err)
	}
	defer input.Close()

	hasher := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(input, hasher))
	scanner.Buffer(make([]byte, 64*1024), maximumJSONLLine)

	result := candidateSet{
		ByField: map[string][]sampleItem{
			fieldNumber: nil,
			fieldTitle:  nil,
		},
		Populations: map[string]populationStats{
			fieldNumber: {},
			fieldTitle:  {},
		},
	}
	works := make(map[string]*accumulatedWork)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			return candidateSet{}, fmt.Errorf("input line %d: blank lines are not allowed", lineNumber)
		}
		var row sourceRow
		if err := json.Unmarshal(line, &row); err != nil {
			return candidateSet{}, fmt.Errorf("input line %d: decode JSON: %w", lineNumber, err)
		}
		if err := validateSourceRow(row); err != nil {
			return candidateSet{}, fmt.Errorf("input line %d: %w", lineNumber, err)
		}

		context := relationContext{
			SeriesID: row.SeriesID, SeriesName: row.SeriesName,
			SeriesDisambiguation: row.SeriesDisambiguation, Number: row.Number,
		}
		work, exists := works[row.WorkID]
		if !exists {
			work = &accumulatedWork{Title: row.WorkTitle, FirstLine: lineNumber}
			works[row.WorkID] = work
		} else if work.Title != row.WorkTitle {
			return candidateSet{}, fmt.Errorf("input line %d: work_id %q has work_title %q, conflicting with %q on line %d", lineNumber, row.WorkID, row.WorkTitle, work.Title, work.FirstLine)
		}
		work.Contexts = append(work.Contexts, context)

		references := catalogue.Parse(row.Number)
		population := result.Populations[fieldNumber]
		population.Inputs++
		switch len(references) {
		case 0:
			population.ZeroOutputInputs++
		case 1:
		default:
			population.MultiOutputInputs++
		}
		population.Candidates += len(references)
		result.Populations[fieldNumber] = population
		for index := range references {
			result.ByField[fieldNumber] = append(result.ByField[fieldNumber], newSampleItem(
				fieldNumber, row.Number, references, index, lineNumber,
				row.WorkID, row.WorkTitle, []relationContext{context},
			))
		}
	}
	if err := scanner.Err(); err != nil {
		return candidateSet{}, fmt.Errorf("read input after line %d: %w", lineNumber, err)
	}
	if lineNumber == 0 {
		return candidateSet{}, fmt.Errorf("input contains no rows")
	}
	result.SourceHash = hex.EncodeToString(hasher.Sum(nil))

	workIDs := make([]string, 0, len(works))
	for workID := range works {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	for _, workID := range workIDs {
		work := works[workID]
		sort.SliceStable(work.Contexts, func(left, right int) bool {
			return contextKey(work.Contexts[left]) < contextKey(work.Contexts[right])
		})
		references := catalogue.Parse(work.Title)
		population := result.Populations[fieldTitle]
		population.Inputs++
		switch len(references) {
		case 0:
			population.ZeroOutputInputs++
		case 1:
		default:
			population.MultiOutputInputs++
		}
		population.Candidates += len(references)
		result.Populations[fieldTitle] = population
		for index := range references {
			result.ByField[fieldTitle] = append(result.ByField[fieldTitle], newSampleItem(
				fieldTitle, work.Title, references, index, 0,
				workID, work.Title, work.Contexts,
			))
		}
	}
	return result, nil
}

func validateSourceRow(row sourceRow) error {
	required := []struct {
		name  string
		value string
	}{
		{name: "series_id", value: row.SeriesID},
		{name: "series_name", value: row.SeriesName},
		{name: "work_id", value: row.WorkID},
		{name: "work_title", value: row.WorkTitle},
		{name: "number", value: row.Number},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("required field %s must be a non-empty string", field.name)
		}
	}
	return nil
}

func contextKey(context relationContext) string {
	return context.SeriesID + "\x00" + context.SeriesName + "\x00" + context.SeriesDisambiguation + "\x00" + context.Number
}

func selectSample(candidates []sampleItem, representativeQuota, diagnosticQuota int, seed int64) []sampleItem {
	if len(candidates) == 0 {
		return nil
	}
	ranked := append([]sampleItem(nil), candidates...)
	sort.Slice(ranked, func(left, right int) bool {
		leftRank := sampleRank(seed, "representative", ranked[left].ID)
		rightRank := sampleRank(seed, "representative", ranked[right].ID)
		if leftRank == rightRank {
			return ranked[left].ID < ranked[right].ID
		}
		return leftRank < rightRank
	})
	representativeCount := min(representativeQuota, len(ranked))
	selected := make([]sampleItem, 0, min(len(ranked), representativeQuota+diagnosticQuota))
	for _, candidate := range ranked[:representativeCount] {
		candidate.Cohort = cohortRepresentative
		selected = append(selected, candidate)
	}
	remaining := ranked[representativeCount:]
	for _, candidate := range selectDiagnostics(remaining, diagnosticQuota, seed) {
		candidate.Cohort = cohortDiagnostic
		selected = append(selected, candidate)
	}
	return selected
}

func sampleRank(seed int64, purpose, id string) string {
	digest := sha256.Sum256([]byte(strconv.FormatInt(seed, 10) + "\x00" + purpose + "\x00" + id))
	return string(digest[:])
}

type diagnosticSymbolQueue struct {
	Symbol     string
	Candidates []sampleItem
	Next       int
}

func selectDiagnostics(candidates []sampleItem, quota int, seed int64) []sampleItem {
	if quota <= 0 || len(candidates) == 0 {
		return nil
	}
	bySymbol := make(map[string][]sampleItem)
	for _, candidate := range candidates {
		bySymbol[candidate.Output.Symbol] = append(bySymbol[candidate.Output.Symbol], candidate)
	}
	queues := make([]diagnosticSymbolQueue, 0, len(bySymbol))
	for symbol, symbolCandidates := range bySymbol {
		bucketCounts := make(map[string]int)
		for _, candidate := range symbolCandidates {
			bucketCounts[candidate.Shape.Bucket]++
		}
		sort.Slice(symbolCandidates, func(left, right int) bool {
			leftCount := bucketCounts[symbolCandidates[left].Shape.Bucket]
			rightCount := bucketCounts[symbolCandidates[right].Shape.Bucket]
			if leftCount != rightCount {
				return leftCount < rightCount
			}
			if symbolCandidates[left].Shape.Bucket != symbolCandidates[right].Shape.Bucket {
				return symbolCandidates[left].Shape.Bucket < symbolCandidates[right].Shape.Bucket
			}
			leftRank := sampleRank(seed, "diagnostic", symbolCandidates[left].ID)
			rightRank := sampleRank(seed, "diagnostic", symbolCandidates[right].ID)
			if leftRank == rightRank {
				return symbolCandidates[left].ID < symbolCandidates[right].ID
			}
			return leftRank < rightRank
		})
		queues = append(queues, diagnosticSymbolQueue{Symbol: symbol, Candidates: symbolCandidates})
	}
	sort.Slice(queues, func(left, right int) bool {
		if len(queues[left].Candidates) != len(queues[right].Candidates) {
			return len(queues[left].Candidates) < len(queues[right].Candidates)
		}
		return queues[left].Symbol < queues[right].Symbol
	})

	wanted := min(quota, len(candidates))
	selected := make([]sampleItem, 0, wanted)
	for len(selected) < wanted {
		advanced := false
		for index := range queues {
			queue := &queues[index]
			if queue.Next >= len(queue.Candidates) {
				continue
			}
			selected = append(selected, queue.Candidates[queue.Next])
			queue.Next++
			advanced = true
			if len(selected) == wanted {
				break
			}
		}
		if !advanced {
			break
		}
	}
	return selected
}

func writeEvaluation(outPath, inputPath string, candidates candidateSet, selected []sampleItem, representativeQuota, diagnosticQuota int, seed int64, now time.Time) error {
	if err := requireOutputAbsent(outPath); err != nil {
		return err
	}
	seenIDs := make(map[string]bool, len(selected))
	for _, item := range selected {
		if err := validateSampleItem(item); err != nil {
			return fmt.Errorf("validate selected item %q: %w", item.ID, err)
		}
		if seenIDs[item.ID] {
			return fmt.Errorf("validate selected items: duplicate ID %q", item.ID)
		}
		seenIDs[item.ID] = true
	}
	parent := filepath.Dir(outPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	temporary, err := os.MkdirTemp(parent, "."+filepath.Base(outPath)+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary evaluation directory: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(temporary)
		}
	}()

	sampleBytes, err := encodeJSONLines(selected)
	if err != nil {
		return err
	}
	sampleHash := sha256.Sum256(sampleBytes)
	counts := map[string]cohortCounts{
		fieldNumber: {},
		fieldTitle:  {},
	}
	for _, item := range selected {
		count := counts[item.Field]
		if item.Cohort == cohortRepresentative {
			count.Representative++
		} else {
			count.Diagnostic++
		}
		counts[item.Field] = count
	}
	registry := catalogue.Default()
	metadata := manifest{
		ArtifactVersion: artifactVersion,
		CreatedAt:       now.UTC().Format(time.RFC3339Nano),
		Source: sourceMetadata{
			Basename: filepath.Base(inputPath),
			SHA256:   candidates.SourceHash,
		},
		Registry: registryMetadata{Schema: registry.Schema(), Revision: registry.Revision()},
		Sampling: samplingMetadata{
			RepresentativePerField: representativeQuota,
			DiagnosticPerField:     diagnosticQuota,
			Seed:                   seed,
		},
		Populations: candidates.Populations,
		Sample: sampleMetadata{
			SHA256: hex.EncodeToString(sampleHash[:]), Items: len(selected), ByField: counts,
		},
	}
	manifestBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.WriteFile(filepath.Join(temporary, "manifest.json"), manifestBytes, 0o600); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temporary, "sample.jsonl"), sampleBytes, 0o600); err != nil {
		return fmt.Errorf("write sample: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temporary, "judgments.jsonl"), nil, 0o600); err != nil {
		return fmt.Errorf("write judgments: %w", err)
	}
	if _, err := os.Lstat(outPath); err == nil {
		return fmt.Errorf("output %q appeared while sampling; refusing to overwrite it", outPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect output before commit: %w", err)
	}
	if err := os.Rename(temporary, outPath); err != nil {
		return fmt.Errorf("commit evaluation directory: %w", err)
	}
	committed = true
	return nil
}

func encodeJSONLines(values []sampleItem) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	for _, value := range values {
		if err := encoder.Encode(value); err != nil {
			return nil, fmt.Errorf("encode sample item %q: %w", value.ID, err)
		}
	}
	return buffer.Bytes(), nil
}
