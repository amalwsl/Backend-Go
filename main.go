package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"

	_ "github.com/glebarez/sqlite"
	"github.com/gorilla/mux"
)

// Car represents a car entity.
type Car struct {
	Model        string `json:"model"`
	Registration string `json:"registration"`
	Mileage      int    `json:"mileage"`
	Rented       bool   `json:"rented"`
}

var (
	cars     []Car
	carsLock sync.RWMutex
	db       *sql.DB
)

func main() {
	var err error
	db, err = sql.Open("sqlite", "cars.db")
	if err != nil {
		log.Fatal("Error opening database:", err)
	}
	defer db.Close()

	// Create table
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS cars (
		model TEXT,
		registration TEXT PRIMARY KEY,
		mileage INTEGER,
		rented BOOLEAN
	)`)
	if err != nil {
		log.Fatal("Error creating table:", err)
	}

	// Insert mock data
	_, err = db.Exec(`INSERT INTO cars (model, registration, mileage, rented)
		VALUES ('Tesla M3', 'BTS812', 6003, 0)`)
	if err != nil {
		log.Fatal("Error inserting data:", err)
	}

	r := mux.NewRouter()

	r.HandleFunc("/cars", listAvailableCars).Methods("GET")
	r.HandleFunc("/cars", addCar).Methods("POST")
	r.HandleFunc("/cars/{registration}/rentals", rentCar).Methods("POST")
	r.HandleFunc("/cars/{registration}/returns", returnCar).Methods("POST")

	log.Fatal(http.ListenAndServe(":8080", r))
}

func listAvailableCars(w http.ResponseWriter, r *http.Request) {
	carsLock.RLock()
	defer carsLock.RUnlock()

	// Query data from database
	rows, err := db.Query("SELECT model, registration, mileage, rented FROM cars")
	if err != nil {
		log.Printf("Error querying data: %v", err)                                         // Log detailed error information
		http.Error(w, "Failed to retrieve available cars", http.StatusInternalServerError) // Return appropriate HTTP status code
		return
	}
	defer rows.Close()

	var availableCars []Car
	for rows.Next() {
		var car Car
		err := rows.Scan(&car.Model, &car.Registration, &car.Mileage, &car.Rented)
		if err != nil {
			log.Printf("Error scanning row: %v", err)                                   // Log detailed error information
			http.Error(w, "Failed to process car data", http.StatusInternalServerError) // Return appropriate HTTP status code
			return
		}
		if !car.Rented {
			availableCars = append(availableCars, car)
		}
	}

	// Encode and send response
	if err := json.NewEncoder(w).Encode(availableCars); err != nil {
		log.Printf("Error encoding JSON response: %v", err)                             // Log detailed error information
		http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError) // Return appropriate HTTP status code
		return
	}
}

func addCar(w http.ResponseWriter, r *http.Request) {
	var newCar Car
	err := json.NewDecoder(r.Body).Decode(&newCar)
	if err != nil {
		log.Printf("Error decoding JSON request: %v", err)           // Log detailed error information
		http.Error(w, "Invalid request body", http.StatusBadRequest) // Return appropriate HTTP status code
		return
	}

	// Insert new car into database
	_, err = db.Exec(`INSERT INTO cars (model, registration, mileage, rented)
        VALUES (?, ?, ?, ?)`, newCar.Model, newCar.Registration, newCar.Mileage, newCar.Rented)
	if err != nil {
		log.Printf("Error inserting data: %v", err)                        // Log detailed error information
		http.Error(w, "Failed to add car", http.StatusInternalServerError) // Return appropriate HTTP status code
		return
	}

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"message": "Car added successfully"}); err != nil {
		log.Printf("Error encoding JSON response: %v", err)                             // Log detailed error information
		http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError) // Return appropriate HTTP status code
		return
	}
}

func rentCar(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	registration := params["registration"]

	carsLock.Lock()
	defer carsLock.Unlock()

	for i := range cars {
		if cars[i].Registration == registration {
			if cars[i].Rented {
				log.Printf("Car %s is already rented", registration)          // Log detailed error information
				http.Error(w, "Car is already rented", http.StatusBadRequest) // Return appropriate HTTP status code
				return
			}
			cars[i].Rented = true
			_, err := db.Exec("UPDATE cars SET rented = true WHERE registration = ?", registration)
			if err != nil {
				log.Printf("Error updating database: %v", err)                                      // Log detailed error information
				http.Error(w, "Failed to update car rental status", http.StatusInternalServerError) // Return appropriate HTTP status code
				return
			}
			if err := json.NewEncoder(w).Encode(map[string]interface{}{"message": "Car rented successfully"}); err != nil {
				log.Printf("Error encoding JSON response: %v", err)                             // Log detailed error information
				http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError) // Return appropriate HTTP status code
				return
			}
			return
		}
	}

	log.Printf("Car %s not found !!--", registration)    // Log detailed error information
	http.Error(w, "Car not found ", http.StatusNotFound) // Return appropriate HTTP status code
}

func returnCar(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	registration := params["registration"]

	carsLock.Lock()
	defer carsLock.Unlock()

	for i := range cars {
		if cars[i].Registration == registration {
			if !cars[i].Rented {
				log.Printf("Car %s was not rented", registration)          // Log detailed error information
				http.Error(w, "Car was not rented", http.StatusBadRequest) // Return appropriate HTTP status code
				return
			}
			// If there's a mileage parameter in the request, update the car's mileage
			if mileageStr := r.URL.Query().Get("mileage"); mileageStr != "" {
				mileage, err := strconv.Atoi(mileageStr)
				if err != nil {
					log.Printf("Invalid mileage: %v", err)                  // Log detailed error information
					http.Error(w, "Invalid mileage", http.StatusBadRequest) // Return appropriate HTTP status code
					return
				}
				cars[i].Mileage += mileage
				_, err = db.Exec("UPDATE cars SET rented = false, mileage = ? WHERE registration = ?", cars[i].Mileage, registration)
				if err != nil {
					log.Printf("Error updating database: %v", err)                             // Log detailed error information
					http.Error(w, "Failed to update car data", http.StatusInternalServerError) // Return appropriate HTTP status code
					return
				}
			} else {
				cars[i].Rented = false
				_, err := db.Exec("UPDATE cars SET rented = false WHERE registration = ?", registration)
				if err != nil {
					log.Printf("Error updating database: %v", err)                                      // Log detailed error information
					http.Error(w, "Failed to update car rental status", http.StatusInternalServerError) // Return appropriate HTTP status code
					return
				}
			}
			if err := json.NewEncoder(w).Encode(map[string]interface{}{"message": "Car returned successfully"}); err != nil {
				log.Printf("Error encoding JSON response: %v", err)                             // Log detailed error information
				http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError) // Return appropriate HTTP status code
				return
			}
			return
		}
	}

	log.Printf("Car %s not found", registration)         // Log detailed error information
	http.Error(w, "Car not found ", http.StatusNotFound) // Return appropriate HTTP status code
}
