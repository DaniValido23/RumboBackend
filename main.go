package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

const (
	writeWait = 10 * time.Second

	pongWait = 60 * time.Second

	pingPeriod = (pongWait * 9) / 10

	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var clients = make(map[*websocket.Conn]bool)
var clientsMutex sync.RWMutex

func main() {
	loadStreetData()
	setupMQTT()

	r := mux.NewRouter()

	r.HandleFunc("/", HomeHandler).Methods("GET")
	r.HandleFunc("/api/streets", GetAllStreetsHandler).Methods("GET")
	r.HandleFunc("/ws/all-streets", AllStreetsWebSocketHandler)
	// Add the new test endpoint route
	r.HandleFunc("/api/test/simulate-cars", SimulateCarsHandler).Methods("GET")
	r.Use(loggingMiddleware)

	handler := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "POST", "OPTIONS"}),
	)(r)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	go broadcastOccupancyUpdates()

	log.Printf("Server started at http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("¡Bienvenido a la API!"))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.RequestURI)
		next.ServeHTTP(w, r)
	})
}

func GetAllStreetsHandler(w http.ResponseWriter, r *http.Request) {
	allStreetsMutex.RLock()
	defer allStreetsMutex.RUnlock()

	resp := make([]StreetResponse, len(allStreets))
	for i, st := range allStreets {
		resp[i] = StreetResponse{
			StreetName:        st.StreetName,
			Polylines:         st.Polylines,
			Occupancy:         st.Occupancy,
			Capacity:          st.Capacity,
			AvailableCapacity: st.Capacity - st.Occupancy,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}
	const chunkSize = 500
	w.Write([]byte("["))
	for i := 0; i < len(resp); i += chunkSize {
		end := i + chunkSize
		if end > len(resp) {
			end = len(resp)
		}
		chunk := resp[i:end]
		data, err := json.Marshal(chunk)
		if err != nil {
			log.Printf("Error marshalling street chunk: %v", err)
			break
		}
		w.Write(data)
		if end < len(resp) {
			w.Write([]byte(","))
		}
		flusher.Flush()
	}
	w.Write([]byte("]"))
}

func AllStreetsWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to websocket for all streets: %v", err)
		return
	}

	clientsMutex.Lock()
	clients[conn] = true
	log.Printf("Client connected to /ws/all-streets: %s. Total clients: %d", conn.RemoteAddr(), len(clients))
	clientsMutex.Unlock()

	pingTicker := time.NewTicker(pingPeriod)

	defer func() {
		pingTicker.Stop()
		clientsMutex.Lock()
		if _, ok := clients[conn]; ok {
			delete(clients, conn)
			log.Printf("Handler cleanup: Removed client %s. Total clients: %d", conn.RemoteAddr(), len(clients))
		} else {
			log.Printf("Handler cleanup: Client %s already removed. Total clients: %d", conn.RemoteAddr(), len(clients))
		}
		clientsMutex.Unlock()
		conn.Close()
		log.Printf("Handler cleanup complete for %s", conn.RemoteAddr()) // Add a final confirmation log
	}()

	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	go func() {
		defer func() {
			log.Printf("Ping goroutine for %s exiting.", conn.RemoteAddr())
		}()
		for range pingTicker.C {
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Error sending ping to %s: %v", conn.RemoteAddr(), err)
				return
			}
		}
	}()
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			// error/disconnect before breaking
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				log.Printf("Handler exit: Unexpected close error for client %s: %v", conn.RemoteAddr(), err)
			} else if e, ok := err.(*websocket.CloseError); ok && e.Code == websocket.CloseNormalClosure {

				log.Printf("Handler exit: Client %s closed connection normally.", conn.RemoteAddr())
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {

				log.Printf("Handler exit: Read deadline exceeded (pong timeout) for client %s: %v", conn.RemoteAddr(), err)
			} else {

				log.Printf("Handler exit: Client %s connection error: %v", conn.RemoteAddr(), err)
			}
			break
		}
	}

	log.Printf("Handler read loop exited for %s", conn.RemoteAddr())
}

