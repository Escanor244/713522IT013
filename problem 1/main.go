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

const (
	WindowSize       = 10
	APITimeoutMs     = 500
	NumberServiceURL = "http://20.244.56.144/test"
)

type NumberResponse struct {
	Numbers []int `json:"numbers"`
}

type APIResponse struct {
	WindowPrevState []int   `json:"windowPrevState"`
	WindowCurrState []int   `json:"windowCurrState"`
	Numbers         []int   `json:"numbers"`
	Average         float64 `json:"avg"`
}

type NumberStore struct {
	numbers []int
	mu      sync.RWMutex
}

func (ns *NumberStore) AddNumbers(newNumbers []int) []int {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	prevState := make([]int, len(ns.numbers))
	copy(prevState, ns.numbers)

	uniqueNumbers := make(map[int]bool)
	for _, num := range ns.numbers {
		uniqueNumbers[num] = true
	}

	for _, num := range newNumbers {
		if !uniqueNumbers[num] {
			ns.numbers = append(ns.numbers, num)
			uniqueNumbers[num] = true
		}
	}

	if len(ns.numbers) > WindowSize {
		ns.numbers = ns.numbers[len(ns.numbers)-WindowSize:]
	}

	return prevState
}

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

func (ns *NumberStore) GetCurrentState() []int {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	current := make([]int, len(ns.numbers))
	copy(current, ns.numbers)
	return current
}

func fetchNumbers(numberType string, authToken string) ([]int, error) {
	client := &http.Client{
		Timeout: time.Duration(APITimeoutMs) * time.Millisecond,
	}

	url := fmt.Sprintf("%s/%s", NumberServiceURL, numberType)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server responded with status %d: %s", resp.StatusCode, string(body))
	}

	var result NumberResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if result.Numbers == nil || len(result.Numbers) == 0 {
		return nil, fmt.Errorf("no numbers received from server")
	}

	return result.Numbers, nil
}

func main() {
	router := gin.Default()
	store := &NumberStore{}

	numberTypes := map[string]string{
		"p": "primes",
		"f": "fibo",
		"e": "even",
		"r": "rand",
	}

	router.GET("/numbers/:numberid", func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authorization header"})
			return
		}

		if len(authHeader) <= 7 || authHeader[:7] != "Bearer " {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format. Use 'Bearer <token>'"})
			return
		}

		authToken := authHeader[7:]
		numberID := c.Param("numberid")
		numberType, valid := numberTypes[numberID]
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid number type"})
			return
		}

		numbers, err := fetchNumbers(numberType, authToken)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		prevState := store.AddNumbers(numbers)
		currState := store.GetCurrentState()
		avg := store.GetAverage()

		c.JSON(http.StatusOK, APIResponse{
			WindowPrevState: prevState,
			WindowCurrState: currState,
			Numbers:         numbers,
			Average:         avg,
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "9877"
	}

	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
