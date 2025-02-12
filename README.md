# Rumbo Backend 🚀

This Go application listens for location updates via MQTT and provides multiple REST and WebSocket endpoints to handle location data and a test location flow.

## Functionality Overview

- MQTT Client  
  - Subscribes to a topic for location updates.  
  - Automatically updates the global locationData when new data arrives.  

- REST Endpoints  
  - /api/location  
    - GET: Returns the current real-time location from mqtt.  
  - /api/update-test-location  
    - POST: Manually update the test location (lat/long) for testing purposes.  
  - /api/run-test-locations  
    - GET: Starts a sequence of test location updates every 5 seconds, using a predefined list of points.  

- WebSocket Endpoints  
  - /ws/location  
    - Sends real-time location updates using locationData.  
    - Includes messages about cached data, new location detection, or no signal if lat/lng = 0.  
  - /ws/test-location  
    - Similar to /ws/location, but uses testLocation.  

- Internal Helper Functions  
  - findStreet / findNearestStreet  
    - Determines the nearest street or checks if the point is within a polygon.  
  - arePointsWithinMargin  
    - Compares two points and returns true if they’re within 50cm.  
  
- Street Information Loading  
  - Loaded from street_info.json at startup, storing street geometry in memory.  

## How to Run

1. Configure environment variable PORT (optional).  
2. Build and run the main package:  
   go run main.go  
3. Open http://localhost:8080/ to verify it's running.  

## Notes

- Logging Middleware: All incoming requests are logged to the console.  
- testInProgress: Prevents overlapping test location sequences.

## License 📜

This project is licensed under the terms of the MIT license.
