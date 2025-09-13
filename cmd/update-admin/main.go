package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Database connection
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	adminTelegramID := os.Getenv("ADMIN_TELEGRAM_ID")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Update admin user with telegram ID
	query := `UPDATE admin_users
			  SET telegram_id = $1, receive_notifications = true
			  WHERE id = 1`

	result, err := db.Exec(query, adminTelegramID)
	if err != nil {
		log.Fatal("Failed to update admin user:", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Create admin user if not exists
		insertQuery := `INSERT INTO admin_users (id, username, password, telegram_id, receive_notifications, is_active)
						VALUES (1, 'admin', '', $1, true, true)
						ON CONFLICT (id) DO UPDATE
						SET telegram_id = $1, receive_notifications = true`

		_, err = db.Exec(insertQuery, adminTelegramID)
		if err != nil {
			log.Fatal("Failed to insert admin user:", err)
		}
		fmt.Println("Admin user created with Telegram ID:", adminTelegramID)
	} else {
		fmt.Println("Admin user updated with Telegram ID:", adminTelegramID)
	}

	// Check the result
	var telegramID sql.NullInt64
	checkQuery := `SELECT telegram_id FROM admin_users WHERE id = 1`
	err = db.QueryRow(checkQuery).Scan(&telegramID)
	if err != nil {
		log.Fatal("Failed to check admin user:", err)
	}

	if telegramID.Valid {
		fmt.Printf("Admin user telegram_id is now: %d\n", telegramID.Int64)
	} else {
		fmt.Println("Admin user telegram_id is NULL")
	}
}