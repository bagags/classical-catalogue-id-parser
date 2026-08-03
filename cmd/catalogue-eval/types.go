package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	catalogue "github.com/bagags/classical-catalogue-id-parser"
)

const artifactVersion = 1

const (
	fieldNumber = "number"
	fieldTitle  = "title"

	cohortRepresentative = "representative"
	cohortDiagnostic     = "diagnostic"

	decisionValid     = "valid"
	decisionInvalid   = "invalid"
	decisionUncertain = "uncertain"
	decisionSkip      = "skip"
)

var invalidReasons = map[string]bool{
	"false-positive": true,
	"symbol":         true,
	"marker":         true,
	"identifier":     true,
	"duplicate":      true,
	"other":          true,
}

var uncertainReasons = map[string]bool{
	"ambiguity": true,
	"context":   true,
	"expertise": true,
	"other":     true,
}

type sourceRow struct {
	SeriesID             string `json:"series_id"`
	SeriesName           string `json:"series_name"`
	SeriesDisambiguation string `json:"series_disambiguation,omitempty"`
	WorkID               string `json:"work_id"`
	WorkTitle            string `json:"work_title"`
	Number               string `json:"number"`
}

type relationContext struct {
	SeriesID             string `json:"series_id"`
	SeriesName           string `json:"series_name"`
	SeriesDisambiguation string `json:"series_disambiguation,omitempty"`
	Number               string `json:"number"`
}

type parsedReference struct {
	Symbol     string `json:"symbol"`
	Marker     string `json:"marker,omitempty"`
	Identifier string `json:"identifier"`
}

type diagnosticShape struct {
	HasMarker   bool   `json:"has_marker"`
	Connectors  string `json:"identifier_connectors"`
	MultiOutput bool   `json:"multi_output_input"`
	Bucket      string `json:"bucket"`
}

type sampleItem struct {
	ArtifactVersion int               `json:"artifact_version"`
	ID              string            `json:"id"`
	Field           string            `json:"field"`
	Cohort          string            `json:"cohort"`
	RawText         string            `json:"raw_text"`
	Output          parsedReference   `json:"output"`
	OutputIndex     int               `json:"output_index"`
	SourceLine      int               `json:"source_line,omitempty"`
	WorkID          string            `json:"work_id"`
	WorkTitle       string            `json:"work_title"`
	Contexts        []relationContext `json:"catalogue_relations"`
	Shape           diagnosticShape   `json:"diagnostic_shape"`
}

type manifest struct {
	ArtifactVersion int                        `json:"artifact_version"`
	CreatedAt       string                     `json:"created_at"`
	Source          sourceMetadata             `json:"source"`
	Registry        registryMetadata           `json:"registry"`
	Sampling        samplingMetadata           `json:"sampling"`
	Populations     map[string]populationStats `json:"candidate_populations"`
	Sample          sampleMetadata             `json:"sample"`
}

type sourceMetadata struct {
	Basename string `json:"basename"`
	SHA256   string `json:"sha256"`
}

type registryMetadata struct {
	Schema   int `json:"schema"`
	Revision int `json:"revision"`
}

type samplingMetadata struct {
	RepresentativePerField int   `json:"representative_per_field"`
	DiagnosticPerField     int   `json:"diagnostic_per_field"`
	Seed                   int64 `json:"seed"`
}

type populationStats struct {
	Inputs            int `json:"inputs"`
	ZeroOutputInputs  int `json:"zero_output_inputs"`
	MultiOutputInputs int `json:"multi_output_inputs"`
	Candidates        int `json:"candidates"`
}

type cohortCounts struct {
	Representative int `json:"representative"`
	Diagnostic     int `json:"diagnostic"`
}

type sampleMetadata struct {
	SHA256  string                  `json:"sha256"`
	Items   int                     `json:"items"`
	ByField map[string]cohortCounts `json:"by_field"`
}

