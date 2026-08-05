package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// init memuat .env SEKALI, SEBELUM variabel package-level lain (mis. jwtSecret) diinisialisasi.
// Karena package config diimpor paling awal, ini cukup — tak perlu godotenv.Load() lagi di main().
func init() {
	_ = godotenv.Load(".env")
}

// Load disediakan untuk command/seed lama yang masih memanggil config.Load().
// init() tetap menjadi mekanisme utama agar env tersedia sebelum package-level var lain dibuat.
func Load() {
	_ = godotenv.Load(".env")
}

func Env(key, defaultVal string) string {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	return v
}

func EnvRequired(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("ERROR: %s harus diset di .env", key)
	}
	return v
}

func EnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

func EnvBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	switch v {
	case "1", "true", "TRUE", "yes", "on":
		return true
	case "0", "false", "FALSE", "no", "off":
		return false
	default:
		return defaultVal
	}
}
