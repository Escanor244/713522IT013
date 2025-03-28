# Average Calculator HTTP Microservice

This is a Go-based microservice that calculates averages based on numbers fetched from a third-party server. The service maintains a sliding window of unique numbers and provides their average.

## Requirements

- Go 1.21 or higher
- Internet connection to access the test server

## Installation

1. Clone the repository
2. Install dependencies:
```bash
go mod download
```

## Running the Service

Start the server:
```bash
go run main.go
```

The server will start on port 9876 by default.

## API Endpoints

### GET /numbers/{numberid}

Fetches numbers based on the specified type and returns their average along with window states.

Valid number IDs:
- `p`: Prime numbers
- `f`: Fibonacci numbers
- `e`: Even numbers
- `r`: Random numbers

Example request:
```bash
curl http://localhost:9876/numbers/e
```

Example response:
```json
{
    "windowPrevState": [],
    "windowCurrState": [2,4,6,8],
    "numbers": [2,4,6,8],
    "avg": 5.00
}
```

## Features

- Window size: 10 numbers
- Timeout: 500ms for external API calls
- Unique number storage
- Thread-safe operations
- Sliding window implementation 