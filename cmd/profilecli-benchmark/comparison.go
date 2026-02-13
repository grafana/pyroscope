package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func uploadComparisonToSheets(config *Config, results []TestResult) error {
	ctx := context.Background()

	gsConfig := config.GoogleSheets

	// Create Google Sheets service
	srv, err := sheets.NewService(ctx, option.WithCredentialsFile(gsConfig.Credentials))
	if err != nil {
		return fmt.Errorf("unable to create sheets client: %w", err)
	}

	// Get baseline flag name (first flag in config)
	baselineFlagName := ""
	if len(config.Tests.Flags) > 0 {
		baselineFlagName = config.Tests.Flags[0].Name
	}

	// Calculate comparison data
	type configKey struct {
		QueryName string
		MaxNodes  int64
		FlagName  string
	}

	type compareKey struct {
		QueryName string
		MaxNodes  int64
	}

	// Group results by config
	grouped := make(map[configKey][]time.Duration)
	outputSizes := make(map[configKey][]int64)
	numSamples := make(map[configKey][]int64)
	totalValues := make(map[configKey][]int64)
	sampleTypeUnits := make(map[configKey]string)

	for _, r := range results {
		if !r.Success {
			continue
		}

		key := configKey{
			QueryName: r.QueryName,
			MaxNodes:  r.MaxNodes,
			FlagName:  r.FlagName,
		}
		grouped[key] = append(grouped[key], r.Duration)
		outputSizes[key] = append(outputSizes[key], r.OutputSize)
		numSamples[key] = append(numSamples[key], r.NumSamples)
		totalValues[key] = append(totalValues[key], r.TotalValue)
		if sampleTypeUnits[key] == "" {
			sampleTypeUnits[key] = r.SampleTypeUnit
		}
	}

	// Calculate medians and prepare comparison data
	comparisons := make(map[compareKey]map[string]time.Duration) // compareKey -> flagName -> medianDuration

	for key, durations := range grouped {
		if len(durations) == 0 {
			continue
		}

		medianDuration := median(durations)

		cmpKey := compareKey{
			QueryName: key.QueryName,
			MaxNodes:  key.MaxNodes,
		}

		if comparisons[cmpKey] == nil {
			comparisons[cmpKey] = make(map[string]time.Duration)
		}
		comparisons[cmpKey][key.FlagName] = medianDuration
	}

	// Prepare header row
	header := []interface{}{
		"Timestamp",
		"Query Name",
		"Max Nodes",
		"Flag Config",
		"Median Duration (ms)",
		"Median Duration (seconds)",
		"Relative Performance (%)",
		"Status",
		"Avg Output Size",
		"Avg Samples",
		"Avg Total Value",
		"Sample Type:Unit",
	}

	// Prepare data rows
	var rows [][]interface{}
	rows = append(rows, header)

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// Sort for consistent ordering
	var cmpKeys []compareKey
	for k := range comparisons {
		cmpKeys = append(cmpKeys, k)
	}
	sort.Slice(cmpKeys, func(i, j int) bool {
		if cmpKeys[i].QueryName != cmpKeys[j].QueryName {
			return cmpKeys[i].QueryName < cmpKeys[j].QueryName
		}
		return cmpKeys[i].MaxNodes < cmpKeys[j].MaxNodes
	})

	for _, cmpKey := range cmpKeys {
		flagDurations := comparisons[cmpKey]

		// Use first flag as baseline
		baselineDuration := flagDurations[baselineFlagName]
		if baselineDuration == 0 {
			// Baseline flag not found for this config, skip
			continue
		}

		// Use flag order from config
		for _, flagConfig := range config.Tests.Flags {
			flagName := flagConfig.Name
			duration, exists := flagDurations[flagName]
			if !exists {
				continue
			}

			// Get averages for other metrics
			key := configKey{
				QueryName: cmpKey.QueryName,
				MaxNodes:  cmpKey.MaxNodes,
				FlagName:  flagName,
			}

			var avgSize int64
			var avgSamples int64
			var avgTotal int64

			if len(outputSizes[key]) > 0 {
				var totalSize int64
				var totalSamp int64
				var totalVal int64
				for i := range outputSizes[key] {
					totalSize += outputSizes[key][i]
					totalSamp += numSamples[key][i]
					totalVal += totalValues[key][i]
				}
				avgSize = totalSize / int64(len(outputSizes[key]))
				avgSamples = totalSamp / int64(len(numSamples[key]))
				avgTotal = totalVal / int64(len(totalValues[key]))
			}

			// Calculate relative performance
			ratio := float64(duration) / float64(baselineDuration)
			relativePercent := ratio * 100
			var status string

			if flagName == baselineFlagName {
				status = "Baseline"
			} else if ratio > 1.0 {
				status = fmt.Sprintf("%.1f%% slower", (ratio-1.0)*100)
			} else {
				status = fmt.Sprintf("%.1f%% faster", (1.0-ratio)*100)
			}

			row := []interface{}{
				timestamp,
				cmpKey.QueryName,
				cmpKey.MaxNodes,
				flagName,
				duration.Milliseconds(),
				fmt.Sprintf("%.3f", duration.Seconds()),
				fmt.Sprintf("%.1f", relativePercent),
				status,
				formatBytes(avgSize),
				avgSamples,
				avgTotal,
				sampleTypeUnits[key],
			}
			rows = append(rows, row)
		}
	}

	// Get or create comparison sheet
	sheetName := gsConfig.SheetName + " - Comparison"
	if gsConfig.SheetName == "" {
		sheetName = "ProfileCLI Benchmark Results - Comparison"
	}

	spreadsheet, err := srv.Spreadsheets.Get(gsConfig.SpreadsheetID).Do()
	if err != nil {
		return fmt.Errorf("unable to get spreadsheet: %w", err)
	}

	var sheetID int64
	sheetExists := false

	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == sheetName {
			sheetID = sheet.Properties.SheetId
			sheetExists = true
			break
		}
	}

	// Create sheet if it doesn't exist
	if !sheetExists {
		req := &sheets.Request{
			AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{
					Title: sheetName,
				},
			},
		}

		batchUpdateRequest := &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{req},
		}

		resp, err := srv.Spreadsheets.BatchUpdate(gsConfig.SpreadsheetID, batchUpdateRequest).Do()
		if err != nil {
			return fmt.Errorf("unable to create comparison sheet: %w", err)
		}
		sheetID = resp.Replies[0].AddSheet.Properties.SheetId
	}

	// Append data
	valueRange := &sheets.ValueRange{
		Values: rows,
	}

	rangeStr := fmt.Sprintf("%s!A1", sheetName)
	_, err = srv.Spreadsheets.Values.Append(gsConfig.SpreadsheetID, rangeStr, valueRange).
		ValueInputOption("RAW").
		InsertDataOption("INSERT_ROWS").
		Do()

	if err != nil {
		return fmt.Errorf("unable to append comparison data: %w", err)
	}

	// Format header row and add conditional formatting
	requests := []*sheets.Request{
		// Format header row
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:       sheetID,
					StartRowIndex: 0,
					EndRowIndex:   1,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold: true,
						},
						BackgroundColor: &sheets.Color{
							Red:   0.9,
							Green: 0.9,
							Blue:  0.9,
						},
					},
				},
				Fields: "userEnteredFormat(textFormat,backgroundColor)",
			},
		},
		// Add conditional formatting for "Status" column (column H, index 7)
		// Green for "faster"
		{
			AddConditionalFormatRule: &sheets.AddConditionalFormatRuleRequest{
				Rule: &sheets.ConditionalFormatRule{
					Ranges: []*sheets.GridRange{
						{
							SheetId:          sheetID,
							StartRowIndex:    1, // Skip header
							StartColumnIndex: 7, // Status column
							EndColumnIndex:   8,
						},
					},
					BooleanRule: &sheets.BooleanRule{
						Condition: &sheets.BooleanCondition{
							Type: "TEXT_CONTAINS",
							Values: []*sheets.ConditionValue{
								{UserEnteredValue: "faster"},
							},
						},
						Format: &sheets.CellFormat{
							BackgroundColor: &sheets.Color{
								Red:   0.85,
								Green: 0.95,
								Blue:  0.85,
							},
							TextFormat: &sheets.TextFormat{
								ForegroundColor: &sheets.Color{
									Red:   0.0,
									Green: 0.5,
									Blue:  0.0,
								},
								Bold: true,
							},
						},
					},
				},
				Index: 0,
			},
		},
		// Red for "slower"
		{
			AddConditionalFormatRule: &sheets.AddConditionalFormatRuleRequest{
				Rule: &sheets.ConditionalFormatRule{
					Ranges: []*sheets.GridRange{
						{
							SheetId:          sheetID,
							StartRowIndex:    1,
							StartColumnIndex: 7, // Status column
							EndColumnIndex:   8,
						},
					},
					BooleanRule: &sheets.BooleanRule{
						Condition: &sheets.BooleanCondition{
							Type: "TEXT_CONTAINS",
							Values: []*sheets.ConditionValue{
								{UserEnteredValue: "slower"},
							},
						},
						Format: &sheets.CellFormat{
							BackgroundColor: &sheets.Color{
								Red:   0.95,
								Green: 0.85,
								Blue:  0.85,
							},
							TextFormat: &sheets.TextFormat{
								ForegroundColor: &sheets.Color{
									Red:   0.8,
									Green: 0.0,
									Blue:  0.0,
								},
								Bold: true,
							},
						},
					},
				},
				Index: 1,
			},
		},
		// Cyan/Blue for "Baseline"
		{
			AddConditionalFormatRule: &sheets.AddConditionalFormatRuleRequest{
				Rule: &sheets.ConditionalFormatRule{
					Ranges: []*sheets.GridRange{
						{
							SheetId:          sheetID,
							StartRowIndex:    1,
							StartColumnIndex: 7, // Status column
							EndColumnIndex:   8,
						},
					},
					BooleanRule: &sheets.BooleanRule{
						Condition: &sheets.BooleanCondition{
							Type: "TEXT_CONTAINS",
							Values: []*sheets.ConditionValue{
								{UserEnteredValue: "Baseline"},
							},
						},
						Format: &sheets.CellFormat{
							BackgroundColor: &sheets.Color{
								Red:   0.85,
								Green: 0.92,
								Blue:  0.98,
							},
							TextFormat: &sheets.TextFormat{
								ForegroundColor: &sheets.Color{
									Red:   0.0,
									Green: 0.4,
									Blue:  0.7,
								},
								Bold: true,
							},
						},
					},
				},
				Index: 2,
			},
		},
	}

	batchUpdateRequest := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}

	_, err = srv.Spreadsheets.BatchUpdate(gsConfig.SpreadsheetID, batchUpdateRequest).Do()
	if err != nil {
		log.Printf("Warning: failed to format comparison sheet: %v", err)
	}

	return nil
}