func broadcastOccupancyUpdates() {
	ticker := time.NewTicker(20 * time.Second) // Keep the ticker interval
	defer ticker.Stop()

	for range ticker.C {
		var disconnectedClients []*websocket.Conn // Collect disconnected clients across all sends in this tick

		allStreetsMutex.Lock() // Use write lock as we might update LastOccupancy
		for i := range allStreets {
			if allStreets[i].Occupancy != allStreets[i].LastOccupancy {
				// Occupancy changed for this street
				allStreets[i].LastOccupancy = allStreets[i].Occupancy // Update LastOccupancy

				// Create response for this specific street
				st := allStreets[i] // Create a copy for safety within the loop iteration
				update := StreetResponse{
					StreetName:        st.StreetName,
					Polylines:         st.Polylines,
					Occupancy:         st.Occupancy,
					Capacity:          st.Capacity,
					AvailableCapacity: st.Capacity - st.Occupancy,
				}

				// Marshal the individual update
				message, err := json.Marshal(update)
				if err != nil {
					log.Printf("Error marshalling single street update (%s): %v", st.StreetName, err)
					continue // Skip broadcasting this update if marshalling fails
				}

				// Broadcast this specific update to all clients
				log.Printf("Broadcasting update for street %s to %d clients.", st.StreetName, len(clients))
				clientsMutex.RLock() // Use read lock for broadcasting
				var disconnectedThisSend []*websocket.Conn
				for client := range clients {
					client.SetWriteDeadline(time.Now().Add(writeWait))
					if err := client.WriteMessage(websocket.TextMessage, message); err != nil {
						log.Printf("Error writing update for street %s to %s: %v", st.StreetName, client.RemoteAddr(), err)
						// Don't remove immediately, collect and remove outside the RLock
						disconnectedThisSend = append(disconnectedThisSend, client)
					}
				}
				clientsMutex.RUnlock()

				// Add clients that failed this specific send to the overall list for this tick
				// Assign the result of append back to disconnectedClients
				disconnectedClients = append(disconnectedClients, disconnectedThisSend...)
			}
		}
		allStreetsMutex.Unlock() // Release lock after checking all streets

		// Clean up disconnected clients after iterating through all streets and attempting sends
		if len(disconnectedClients) > 0 {
			clientsMutex.Lock()                                  // Use write lock for removal
			uniqueDisconnected := make(map[*websocket.Conn]bool) // Ensure we only process each disconnected client once per tick
			for _, c := range disconnectedClients {
				if !uniqueDisconnected[c] {
					if _, ok := clients[c]; ok { // Check if client wasn't already removed
						delete(clients, c)
						c.Close() // Close the connection
						log.Printf("Removed client %s due to write error during broadcast. Total clients: %d", c.RemoteAddr(), len(clients))
					}
					uniqueDisconnected[c] = true
				}
			}
			clientsMutex.Unlock()
		}
	}
}

// SimulateCarsHandler handles the test endpoint to simulate car presence and disappearance.
func SimulateCarsHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request to simulate cars.")

	testLocations := []Point{
		{Lat: 45.582959, Lng: -73.532044},
		{Lat: 45.584344, Lng: -73.531706},
		{Lat: 45.585149, Lng: -73.536469},
		{Lat: 45.586456, Lng: -73.535766},
		{Lat: 45.583817, Lng: -73.533524},
	}

	// Keep track of the IDs used in this simulation run
	simulationCarIDs := []string{}

	// --- Start Simulation ---
	carLocationsMutex.Lock()
	// Don't clear existing locations. Add/overwrite simulated cars.
	// carLocations = make(map[string]Point)
	// log.Printf("Cleared existing car locations for simulation.") // Log message no longer accurate

	// Add test locations
	for i, loc := range testLocations {
		carID := fmt.Sprintf("sim-%d", i+1)
		carLocations[carID] = loc
		// Collect the IDs we are adding/modifying
		simulationCarIDs = append(simulationCarIDs, carID)
		log.Printf("Added/Updated simulated car %s at Lat: %f, Lng: %f", carID, loc.Lat, loc.Lng)
	}

	// Create a copy for recalculation (includes real cars + simulated cars)
	currentLocationsCopy := make(map[string]Point, len(carLocations))
	for k, v := range carLocations {
		currentLocationsCopy[k] = v
	}
	carLocationsMutex.Unlock()

	// Recalculate occupancy with simulated cars added
	log.Println("Recalculating occupancy for simulated cars...")
	go recalculateStreetOccupancy(currentLocationsCopy) // Run in goroutine to avoid blocking response

	// Respond to the client immediately
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Car simulation started. Simulated cars will disappear in 1 minute."))
	log.Println("Simulation started. Scheduling disappearance in 1 minute.")

	// --- Schedule End Simulation ---
	// Use the simulationCarIDs captured at the start of this specific request
	time.AfterFunc(1*time.Minute, func() {
		log.Println("1 minute passed. Simulating car disappearance...")
		carLocationsMutex.Lock()

		// Remove only the specific simulation cars added by this run
		// carLocations = make(map[string]Point) // Don't clear all
		log.Printf("Removing %d simulated car locations.", len(simulationCarIDs))
		for _, id := range simulationCarIDs {
			delete(carLocations, id)
			log.Printf("Removed simulated car %s", id)
		}

		// Create a copy (now with only real cars or cars from other simulations) for recalculation
		currentLocationsCopyAfter := make(map[string]Point, len(carLocations))
		for k, v := range carLocations {
			currentLocationsCopyAfter[k] = v
		}
		carLocationsMutex.Unlock()

		// Recalculate occupancy (should reset occupancy caused by these simulated cars)
		log.Println("Recalculating occupancy after simulated car disappearance...")
		recalculateStreetOccupancy(currentLocationsCopyAfter)
		log.Println("Simulation ended.")
	})
}
