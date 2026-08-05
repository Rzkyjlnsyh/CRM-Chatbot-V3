package main

import (
	"flag"
	"log"

	"wa-assistant/backend/config"
	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"golang.org/x/crypto/bcrypt"
)

// Seeder akun login. Jalankan dengan:
//
//	go run ./backend/cmd/seed
//
// Atau dengan kredensial kustom:
//
//	go run ./backend/cmd/seed -username budi -password rahasia123 -name "Budi"
//
// Bila username sudah ada, password-nya akan di-reset (upsert),
// sehingga kamu selalu bisa login.
func main() {
	username := flag.String("username", "admin", "username untuk login")
	password := flag.String("password", "admin123", "password untuk login")
	name := flag.String("name", "Admin", "nama lengkap")
	email := flag.String("email", "admin@wa-assistant.local", "email")
	flag.Parse()

	config.Load()
	database.Init()

	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal("Gagal hash password:", err)
	}

	var user models.User
	err = database.DB.Where("username = ?", *username).First(&user).Error
	if err != nil {
		// Belum ada: buat baru.
		user = models.User{
			Name:     *name,
			Username: *username,
			Email:    *email,
			Password: string(hash),
		}
		if err := database.DB.Create(&user).Error; err != nil {
			log.Fatal("Gagal membuat user:", err)
		}
		log.Printf("User dibuat: %s / %s", *username, *password)
		return
	}

	// Sudah ada: reset password (dan perbarui nama/email).
	user.Name = *name
	user.Email = *email
	user.Password = string(hash)
	if err := database.DB.Save(&user).Error; err != nil {
		log.Fatal("Gagal memperbarui user:", err)
	}
	log.Printf("Password user di-reset: %s / %s", *username, *password)
}
