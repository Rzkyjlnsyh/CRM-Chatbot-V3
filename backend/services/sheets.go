package services

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// ParseSpreadsheetID mengambil ID spreadsheet dari URL Google Sheets.
// Input: "https://docs.google.com/spreadsheets/d/xxx/edit"
// Output: "xxx"
func ParseSpreadsheetID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(u.Path, "/")
	for i, p := range parts {
		if p == "d" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

var sheetsClient *sheets.Service

// InitSheets inisialisasi Google Sheets client dari service account JSON.
// Dipanggil saat startup jika GOOGLE_APPLICATION_CREDENTIALS diset.
func InitSheets() {
	credFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if credFile == "" {
		log.Println("[sheets] GOOGLE_APPLICATION_CREDENTIALS tidak diset, integrasi Google Sheets dinonaktifkan")
		return
	}
	ctx := context.Background()
	srv, err := sheets.NewService(ctx, option.WithCredentialsFile(credFile))
	if err != nil {
		log.Printf("[sheets] Gagal inisialisasi: %v", err)
		return
	}
	sheetsClient = srv
	log.Println("[sheets] Google Sheets client siap")
}

// AppendRow menambahkan satu baris data ke sheet & mengembalikan nomor baris (1-based) hasil append.
// spreadsheetID: ID dari URL, sheetName: nama tab, values: slice string per kolom.
func AppendRow(spreadsheetID, sheetName string, values []string) (int, error) {
	if sheetsClient == nil {
		return 0, fmt.Errorf("sheets client belum diinisialisasi")
	}
	range_ := sheetName + "!A:Z"
	resp, err := sheetsClient.Spreadsheets.Values.Append(spreadsheetID, range_, &sheets.ValueRange{
		Values: [][]interface{}{toInterfaceSlice(values)},
	}).ValueInputOption("RAW").Context(context.Background()).Do()
	if err != nil {
		return 0, err
	}
	row := 0
	if resp.Updates != nil {
		row = parseRowFromRange(resp.Updates.UpdatedRange)
	}
	return row, nil
}

// UpdateRow menimpa isi satu baris (1-based) di sheet — dipakai untuk memperbarui order yang berkembang.
func UpdateRow(spreadsheetID, sheetName string, row int, values []string) error {
	if sheetsClient == nil {
		return fmt.Errorf("sheets client belum diinisialisasi")
	}
	if row < 1 {
		return fmt.Errorf("nomor baris tidak valid: %d", row)
	}
	range_ := fmt.Sprintf("%s!A%d", sheetName, row)
	_, err := sheetsClient.Spreadsheets.Values.Update(spreadsheetID, range_, &sheets.ValueRange{
		Values: [][]interface{}{toInterfaceSlice(values)},
	}).ValueInputOption("RAW").Context(context.Background()).Do()
	return err
}

// parseRowFromRange mengambil nomor baris dari range A1 seperti "Leads!A5:H5" -> 5.
func parseRowFromRange(rng string) int {
	if i := strings.LastIndex(rng, "!"); i >= 0 {
		rng = rng[i+1:]
	}
	if i := strings.Index(rng, ":"); i >= 0 {
		rng = rng[:i] // "A5:H5" -> "A5"
	}
	num := 0
	for _, r := range rng {
		if r >= '0' && r <= '9' {
			num = num*10 + int(r-'0')
		}
	}
	return num
}

func toInterfaceSlice(vals []string) []interface{} {
	out := make([]interface{}, len(vals))
	for i, v := range vals {
		out[i] = v
	}
	return out
}

// TestConnection mencoba membaca 1 baris dari sheet untuk verifikasi akses.
func TestConnection(spreadsheetID, sheetName string) error {
	if sheetsClient == nil {
		return fmt.Errorf("sheets client belum diinisialisasi")
	}
	range_ := sheetName + "!A1:A1"
	_, err := sheetsClient.Spreadsheets.Values.Get(spreadsheetID, range_).Context(context.Background()).Do()
	return err
}

// GetSheetNames mengembalikan daftar nama tab/sheet dalam spreadsheet.
func GetSheetNames(spreadsheetID string) ([]string, error) {
	if sheetsClient == nil {
		return nil, fmt.Errorf("sheets client belum diinisialisasi")
	}
	ss, err := sheetsClient.Spreadsheets.Get(spreadsheetID).Context(context.Background()).Do()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ss.Sheets))
	for _, s := range ss.Sheets {
		if s.Properties != nil && s.Properties.Title != "" {
			names = append(names, s.Properties.Title)
		}
	}
	return names, nil
}
