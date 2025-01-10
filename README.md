# Rumbo Backend

This is a simple backend in Go using the `gorilla/mux` package for routing. It is part of the RumboApp, which is still under construction.

## Requirements

- Go 1.21 or higher

## Installation

1. Clone the repository:
    ```bash
    git clone <REPOSITORY_URL>
    cd rumbo_backend
    ```

2. Install dependencies:
    ```bash
    go mod tidy
    ```

## Usage

To start the server, run:
```bash
go run main.go
```

The server will start at `http://localhost:8080`.

## Endpoints

- `GET /` - Welcome page
- `GET /api/items` - Get items
- `POST /api/items` - Create an item

## Middleware

This project includes a middleware for request logging.

## License

This project is licensed under the terms of the MIT license.
