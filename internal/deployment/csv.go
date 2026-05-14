// Package deployment implements deployment orchestration: CSV parsing, execution, and result logging.
package deployment

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/hellowynd/wyndctl/internal/models"
)

// LoadSettings reads deployment settings from a CSV file.
// Empty cells inherit from the previous row (except wifi_psk which may be empty).
func LoadSettings(path string) ([]models.DeploymentSetting, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening settings file: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("settings file must have a header row and at least one data row")
	}

	header := records[0]
	colIndex := make(map[string]int)
	for i, col := range header {
		colIndex[col] = i
	}

	requiredCols := []string{"bay", "wifi_ssid", "wifi_psk", "account", "lodging_id", "room", "room_type"}
	for _, col := range requiredCols {
		if _, ok := colIndex[col]; !ok {
			return nil, fmt.Errorf("missing required column %q in settings CSV", col)
		}
	}

	var settings []models.DeploymentSetting
	for rowNum, record := range records[1:] {
		if len(record) == 0 || (len(record) == 1 && record[0] == "") {
			continue
		}
		allEmpty := true
		for _, field := range record {
			if field != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			continue
		}

		s, err := parseSettingRow(record, colIndex, settings, rowNum+1)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNum+2, err)
		}
		settings = append(settings, *s)
	}

	return settings, nil
}

func parseSettingRow(
	record []string,
	colIndex map[string]int,
	previous []models.DeploymentSetting,
	rowNum int,
) (*models.DeploymentSetting, error) {
	get := func(col string) string {
		idx, ok := colIndex[col]
		if !ok || idx >= len(record) {
			return ""
		}
		return record[idx]
	}

	inherit := func(col, val string) string {
		if val != "" {
			return val
		}
		if len(previous) > 0 {
			switch col {
			case "wifi_ssid":
				return previous[len(previous)-1].WiFiSSID
			case "account":
				return previous[len(previous)-1].Account
			case "lodging_id":
				return strconv.Itoa(previous[len(previous)-1].LodgingID)
			case "room_type":
				return previous[len(previous)-1].RoomType
			}
		}
		return val
	}

	bayStr := get("bay")
	bay, err := strconv.Atoi(bayStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bay number %q: %w", bayStr, err)
	}

	lodgingIDStr := inherit("lodging_id", get("lodging_id"))
	lodgingID := 0
	if lodgingIDStr != "" {
		lodgingID, err = strconv.Atoi(lodgingIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid lodging_id %q: %w", lodgingIDStr, err)
		}
	}

	s := &models.DeploymentSetting{
		Bay:       bay,
		WiFiSSID:  inherit("wifi_ssid", get("wifi_ssid")),
		WiFiPSK:   get("wifi_psk"),
		Account:   inherit("account", get("account")),
		LodgingID: lodgingID,
		Room:      get("room"),
		RoomType:  inherit("room_type", get("room_type")),
	}

	if rowNum == 1 {
		if err := s.Validate(); err != nil {
			return nil, err
		}
	}

	return s, nil
}

// LoadLocationMap reads the bay-to-location CSV mapping.
func LoadLocationMap(path string, locationToBay bool) (map[string]int, map[int]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("opening location map: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("reading location map CSV: %w", err)
	}

	if len(records) < 2 {
		return nil, nil, fmt.Errorf("location map must have a header and at least one row")
	}

	header := records[0]
	colIndex := make(map[string]int)
	for i, col := range header {
		colIndex[col] = i
	}

	locToBay := make(map[string]int)
	bayToLoc := make(map[int]string)

	for _, record := range records[1:] {
		bayStr := record[colIndex["bay"]]
		bay, err := strconv.Atoi(bayStr)
		if err != nil {
			continue
		}
		loc := record[colIndex["location"]]
		locToBay[loc] = bay
		bayToLoc[bay] = loc
	}

	return locToBay, bayToLoc, nil
}

// WriteResultHeader creates the output CSV file with headers.
func WriteResultHeader(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating result file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	return w.Write([]string{"bay", "device_id", "mac_addr", "succeeded", "room_id", "room_name", "reason"})
}

// AppendResult writes a single deployment result row.
func AppendResult(path string, result *models.DeploymentResult) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening result file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	return w.Write([]string{
		strconv.Itoa(result.Bay),
		result.DeviceID,
		result.MACAddr,
		strconv.FormatBool(result.Succeeded),
		strconv.Itoa(result.RoomID),
		result.RoomName,
		result.Reason,
	})
}

// ReadResults loads all results from a previous deployment run.
func ReadResults(path string) (map[int]*models.DeploymentResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening result file: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading result CSV: %w", err)
	}

	results := make(map[int]*models.DeploymentResult)
	for _, record := range records[1:] {
		if len(record) < 7 {
			continue
		}
		bay, _ := strconv.Atoi(record[0])
		roomID, _ := strconv.Atoi(record[4])
		results[bay] = &models.DeploymentResult{
			Bay:       bay,
			DeviceID:  record[1],
			MACAddr:   record[2],
			Succeeded: record[3] == "true",
			RoomID:    roomID,
			RoomName:  record[5],
			Reason:    record[6],
		}
	}

	return results, nil
}
