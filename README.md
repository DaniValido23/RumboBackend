# Rumbo Backend 🚀

This Go application listens for location updates via MQTT, stores historical data, and provides multiple REST and WebSocket endpoints to handle location data and a test location flow.

## Functionality Overview

- MQTT Client
  - Subscribes to a topic for location updates.
  - Automatically updates the global locationData when new data arrives.
- **Database Integration**
  - **Stores historical location updates in a PostgreSQL database.**
- REST Endpoints
  - `/api/location`
    - GET: Returns the current real-time location from mqtt.
  - **`/api/location/history`**
    - **GET: Returns historical location data from the database (pagination supported).**
  - `/api/update-test-location`
    - POST: Manually update the test location (lat/long) for testing purposes.
  - `/api/run-test-locations`
    - GET: Starts a sequence of test location updates every 5 seconds, using a predefined list of points.
- WebSocket Endpoints
  - `/ws/location`
    - Sends real-time location updates using locationData.
    - Includes messages about cached data, new location detection, or no signal if lat/lng = 0.
  - `/ws/test-location`
    - Similar to /ws/location, but uses testLocation.
- Internal Helper Functions
  - findStreet / findNearestStreet
    - Determines the nearest street or checks if the point is within a polygon.
  - arePointsWithinMargin
    - Compares two points and returns true if they’re within 50cm.
- Street Information Loading
  - Loaded from street_info.json at startup, storing street geometry in memory.

## Configuration

Configuration is managed through a `config.yaml` file and environment variables (environment variables override file settings).

- `config.yaml`: Defines MQTT broker details, database connection string, server port, etc.
- Environment Variables: `PORT`, `DATABASE_URL`, `MQTT_BROKER`, etc.

## How to Run

### Prerequisites

- Go (version X.Y or later)
- PostgreSQL Database
- Docker (optional, for containerized deployment)

### Running Locally

1. Clone the repository.
2. Set up the PostgreSQL database and update the connection string in `config.yaml` or via the `DATABASE_URL` environment variable.
3. Configure MQTT settings in `config.yaml` or via environment variables.
4. Run database migrations (if applicable).
5. Build and run the main package:
   ```bash
   go run main.go
   ```
6. Open http://localhost:[PORT] (default 8080) to verify it's running.


## Notes

- Logging Middleware: All incoming requests are logged to the console.
- testInProgress: Prevents overlapping test location sequences.

## License 📜

This project is licensed under the terms of the MIT license.
