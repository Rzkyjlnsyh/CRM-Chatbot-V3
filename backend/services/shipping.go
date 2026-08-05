package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"wa-assistant/backend/config"
	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

// ShippingResult = satu hasil ongkir dari RajaOngkir V2.
type ShippingResult struct {
	Courier  string `json:"courier"`
	Service  string `json:"service"`
	Cost     int    `json:"cost"`
	Estimate string `json:"estimate"`
}

// CheckShippingCost memanggil RajaOngkir V2 untuk cek ongkir (POST form-encoded).
func CheckShippingCost(origin, destination int, weight int, couriers []string) ([]ShippingResult, error) {
	apiKey := config.Env("RAJAONGKIR_API_KEY", "")
	if apiKey == "" {
		return nil, fmt.Errorf("RAJAONGKIR_API_KEY belum diset")
	}

	form := url.Values{}
	form.Set("origin", fmt.Sprintf("%d", origin))
	form.Set("destination", fmt.Sprintf("%d", destination))
	form.Set("weight", fmt.Sprintf("%d", weight))
	form.Set("courier", strings.Join(couriers, ":"))
	form.Set("price", "lowest")

	reqURL := config.Env("RAJAONGKIR_BASE_URL", "https://rajaongkir.komerce.id/api/v1") + "/calculate/domestic-cost"
	httpReq, _ := http.NewRequest("POST", reqURL, strings.NewReader(form.Encode()))
	httpReq.Header.Set("key", apiKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gagal panggil RajaOngkir: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var apiResp struct {
		Meta struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"meta"`
		Data []struct {
			Name    string `json:"name"`
			Code    string `json:"code"`
			Service string `json:"service"`
			Cost    int    `json:"cost"`
			Etd     string `json:"etd"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("gagal parse respon: %w", err)
	}
	if apiResp.Meta.Code != 200 {
		return nil, fmt.Errorf("RajaOngkir error: %s", apiResp.Meta.Message)
	}

	var results []ShippingResult
	for _, r := range apiResp.Data {
		results = append(results, ShippingResult{
			Courier:  strings.ToUpper(r.Code),
			Service:  r.Service,
			Cost:     r.Cost,
			Estimate: r.Etd,
		})
	}
	return results, nil
}

// ResolveCity mencari kota dari teks di DB lokal.
func ResolveCity(query string) []models.ShippingCity {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return nil
	}
	var cities []models.ShippingCity
	database.DB.Where("search_text LIKE ?", "%"+query+"%").Order("city_name asc").Limit(5).Find(&cities)
	if len(cities) > 0 {
		return cities
	}
	// Fallback 1: coba per kata (mis. "jakarta utara" tetap menemukan "jakarta").
	words := strings.Fields(query)
	if len(words) > 1 {
		for _, w := range words {
			if len(w) < 3 {
				continue
			}
			database.DB.Where("search_text LIKE ?", "%"+w+"%").Order("city_name asc").Limit(5).Find(&cities)
			if len(cities) > 0 {
				return cities
			}
		}
	}
	// Fallback 2: coba empat karakter awal untuk toleransi typo ringan.
	runes := []rune(query)
	if len(runes) >= 4 {
		prefix := string(runes[:4])
		database.DB.Where("search_text LIKE ?", "%"+prefix+"%").Order("city_name asc").Limit(5).Find(&cities)
		if len(cities) > 0 {
			return cities
		}
	}
	// Fallback: search via API langsung (kalau DB belum di-seed)
	return SearchCityViaAPI(query)
}

// SearchCityViaAPI mencari kota langsung ke RajaOngkir V2.
func SearchCityViaAPI(query string) []models.ShippingCity {
	apiKey := config.Env("RAJAONGKIR_API_KEY", "")
	if apiKey == "" {
		return nil
	}
	baseURL := config.Env("RAJAONGKIR_BASE_URL", "https://rajaongkir.komerce.id/api/v1")
	reqURL := fmt.Sprintf("%s/destination/domestic-destination?search=%s&limit=5", baseURL, url.QueryEscape(query))
	httpReq, _ := http.NewRequest("GET", reqURL, nil)
	httpReq.Header.Set("key", apiKey)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var apiResp struct {
		Data []struct {
			ID       int    `json:"id"`
			Label    string `json:"label"`
			CityName string `json:"city_name"`
			Province string `json:"province_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil
	}
	var cities []models.ShippingCity
	for _, c := range apiResp.Data {
		cities = append(cities, models.ShippingCity{
			RajaOngkirID: c.ID,
			CityName:     c.CityName,
			Province:     c.Province,
			FullName:     c.Label,
			Type:         "",
		})
	}
	return cities
}

// SeedShippingCities = impor daftar kota dari RajaOngkir V2 (POST search).
func SeedShippingCities() {
	var count int64
	database.DB.Model(&models.ShippingCity{}).Count(&count)
	if count > 100 {
		return
	}
	apiKey := config.Env("RAJAONGKIR_API_KEY", "")
	if apiKey == "" {
		return
	}

	// Gunakan search dengan karakter umum untuk tarik semua kota.
	common := []string{"a", "i", "u", "e", "o", "k", "m", "n", "s", "p", "r", "t", "b", "d", "j", "l", "c"}
	baseURL := config.Env("RAJAONGKIR_BASE_URL", "https://rajaongkir.komerce.id/api/v1")
	client := &http.Client{Timeout: 15 * time.Second}
	seen := map[int]bool{}

	for _, ch := range common {
		if count > 1000 { // already enough
			break
		}
		reqURL := fmt.Sprintf("%s/destination/domestic-destination?search=%s&limit=200", baseURL, ch)
		httpReq, _ := http.NewRequest("GET", reqURL, nil)
		httpReq.Header.Set("key", apiKey)

		resp, err := client.Do(httpReq)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var apiResp struct {
			Data []struct {
				ID          int    `json:"id"`
				Label       string `json:"label"`
				CityName    string `json:"city_name"`
				Province    string `json:"province_name"`
				District    string `json:"district_name"`
				Subdistrict string `json:"subdistrict_name"`
				ZipCode     string `json:"zip_code"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &apiResp); err != nil {
			continue
		}

		for _, c := range apiResp.Data {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			cityType := "Kota"
			if c.Subdistrict != "" && c.Subdistrict != "-" {
				cityType = "Kecamatan"
			}
			fullName := c.Label
			searchText := strings.ToLower(c.CityName + " " + c.Province + " " + c.District)
			database.DB.Create(&models.ShippingCity{
				RajaOngkirID: c.ID,
				Province:     c.Province,
				Type:         cityType,
				CityName:     c.CityName,
				FullName:     fullName,
				SearchText:   searchText,
			})
			count++
		}
	}
}
