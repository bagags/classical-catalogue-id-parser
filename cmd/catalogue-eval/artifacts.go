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
	"strings"
	"time"

	catalogue "github.com/bagags/classical-catalogue-id-parser"
)

type evaluation struct {
	Manifest        manifest
	Items           []sampleItem
	ItemsByID       map[string]sampleItem
	LatestJudgments map[string]judgment
}

func loadEvaluation(directory string) (evaluation, error) {
	metadata, err := loadManifest(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return evaluation{}, err
	}
	items, itemsByID, err := loadSample(filepath.Join(directory, "sample.jsonl"), metadata)
	if err != nil {
		return evaluation{}, err
	}
	latest, err := loadJudgments(filepath.Join(directory, "judgments.jsonl"), itemsByID)
	if err != nil {
		return evaluation{}, err
	}
	return evaluation{Manifest: metadata, Items: items, ItemsByID: itemsByID, LatestJudgments: latest}, nil
}

func loadManifest(path string) (manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var value manifest
	if err := decodeStrictJSON(data, &value); err != nil {
		return manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if err := validateManifest(value); err != nil {
		return manifest{}, fmt.Errorf("validate manifest: %w", err)
	}
	return value, nil
}

func validateManifest(value manifest) error {
	if value.ArtifactVersion != artifactVersion {
		return fmt.Errorf("artifact_version = %d, want %d", value.ArtifactVersion, artifactVersion)
	}
	if _, err := time.Parse(time.RFC3339Nano, value.CreatedAt); err != nil {
		return fmt.Errorf("created_at is not RFC3339: %w", err)
	}
	if value.Source.Basename == "" || filepath.Base(value.Source.Basename) != value.Source.Basename {
		return fmt.Errorf("source basename is invalid")
	}
	if !validHexSHA256(value.Source.SHA256) {
		return fmt.Errorf("source sha256 is invalid")
	}
	if value.Registry.Schema < 1 || value.Registry.Revision < 1 {
		return fmt.Errorf("registry schema and revision must be positive")
	}
	registry := catalogue.Default()
	if value.Registry.Schema != registry.Schema() || value.Registry.Revision != registry.Revision() {
		return fmt.Errorf("registry schema/revision %d/%d does not match current %d/%d", value.Registry.Schema, value.Registry.Revision, registry.Schema(), registry.Revision())
	}
	if value.Sampling.RepresentativePerField < 0 || value.Sampling.DiagnosticPerField < 0 {
		return fmt.Errorf("sampling quotas must not be negative")
	}
	if len(value.Populations) != 2 {
		return fmt.Errorf("candidate populations must contain exactly number and title")
	}
	if len(value.Sample.ByField) != 2 {
		return fmt.Errorf("sample counts must contain exactly number and title")
	}
	if !validHexSHA256(value.Sample.SHA256) {
		return fmt.Errorf("sample sha256 is invalid")
	}
	if value.Sample.Items < 0 {
		return fmt.Errorf("sample item count must not be negative")
	}
	for _, field := range []string{fieldNumber, fieldTitle} {
		population, ok := value.Populations[field]
		if !ok {
			return fmt.Errorf("candidate population for %s is missing", field)
		}
		if population.Inputs < 0 || population.ZeroOutputInputs < 0 || population.MultiOutputInputs < 0 || population.Candidates < 0 {
			return fmt.Errorf("candidate population for %s contains a negative count", field)
		}
		if population.ZeroOutputInputs > population.Inputs || population.MultiOutputInputs > population.Inputs {
			return fmt.Errorf("candidate population for %s is inconsistent", field)
		}
		count, ok := value.Sample.ByField[field]
		if !ok {
			return fmt.Errorf("sample count for %s is missing", field)
		}
		if count.Representative < 0 || count.Diagnostic < 0 {
			return fmt.Errorf("sample count for %s is negative", field)
		}
		if count.Representative > value.Sampling.RepresentativePerField || count.Diagnostic > value.Sampling.DiagnosticPerField {
			return fmt.Errorf("sample count for %s exceeds its quota", field)
		}
		if count.Representative+count.Diagnostic > population.Candidates {
			return fmt.Errorf("sample count for %s exceeds its candidate population", field)
		}
	}
	return nil
}

func loadSample(path string, metadata manifest) ([]sampleItem, map[string]sampleItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read sample: %w", err)
	}
	digest := sha256.Sum256(data)
	if got := hex.EncodeToString(digest[:]); got != metadata.Sample.SHA256 {
		return nil, nil, fmt.Errorf("validate sample: sha256 = %s, want %s", got, metadata.Sample.SHA256)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), maximumJSONLLine)
	items := make([]sampleItem, 0, metadata.Sample.Items)
	byID := make(map[string]sampleItem, metadata.Sample.Items)
	counts := map[string]cohortCounts{fieldNumber: {}, fieldTitle: {}}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var item sampleItem
		if err := decodeStrictJSON(scanner.Bytes(), &item); err != nil {
			return nil, nil, fmt.Errorf("decode sample line %d: %w", lineNumber, err)
		}
		if err := validateSampleItem(item); err != nil {
			return nil, nil, fmt.Errorf("validate sample line %d: %w", lineNumber, err)
		}
		if _, exists := byID[item.ID]; exists {
			return nil, nil, fmt.Errorf("validate sample line %d: duplicate item ID %q", lineNumber, item.ID)
		}
		items = append(items, item)
		byID[item.ID] = item
		count := counts[item.Field]
		if item.Cohort == cohortRepresentative {
			count.Representative++
		} else {
			count.Diagnostic++
		}
		counts[item.Field] = count
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read sample after line %d: %w", lineNumber, err)
	}
	if len(items) != metadata.Sample.Items {
		return nil, nil, fmt.Errorf("validate sample: item count = %d, want %d", len(items), metadata.Sample.Items)
	}
	for _, field := range []string{fieldNumber, fieldTitle} {
		if counts[field] != metadata.Sample.ByField[field] {
			return nil, nil, fmt.Errorf("validate sample: %s cohort counts = %+v, want %+v", field, counts[field], metadata.Sample.ByField[field])
		}
	}
	return items, byID, nil
}

