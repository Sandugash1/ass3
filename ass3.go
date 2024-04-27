package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type Product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       int    `json:"price"`
}

var (
	ctx         context.Context
	redisClient *redis.Client
	db          *sqlx.DB
)

const (
	redisAddr     = "localhost:6379"
	redisPassword = ""
	redisDB       = 0

	psqlHost     = "localhost"
	psqlPort     = 5432
	psqlUser     = "postgres"
	psqlPassword = "12345S"
	psqlDBName   = "Sandugash"
)

func init() {
	ctx = context.Background()

	// Initialize Redis client
	redisClient = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	})

	psqlConn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", psqlHost, psqlPort, psqlUser, psqlPassword, psqlDBName)
	var err error
	db, err = sqlx.Connect("postgres", psqlConn)
	if err != nil {
		panic(err)
	}
}

func getProductByIDHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	productIDStr := params["id"]
	productID, err := strconv.Atoi(productIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Attempt to retrieve product data from Redis cache
	cachedProduct, err := redisClient.Get(ctx, "product:"+productIDStr).Result()
	if err == nil {
		// Return product data from Redis cache
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(cachedProduct))
		return
	}

	// Retrieve product data from PostgreSQL database
	var product Product
	err = db.Get(&product, "SELECT id, name, description, price FROM products WHERE id = $1", productID)
	if err != nil {
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Convert product to JSON
	productJSON, err := json.Marshal(product)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Cache product data in Redis with a TTL of 24 hours
	err = redisClient.Set(ctx, "product:"+productIDStr, string(productJSON), 24*time.Hour).Err()
	if err != nil {
		fmt.Println("Failed to cache product:", err)
	}

	// Return product data to client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(productJSON)
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/products/{id}", getProductByIDHandler).Methods("GET")

	fmt.Println("Server is running...")
	if err := http.ListenAndServe(":8080", r); err != nil {
		panic(err)
	}
}
