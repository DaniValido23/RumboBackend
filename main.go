package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
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
	r.HandleFunc("/ws/all-streets", AllStreetsWebSocketHandler)
	r.HandleFunc("/api/test/simulate-cars-100", SimulateCarsGroupHandler(100)).Methods("GET")
	r.HandleFunc("/api/test/simulate-cars-300", SimulateCarsGroupHandler(300)).Methods("GET")
	r.HandleFunc("/api/test/simulate-cars-500", SimulateCarsGroupHandler(500)).Methods("GET")
	r.Use(loggingMiddleware)

	handler := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "POST", "OPTIONS"}),
	)(r)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

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
		log.Printf("Handler cleanup complete for %s", conn.RemoteAddr())
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
			break
		}
	}

	log.Printf("Handler read loop exited for %s", conn.RemoteAddr())
}

func broadcastStreetIDUpdates(affectedIDs []int) {
	if len(affectedIDs) == 0 {
		return
	}
	var disconnectedClients []*websocket.Conn

	allStreetsMutex.RLock()
	streetMessages := make([]StreetUpdateMessage, 0, len(affectedIDs))
	for _, id := range affectedIDs {
		for _, street := range allStreets {
			if street.ID == id {
				msg := StreetUpdateMessage{
					ODONYME:    street.StreetName,
					ID_TRC:     street.ID,
					CarsNumber: street.Occupancy,
					HexColor:   calculateColorFromOccupancy(street.Occupancy),
				}
				streetMessages = append(streetMessages, msg)
				break
			}
		}
	}
	allStreetsMutex.RUnlock()

	for _, msg := range streetMessages {
		message, err := json.Marshal(msg)
		if err != nil {
			log.Printf("Error marshalling street update (%d): %v", msg.ID_TRC, err)
			continue
		}
		clientsMutex.RLock()
		var disconnectedThisSend []*websocket.Conn
		for client := range clients {
			client.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Error writing update for street ID %d to %s: %v", msg.ID_TRC, client.RemoteAddr(), err)
				disconnectedThisSend = append(disconnectedThisSend, client)
			}
		}
		clientsMutex.RUnlock()
		disconnectedClients = append(disconnectedClients, disconnectedThisSend...)
	}

	if len(disconnectedClients) > 0 {
		clientsMutex.Lock()
		uniqueDisconnected := make(map[*websocket.Conn]bool)
		for _, c := range disconnectedClients {
			if !uniqueDisconnected[c] {
				if _, ok := clients[c]; ok {
					delete(clients, c)
					c.Close()
					log.Printf("Removed client %s due to write error during broadcast. Total clients: %d", c.RemoteAddr(), len(clients))
				}
				uniqueDisconnected[c] = true
			}
		}
		clientsMutex.Unlock()
	}
}

func SimulateCarsGroupHandler(groupSize int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupIDs, err := loadRandomGroupIDs(groupSize)
		if err != nil {
			http.Error(w, "Error loading group IDs", http.StatusInternalServerError)
			return
		}
		allStreetsMutex.RLock()
		var testLocations []Point
		for _, id := range groupIDs {
			for _, st := range allStreets {
				if st.ID == id && len(st.Polylines) > 0 {
					testLocations = append(testLocations, st.Polylines[0])
					break
				}
			}
		}
		allStreetsMutex.RUnlock()
		if len(testLocations) == 0 {
			http.Error(w, "No test locations found for group", http.StatusInternalServerError)
			return
		}
		simulateCarsWithLocations(w, testLocations)
	}
}

func simulateCarsWithLocations(w http.ResponseWriter, testLocations []Point) {
	simulationCarIDs := []string{}

	carLocationsMutex.Lock()
	for i, loc := range testLocations {
		carID := fmt.Sprintf("sim-%d", i+1)
		carLocations[carID] = loc
		simulationCarIDs = append(simulationCarIDs, carID)
	}
	currentLocationsCopy := make(map[string]Point, len(carLocations))
	for k, v := range carLocations {
		currentLocationsCopy[k] = v
	}
	carLocationsMutex.Unlock()

	affectedIDs := recalculateStreetOccupancy(currentLocationsCopy)
	broadcastStreetIDUpdates(affectedIDs)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Car simulation started for %d cars. Simulated cars will disappear in 3 minutes.", len(testLocations))))
	log.Printf("Simulation started for %d cars. Scheduling disappearance in 3 minutes.", len(testLocations))

	time.AfterFunc(3*time.Minute, func() {
		carLocationsMutex.Lock()
		for _, id := range simulationCarIDs {
			delete(carLocations, id)
		}
		currentLocationsCopyAfter := make(map[string]Point, len(carLocations))
		for k, v := range carLocations {
			currentLocationsCopyAfter[k] = v
		}
		carLocationsMutex.Unlock()
		affectedIDs := recalculateStreetOccupancy(currentLocationsCopyAfter)
		broadcastStreetIDUpdates(affectedIDs)
		log.Println("Simulation ended.")
	})
}

func loadRandomGroupIDs(groupSize int) ([]int, error) {
	data, err := ioutil.ReadFile("random_id_groups.json")
	if err != nil {
		return nil, err
	}
	var groups map[string][]int
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, err
	}
	var key string
	switch groupSize {
	case 100:
		key = "group_100"
	case 300:
		key = "group_300"
	case 500:
		key = "group_500"
	default:
		return nil, fmt.Errorf("unsupported group size")
	}
	return groups[key], nil
}
