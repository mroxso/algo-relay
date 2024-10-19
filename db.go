package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func getDBConnection() (*sql.DB, error) {
	loadEnv()

	dbUser := os.Getenv("POSTGRES_USER")
	dbPassword := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")
	dbPort := os.Getenv("POSTGRES_PORT")
	dbHost := os.Getenv("POSTGRES_HOST")

	if dbUser == "" || dbPassword == "" || dbName == "" || dbPort == "" || dbHost == "" {
		return nil, fmt.Errorf("missing required environment variables")
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable", dbHost, dbUser, dbPassword, dbName, dbPort)

	// Open the database connection
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %v", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(50)                  // Maximum number of open connections to the database
	db.SetMaxIdleConns(25)                  // Maximum number of idle connections in the pool
	db.SetConnMaxLifetime(time.Minute * 30) // Maximum lifetime of a connection

	// Ping the database to ensure the connection is successful
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("error connecting to the db: %v", err)
	}

	initDB(db) // Call your database initialization logic

	return db, nil
}

func initDB(db *sql.DB) error {
	var exists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'posts')").Scan(&exists)
	if err != nil {
		return fmt.Errorf("error checking if posts table exists: %v", err)
	}

	if exists {
		return nil
	}

	log.Println("Migration not applied, running migration")

	migrationSQL, err := os.ReadFile("sql/init.sql")
	if err != nil {
		return fmt.Errorf("error reading migration file: %v", err)
	}

	_, err = db.Exec(string(migrationSQL))
	if err != nil {
		return fmt.Errorf("error applying migration: %v", err)
	}

	log.Println("Migration applied successfully")
	return nil
}