func validateSampleItem(item sampleItem) error {
	if item.ArtifactVersion != artifactVersion {
		return fmt.Errorf("artifact_version = %d, want %d", item.ArtifactVersion, artifactVersion)
	}
	if item.Field != fieldNumber && item.Field != fieldTitle {
		return fmt.Errorf("field %q is not allowed", item.Field)
	}
	if item.Cohort != cohortRepresentative && item.Cohort != cohortDiagnostic {
		return fmt.Errorf("cohort %q is not allowed", item.Cohort)
	}
	if item.RawText == "" || item.WorkID == "" || item.WorkTitle == "" {
		return fmt.Errorf("raw_text, work_id, and work_title are required")
	}
	if item.Output.Symbol == "" || item.Output.Identifier == "" {
		return fmt.Errorf("output symbol and identifier are required")
	}
	if item.OutputIndex < 0 {
		return fmt.Errorf("output_index must not be negative")
	}
	if item.Field == fieldNumber && item.SourceLine < 1 {
		return fmt.Errorf("number item source_line must be positive")
	}
	if item.Field == fieldTitle && item.SourceLine != 0 {
		return fmt.Errorf("title item source_line must be omitted")
	}
	if len(item.Contexts) == 0 {
		return fmt.Errorf("at least one catalogue relation is required")
	}
	for index, context := range item.Contexts {
		if strings.TrimSpace(context.SeriesID) == "" || strings.TrimSpace(context.SeriesName) == "" || strings.TrimSpace(context.Number) == "" {
			return fmt.Errorf("catalogue relation %d is missing a required field", index)
		}
	}
	if item.Shape.HasMarker != (item.Output.Marker != "") {
		return fmt.Errorf("diagnostic marker shape disagrees with output")
	}
	reference := catalogue.Reference{Symbol: item.Output.Symbol, Marker: item.Output.Marker, Identifier: item.Output.Identifier}
	shape := shapeFor(reference, item.Shape.MultiOutput)
	if item.Shape != shape {
		return fmt.Errorf("diagnostic shape is inconsistent")
	}
	if item.ID != candidateID(item) {
		return fmt.Errorf("item ID %q does not match its content", item.ID)
	}
	return nil
}

func loadJudgments(path string, items map[string]sampleItem) (map[string]judgment, error) {
	input, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open judgments: %w", err)
	}
	defer input.Close()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), maximumJSONLLine)
	latest := make(map[string]judgment)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			return nil, fmt.Errorf("decode judgments line %d: blank lines are not allowed", lineNumber)
		}
		var value judgment
		if err := decodeStrictJSON(scanner.Bytes(), &value); err != nil {
			return nil, fmt.Errorf("decode judgments line %d: %w", lineNumber, err)
		}
		if err := validateJudgment(value); err != nil {
			return nil, fmt.Errorf("validate judgments line %d: %w", lineNumber, err)
		}
		if _, ok := items[value.ItemID]; !ok {
			return nil, fmt.Errorf("validate judgments line %d: unknown item ID %q", lineNumber, value.ItemID)
		}
		latest[value.ItemID] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read judgments after line %d: %w", lineNumber, err)
	}
	return latest, nil
}

func appendJudgment(path string, value judgment) error {
	if err := validateJudgment(value); err != nil {
		return err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode judgment: %w", err)
	}
	encoded = append(encoded, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return fmt.Errorf("open judgments for append: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(encoded); err != nil {
		return fmt.Errorf("append judgment: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync judgment: %w", err)
	}
	return nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
