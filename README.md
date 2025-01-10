# Rumbo Backend 🚀

This is a simple backend in Go using the `gorilla/mux` package for routing. It is part of the RumboApp, which is still under construction. 🛠️



## Usage ▶️

To start the server, run:
```bash
go run main.go
```

The server will start at `http://localhost:8080`.

## Endpoints 🌐

- `GET /` - Welcome page
- `GET /api/location` - Get the location of Tower1

## MQTT Connection 📡

This project connects to the Tower1 device using the MQTT protocol to retrieve its location data. The location data is then served through the `/api/location` endpoint.


## License 📜

This project is licensed under the terms of the MIT license.
