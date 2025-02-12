package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var locationData struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

var testLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

var testInProgress bool

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type StreetInfo struct {
	StreetName string  `json:"streetName"`
	Polylines  []Point `json:"polylines"`
}

type LocationResponse struct {
	Location struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"location"`
	StreetInfo StreetInfo `json:"street_info"`
	Message    string     `json:"message,omitempty"`
}

var allStreets []StreetData

type StreetPoint struct {
	ID  int     `json:"id"`
	Lat float64 `json:"lat"`
	Lng float64 `json:"long"`
}

type StreetData struct {
	ID              int           `json:"id"`
	StreetName      string        `json:"street_name"`
	ParkingCapacity int           `json:"parking_capacity"`
	Points          []StreetPoint `json:"points"`
	Polylines       []struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"long"`
	} `json:"polylines"`
}

func main() {

	setupMQTT()

	r := mux.NewRouter()

	r.HandleFunc("/", HomeHandler).Methods("GET")
	r.HandleFunc("/api/location", GetLocationHandler).Methods("GET")
	r.HandleFunc("/api/update-test-location", UpdateTestLocationHandler).Methods("POST")
	r.HandleFunc("/ws/location", LocationWebSocketHandler)
	r.HandleFunc("/ws/test-location", TestLocationWebSocketHandler)
	r.HandleFunc("/api/run-test-locations", RunTestLocationsHandler).Methods("GET")

	r.Use(loggingMiddleware)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server started at http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func setupMQTT() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://eu1.cloud.thethings.industries:1883")
	opts.SetClientID("go_mqtt_client")
	opts.SetUsername("gps-3a@parallel")
	opts.SetPassword("NNSXS.UYPEF3S77QZ7WFOPUTE3QWTFNBDIS7LPTRLJLPA.ZYIZOEME4WTRWUI6KV6VCFJIHBXO5TYXD6JFZS2V4GCCU27FQOQQ")

	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		log.Printf("Received message from topic: %s\n", msg.Topic())
		var payload struct {
			LocationSolved struct {
				Location struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				} `json:"location"`
			} `json:"location_solved"`
		}
		if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
			log.Printf("Error unmarshalling payload: %v", err)
			return
		}
		locationData.Latitude = payload.LocationSolved.Location.Latitude
		locationData.Longitude = payload.LocationSolved.Location.Longitude
		log.Printf("New location received - Latitude: %f, Longitude: %f", locationData.Latitude, locationData.Longitude)
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Error connecting to MQTT broker: %v", token.Error())
	}

	if token := client.Subscribe("v3/gps-3a@parallel/devices/eui-70b3d57ed80025b4/location/solved", 0, nil); token.Wait() && token.Error() != nil {
		log.Fatalf("Error subscribing to topic: %v", token.Error())
	}
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("¡Bienvenido a la API!"))
}

func GetLocationHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(locationData)
}

func parseCoordinate(coord string) (float64, error) {
	coord = strings.ReplaceAll(coord, ",", ".")
	return strconv.ParseFloat(coord, 64)
}

func UpdateTestLocationHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	lat, err := parseCoordinate(input.Latitude)
	if err != nil {
		http.Error(w, "Invalid latitude", http.StatusBadRequest)
		return
	}
	lng, err := parseCoordinate(input.Longitude)
	if err != nil {
		http.Error(w, "Invalid longitude", http.StatusBadRequest)
		return
	}

	testLocation.Latitude = lat
	testLocation.Longitude = lng
	w.WriteHeader(http.StatusOK)
}

func init() {
	loadStreetData()
}

func loadStreetData() {
	file, err := os.Open("street_info.json")
	if err != nil {
		log.Printf("Error opening street_info.json: %v", err)
		return
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&allStreets); err != nil {
		log.Printf("Error decoding street_info.json: %v", err)
	}
}

func pointInPolygon(lat, lng float64, poly []StreetPoint) bool {
	num := len(poly)
	inside := false
	for i := 0; i < num; i++ {
		j := (i + num - 1) % num
		if ((poly[i].Lat > lat) != (poly[j].Lat > lat)) &&
			(lng < (poly[j].Lng-poly[i].Lng)*(lat-poly[i].Lat)/(poly[j].Lat-poly[i].Lat)+poly[i].Lng) {
			inside = !inside
		}
	}
	return inside
}

func findNearestStreet(lat, lng float64) StreetInfo {
	var nearest StreetInfo
	minDistance := math.MaxFloat64
	origin := Point{Lat: lat, Lng: lng}

	for _, s := range allStreets {
		for _, pt := range s.Points {
			d := calculateDistance(origin, Point{Lat: pt.Lat, Lng: pt.Lng})
			if d < minDistance {
				minDistance = d
				var polylines []Point
				for _, p := range s.Polylines {
					polylines = append(polylines, Point{Lat: p.Lat, Lng: p.Lng})
				}
				nearest = StreetInfo{
					StreetName: s.StreetName,
					Polylines:  polylines,
				}
			}
		}
	}
	return nearest
}

func findStreet(lat, lng float64) StreetInfo {
	for _, s := range allStreets {
		if pointInPolygon(lat, lng, s.Points) {
			var resultPolylines []Point
			for _, p := range s.Polylines {
				resultPolylines = append(resultPolylines, Point{Lat: p.Lat, Lng: p.Lng})
			}
			return StreetInfo{
				StreetName: s.StreetName,
				Polylines:  resultPolylines,
			}
		}
	}
	return findNearestStreet(lat, lng)
}

func LocationWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to websocket: %v", err)
		return
	}
	defer conn.Close()

	var lastLocation Point
	var lastResponse LocationResponse
	firstRun := true

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		currentPoint := Point{Lat: locationData.Latitude, Lng: locationData.Longitude}

		if currentPoint.Lat == 0.0 && currentPoint.Lng == 0.0 {
			response := LocationResponse{
				Location: struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				}{
					Latitude:  0.0,
					Longitude: 0.0,
				},
				StreetInfo: StreetInfo{
					StreetName: "",
					Polylines:  []Point{},
				},
				Message: "without signal",
			}
			if err := conn.WriteJSON(response); err != nil {
				log.Printf("Error writing JSON to websocket: %v", err)
				return
			}
			continue
		}

		if !firstRun && arePointsWithinMargin(currentPoint, lastLocation) {
			lastResponse.Message = "Using cached location"
			if err := conn.WriteJSON(lastResponse); err != nil {
				log.Printf("Error writing cached JSON to websocket: %v", err)
				return
			}
			continue
		} else {
			log.Printf("New location update - Latitude: %f, Longitude: %f", currentPoint.Lat, currentPoint.Lng)
			streetInfo := findStreet(locationData.Latitude, locationData.Longitude)
			response := LocationResponse{
				Location: struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				}{
					Latitude:  locationData.Latitude,
					Longitude: locationData.Longitude,
				},
				StreetInfo: streetInfo,
				Message:    "New location detected",
			}
			lastLocation = currentPoint
			lastResponse = response
			firstRun = false

			if err := conn.WriteJSON(response); err != nil {
				log.Printf("Error writing JSON to websocket: %v", err)
				return
			}
		}
	}
}

func TestLocationWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to websocket: %v", err)
		return
	}
	defer conn.Close()

	var lastLocation Point
	var lastResponse LocationResponse
	firstRun := true

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !testInProgress {
			log.Printf("Test run completed. Stopping WebSocket updates.")
			break
		}

		currentPoint := Point{Lat: testLocation.Latitude, Lng: testLocation.Longitude}

		if currentPoint.Lat == 0.0 && currentPoint.Lng == 0.0 {
			response := LocationResponse{
				Location: struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				}{
					Latitude:  0.0,
					Longitude: 0.0,
				},
				StreetInfo: StreetInfo{
					StreetName: "",
					Polylines:  []Point{},
				},
				Message: "without signal",
			}
			if err := conn.WriteJSON(response); err != nil {
				log.Printf("Error writing JSON to websocket: %v", err)
				return
			}
			continue
		}

		if !firstRun && arePointsWithinMargin(currentPoint, lastLocation) {
			lastResponse.Message = "Using cached location"
			if err := conn.WriteJSON(lastResponse); err != nil {
				log.Printf("Error writing cached JSON to websocket: %v", err)
				return
			}
			continue
		} else {
			log.Printf("New test location update - Latitude: %f, Longitude: %f", currentPoint.Lat, currentPoint.Lng)
			streetInfo := findStreet(testLocation.Latitude, testLocation.Longitude)
			response := LocationResponse{
				Location: struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				}{
					Latitude:  testLocation.Latitude,
					Longitude: testLocation.Longitude,
				},
				StreetInfo: streetInfo,
				Message:    "New location detected",
			}
			lastLocation = currentPoint
			lastResponse = response
			firstRun = false

			if err := conn.WriteJSON(response); err != nil {
				log.Printf("Error writing JSON to websocket: %v", err)
				return
			}
		}
	}
}

func RunTestLocationsHandler(w http.ResponseWriter, r *http.Request) {
	if testInProgress {
		w.Write([]byte("Test already in progress"))
		return
	}
	testInProgress = true

	go func() {
		points := []Point{
			{ Lat: 45.5834154, Lng: -73.5331976 },
			{ Lat: 45.5836587, Lng: -73.5330197 },
			{ Lat: 45.5839076, Lng: -73.5328529 },
			{ Lat: 45.5841270, Lng: -73.5334444 },
			{ Lat: 45.5844604, Lng: -73.5344887 },
			{ Lat: 45.5847659, Lng: -73.5353965 },
			{ Lat: 45.5850823, Lng: -73.5363582 },
			{ Lat: 45.5858864, Lng: -73.5362865 },
			{ Lat: 45.5855239, Lng: -73.5351299 },
			{ Lat: 45.5850867, Lng: -73.5338104 },
			{ Lat: 45.5847081, Lng: -73.5326593 },
			{ Lat: 45.5846233, Lng: -73.5323988 },
			{ Lat: 45.5848715, Lng: -73.5322361 },
			{ Lat: 45.5850948, Lng: -73.5320983 },
			{ Lat: 0.0, Lng: 0.0 },
			{ Lat: 0.0, Lng: 0.0 },
			{ Lat: 0.0, Lng: 0.0 },
		}
		for _, p := range points {
			testLocation.Latitude = p.Lat
			testLocation.Longitude = p.Lng
			time.Sleep(5 * time.Second)
		}
		testInProgress = false
	}()

	w.Write([]byte("Test started"))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.RequestURI)
		next.ServeHTTP(w, r)
	})
}

// Helper function to check if two points are within 50cm of each other
func arePointsWithinMargin(p1, p2 Point) bool {
	distance := calculateDistance(p1, p2)
	return distance <= 0.5
}

func calculateDistance(p1, p2 Point) float64 {
	const R = 6371000

	lat1 := p1.Lat * math.Pi / 180
	lat2 := p2.Lat * math.Pi / 180
	deltaLat := (p2.Lat - p1.Lat) * math.Pi / 180
	deltaLng := (p2.Lng - p1.Lng) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*
			math.Sin(deltaLng/2)*math.Sin(deltaLng/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}