type judgment struct {
	ArtifactVersion int    `json:"artifact_version"`
	ItemID          string `json:"item_id"`
	Timestamp       string `json:"timestamp"`
	Decision        string `json:"decision"`
	Reason          string `json:"reason,omitempty"`
	Note            string `json:"note,omitempty"`
}

func newSampleItem(field, raw string, references []catalogue.Reference, outputIndex int, sourceLine int, workID, workTitle string, contexts []relationContext) sampleItem {
	reference := references[outputIndex]
	item := sampleItem{
		ArtifactVersion: artifactVersion,
		Field:           field,
		RawText:         raw,
		Output:          parsedReference{Symbol: reference.Symbol, Marker: reference.Marker, Identifier: reference.Identifier},
		OutputIndex:     outputIndex,
		SourceLine:      sourceLine,
		WorkID:          workID,
		WorkTitle:       workTitle,
		Contexts:        append([]relationContext(nil), contexts...),
	}
	item.Shape = shapeFor(reference, len(references) > 1)
	item.ID = candidateID(item)
	return item
}

func shapeFor(reference catalogue.Reference, multiOutput bool) diagnosticShape {
	core := strings.TrimPrefix(reference.Identifier, reference.Marker)
	var connectors strings.Builder
	for _, candidate := range "-/:," {
		if strings.ContainsRune(core, candidate) {
			connectors.WriteRune(candidate)
		}
	}
	connectorShape := connectors.String()
	if connectorShape == "" {
		connectorShape = "none"
	}
	markerShape := "no-marker"
	if reference.Marker != "" {
		markerShape = "marker"
	}
	outputShape := "single-output"
	if multiOutput {
		outputShape = "multi-output"
	}
	return diagnosticShape{
		HasMarker:   reference.Marker != "",
		Connectors:  connectorShape,
		MultiOutput: multiOutput,
		Bucket:      markerShape + "|connectors=" + connectorShape + "|" + outputShape,
	}
}

func candidateID(item sampleItem) string {
	type identity struct {
		Field       string            `json:"field"`
		RawText     string            `json:"raw_text"`
		Output      parsedReference   `json:"output"`
		OutputIndex int               `json:"output_index"`
		SourceLine  int               `json:"source_line,omitempty"`
		WorkID      string            `json:"work_id"`
		Contexts    []relationContext `json:"catalogue_relations"`
	}
	encoded, err := json.Marshal(identity{
		Field: item.Field, RawText: item.RawText, Output: item.Output,
		OutputIndex: item.OutputIndex, SourceLine: item.SourceLine,
		WorkID: item.WorkID, Contexts: item.Contexts,
	})
	if err != nil {
		panic(fmt.Sprintf("marshal candidate identity: %v", err))
	}
	digest := sha256.Sum256(encoded)
	return item.Field + "-" + hex.EncodeToString(digest[:])
}

func validateJudgment(value judgment) error {
	if value.ArtifactVersion != artifactVersion {
		return fmt.Errorf("artifact_version = %d, want %d", value.ArtifactVersion, artifactVersion)
	}
	if value.ItemID == "" {
		return fmt.Errorf("item_id is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, value.Timestamp); err != nil {
		return fmt.Errorf("timestamp is not RFC3339: %w", err)
	}
	switch value.Decision {
	case decisionValid, decisionSkip:
		if value.Reason != "" {
			return fmt.Errorf("%s judgment must not have a reason", value.Decision)
		}
	case decisionInvalid:
		if !invalidReasons[value.Reason] {
			return fmt.Errorf("invalid judgment reason %q is not allowed", value.Reason)
		}
	case decisionUncertain:
		if !uncertainReasons[value.Reason] {
			return fmt.Errorf("uncertain judgment reason %q is not allowed", value.Reason)
		}
	default:
		return fmt.Errorf("decision %q is not allowed", value.Decision)
	}
	if value.Reason == "other" && strings.TrimSpace(value.Note) == "" {
		return fmt.Errorf("reason other requires a note")
	}
	return nil
}

func validHexSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
