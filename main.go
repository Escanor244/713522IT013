package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Configuration constants
const (
	// Maximum number of numbers to store in the sliding window
	WindowSize = 10

	// Timeout for external API calls in milliseconds
	APITimeoutMs = 500

	// Base URL for the number generation service
	NumberServiceURL = "http://20.244.56.144/test"
)

// NumberResponse represents the response from the number service
type NumberResponse struct {
	Numbers []int `json:"numbers"`
}

// APIResponse represents our service's response format
type APIResponse struct {
	WindowPrevState []int   `json:"windowPrevState"` // State before adding new numbers
	WindowCurrState []int   `json:"windowCurrState"` // State after adding new numbers
	Numbers         []int   `json:"numbers"`         // Numbers received from external service
	Average         float64 `json:"avg"`             // Average of current window state
}

// NumberStore manages the sliding window of numbers
type NumberStore struct {
	numbers []int
	mu      sync.RWMutex // Protects concurrent access to numbers
}

// AddNumbers adds new numbers to the store and returns the previous state
func (ns *NumberStore) AddNumbers(newNumbers []int) []int {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	// Save current state to return later
	prevState := make([]int, len(ns.numbers))
	copy(prevState, ns.numbers)

	// Track unique numbers using a map
	uniqueNumbers := make(map[int]bool)
	for _, num := range ns.numbers {
		uniqueNumbers[num] = true
	}

	// Add new unique numbers
	for _, num := range newNumbers {
		if !uniqueNumbers[num] {
			ns.numbers = append(ns.numbers, num)
			uniqueNumbers[num] = true
		}
	}

	// Maintain window size by keeping only the last WindowSize numbers
	if len(ns.numbers) > WindowSize {
		ns.numbers = ns.numbers[len(ns.numbers)-WindowSize:]
	}

	return prevState
}

// GetAverage calculates the average of numbers in the current window
func (ns *NumberStore) GetAverage() float64 {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	if len(ns.numbers) == 0 {
		return 0
	}

	sum := 0
	for _, num := range ns.numbers {
		sum += num
	}
	return float64(sum) / float64(len(ns.numbers))
}

// GetCurrentState returns a copy of the current window state
func (ns *NumberStore) GetCurrentState() []int {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	currentState := make([]int, len(ns.numbers))
	copy(currentState, ns.numbers)
	return currentState
}

// fetchNumbers retrieves numbers from the external service
func fetchNumbers(numberType string, authToken string) ([]int, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: time.Duration(APITimeoutMs) * time.Millisecond,
	}

	// Prepare the request
	url := fmt.Sprintf("%s/%s", NumberServiceURL, numberType)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Add required headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server responded with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result NumberResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	// Validate response data
	if result.Numbers == nil || len(result.Numbers) == 0 {
		return nil, fmt.Errorf("no numbers received from server")
	}

	return result.Numbers, nil
}

func main() {
	// Initialize Gin router
	router := gin.Default()

	// Initialize number store
	store := &NumberStore{}

	// Define number type mapping
	numberTypes := map[string]string{
		"p": "primes", // Prime numbers
		"f": "fibo",   // Fibonacci numbers
		"e": "even",   // Even numbers
		"r": "rand",   // Random numbers
	}

	// Handler for number requests
	router.GET("/numbers/:numberid", func(c *gin.Context) {
		// Get authorization token from header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			return
		}

		// Extract token from "Bearer <token>"
		if len(authHeader) <= 7 || authHeader[:7] != "Bearer " {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format. Use 'Bearer <token>'"})
			return
		}
		authToken := authHeader[7:]

		numberID := c.Param("numberid")

		// Validate number type
		numberType, valid := numberTypes[numberID]
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid number ID. Use 'p' for prime, 'f' for fibonacci, 'e' for even, or 'r' for random numbers"})
			return
		}

		// Fetch numbers from external service
		numbers, err := fetchNumbers(numberType, authToken)
		if err != nil {
			log.Printf("Error fetching numbers: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":           fmt.Sprintf("Failed to fetch numbers: %v", err),
				"windowPrevState": store.GetCurrentState(),
				"windowCurrState": store.GetCurrentState(),
				"numbers":         nil,
				"avg":             store.GetAverage(),
			})
			return
		}

		// Update store and prepare response
		prevState := store.AddNumbers(numbers)
		currState := store.GetCurrentState()
		avg := store.GetAverage()

		// Send response
		c.JSON(http.StatusOK, APIResponse{
			WindowPrevState: prevState,
			WindowCurrState: currState,
			Numbers:         numbers,
			Average:         avg,
		})
	})

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "9877"
	}

	// Start server
	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
