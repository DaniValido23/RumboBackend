package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
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

var testLocation = struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}{
	Latitude:  45.492074,
	Longitude: -73.613610,
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// New structures for location and street information
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type StreetInfo struct {
	PlaceID string  `json:"placeId"`
	Points  []Point `json:"points"`
}

type LocationResponse struct {
	Location struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"location"`
	StreetInfo StreetInfo `json:"street_info"`
}



func main() {

	setupMQTT()

	r := mux.NewRouter()

	r.HandleFunc("/", HomeHandler).Methods("GET")
	r.HandleFunc("/api/location", GetLocationHandler).Methods("GET")
	r.HandleFunc("/api/update-test-location", UpdateTestLocationHandler).Methods("POST")
	r.HandleFunc("/ws/location", LocationWebSocketHandler)
	r.HandleFunc("/ws/test-location", TestLocationWebSocketHandler)

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

func UpdateTestLocationHandler(w http.ResponseWriter, r *http.Request) {
	var newLocation struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}
	if err := json.NewDecoder(r.Body).Decode(&newLocation); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	testLocation.Latitude = newLocation.Latitude
	testLocation.Longitude = newLocation.Longitude
	w.WriteHeader(http.StatusOK)
}

type GoogleRoadsResponse struct {
	SnappedPoints []struct {
		Location struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"location"`
		PlaceID string `json:"placeId"`
	} `json:"snappedPoints"`
}

func hasEnoughPoints(streetInfo StreetInfo) bool {
    return len(streetInfo.Points) > 1
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

func interpolatePoint(p1, p2 Point, fraction float64) Point {
    return Point{
        Lat: p1.Lat + (p2.Lat-p1.Lat)*fraction,
        Lng: p1.Lng + (p2.Lng-p1.Lng)*fraction,
    }
}

func generateEquidistantPoints(points []Point, targetCount int) []Point {
    if len(points) < 2 {
        return points
    }

    var totalDistance float64
    for i := 0; i < len(points)-1; i++ {
        totalDistance += calculateDistance(points[i], points[i+1])
    }

    spacing := totalDistance / float64(targetCount-1)
    result := make([]Point, 0, targetCount)
    result = append(result, points[0])

    currentDistance := 0.0
    segmentIndex := 0

    for i := 1; i < targetCount-1; i++ {
        targetDistance := spacing * float64(i)

        for currentDistance+calculateDistance(points[segmentIndex], points[segmentIndex+1]) < targetDistance {
            currentDistance += calculateDistance(points[segmentIndex], points[segmentIndex+1])
            segmentIndex++
        }

        remainingDistance := targetDistance - currentDistance
        segmentLength := calculateDistance(points[segmentIndex], points[segmentIndex+1])
        fraction := remainingDistance / segmentLength

        interpolated := interpolatePoint(points[segmentIndex], points[segmentIndex+1], fraction)
        result = append(result, interpolated)
    }

    result = append(result, points[len(points)-1])
    return result
}

func determineStreetDirection(lat, lng float64) (float64, float64, error) {
    initialOffset := 0.0001
    initialPoints := []Point{
        {Lat: lat, Lng: lng},
        {Lat: lat + initialOffset/2, Lng: lng},
        {Lat: lat + initialOffset, Lng: lng},
        {Lat: lat - initialOffset/2, Lng: lng},
        {Lat: lat - initialOffset, Lng: lng},
        {Lat: lat, Lng: lng + initialOffset/2},
        {Lat: lat, Lng: lng + initialOffset},
        {Lat: lat, Lng: lng - initialOffset/2},
        {Lat: lat, Lng: lng - initialOffset},
    }

    pointsStr := formatPointsForAPI(initialPoints)
    url := fmt.Sprintf(
        "https://roads.googleapis.com/v1/snapToRoads?path=%s&interpolate=true&key=%s",
        pointsStr, "AIzaSyA_igs8K_3i9fWu6htBgHQsHN_LxaZKMvk",
    )

    resp, err := http.Get(url)
    if err != nil {
        return 0, 0, err
    }
    defer resp.Body.Close()

    var roadResp GoogleRoadsResponse
    if err := json.NewDecoder(resp.Body).Decode(&roadResp); err != nil {
        return 0, 0, err
    }

    if len(roadResp.SnappedPoints) < 2 {
        return 0, 0, fmt.Errorf("insufficient points to determine direction")
    }

    p1 := roadResp.SnappedPoints[0].Location
    p2 := roadResp.SnappedPoints[len(roadResp.SnappedPoints)-1].Location
    
    dirLat := p2.Latitude - p1.Latitude
    dirLng := p2.Longitude - p1.Longitude
    
    magnitude := math.Sqrt(dirLat*dirLat + dirLng*dirLng)
    if magnitude == 0 {
        return 0, 0, fmt.Errorf("could not determine direction")
    }
    
    return dirLat/magnitude, dirLng/magnitude, nil
}

func generateStreetPoints(lat, lng float64) []Point {
    offset := 0.0009  // approximately 100 meters
    
    dirLat, dirLng, err := determineStreetDirection(lat, lng)
    if err != nil {
        smallOffset := offset / 5
        return []Point{
            {Lat: lat - offset, Lng: lng},
            {Lat: lat - smallOffset*4, Lng: lng},
            {Lat: lat - smallOffset*3, Lng: lng},
            {Lat: lat - smallOffset*2, Lng: lng},
            {Lat: lat - smallOffset, Lng: lng},
            {Lat: lat, Lng: lng},
            {Lat: lat + smallOffset, Lng: lng},
            {Lat: lat + smallOffset*2, Lng: lng},
            {Lat: lat + smallOffset*3, Lng: lng},
            {Lat: lat + smallOffset*4, Lng: lng},
            {Lat: lat + offset, Lng: lng},
        }
    }

    points := make([]Point, 0, 21)
    
    points = append(points, Point{Lat: lat, Lng: lng})
    
    for i := 1; i <= 10; i++ {
        fraction := float64(i) * 0.1
        points = append(points, Point{
            Lat: lat + dirLat*offset*fraction,
            Lng: lng + dirLng*offset*fraction,
        })
    }
    
    for i := 1; i <= 10; i++ {
        fraction := float64(i) * 0.1
        points = append(points, Point{
            Lat: lat - dirLat*offset*fraction,
            Lng: lng - dirLng*offset*fraction,
        })
    }
    
    return points
}

func formatPointsForAPI(points []Point) string {
	var parts []string
	for _, p := range points {
		parts = append(parts, fmt.Sprintf("%f,%f", p.Lat, p.Lng))
	}
	return strings.Join(parts, "|")
}

func getStreetInfo(lat, lng float64) (StreetInfo, error) {
    apiKey := "AIzaSyA_igs8K_3i9fWu6htBgHQsHN_LxaZKMvk"
    if apiKey == "" {
        return StreetInfo{}, fmt.Errorf("google maps api key not found")
    }

    var streetInfo StreetInfo
    maxRetries := 3
    retryCount := 0

    for retryCount < maxRetries {
        points := generateStreetPoints(lat, lng)
        pointsStr := formatPointsForAPI(points)

        url := fmt.Sprintf(
            "https://roads.googleapis.com/v1/snapToRoads?path=%s&interpolate=true&key=%s",
            pointsStr, apiKey,
        )

        resp, err := http.Get(url)
        if err != nil {
            log.Printf("Error making request: %v", err)
            return StreetInfo{}, err
        }

        body, err := io.ReadAll(resp.Body)
        resp.Body.Close()
        if err != nil {
            log.Printf("Error reading response body: %v", err)
            return StreetInfo{}, err
        }

        var roadResp GoogleRoadsResponse
        if err := json.Unmarshal(body, &roadResp); err != nil {
            log.Printf("Error unmarshalling response: %v", err)
            return StreetInfo{}, err
        }

        log.Printf("Number of snapped points received: %d", len(roadResp.SnappedPoints))

        points = make([]Point, 0)
        placeID := "-"

        if len(roadResp.SnappedPoints) > 0 {
            placeID = roadResp.SnappedPoints[0].PlaceID
            for _, sp := range roadResp.SnappedPoints {
                points = append(points, Point{
                    Lat: sp.Location.Latitude,
                    Lng: sp.Location.Longitude,
                })
            }
        }

        if len(points) > 1 {
            center := Point{Lat: lat, Lng: lng}
            sortedPoints := sortPointsByDirection(points, center)
            
            if len(sortedPoints) > 4 {
                points = []Point{
                    sortedPoints[0],
                    sortedPoints[len(sortedPoints)/2],
                    sortedPoints[len(sortedPoints)-1],
                }
            }
            
            points = generateEquidistantPoints(points, 10)
        }

        streetInfo = StreetInfo{
            PlaceID: placeID,
            Points:  points,
        }

        if hasEnoughPoints(streetInfo) {
            log.Printf("Got sufficient points (%d) on attempt %d", len(points), retryCount+1)
            return streetInfo, nil
        }

        log.Printf("Insufficient points (%d) on attempt %d", len(points), retryCount+1)
        retryCount++
        time.Sleep(time.Second)
    }

    return streetInfo, nil
}

func sortPointsByDirection(points []Point, center Point) []Point {
    dirLat, dirLng, err := determineStreetDirection(center.Lat, center.Lng)
    if err != nil {
        return points
    }

    sorted := make([]Point, len(points))
    copy(sorted, points)
    
    sort.Slice(sorted, func(i, j int) bool {
        pi := sorted[i]
        pj := sorted[j]
        
        proji := (pi.Lat-center.Lat)*dirLat + (pi.Lng-center.Lng)*dirLng
        projj := (pj.Lat-center.Lat)*dirLat + (pj.Lng-center.Lng)*dirLng
        
        return proji < projj
    })
    
    return sorted
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

    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        currentPoint := Point{Lat: locationData.Latitude, Lng: locationData.Longitude}

        if !firstRun && currentPoint == lastLocation {
            log.Printf("Using cached response for location: {lat: %f, lng: %f}", 
                currentPoint.Lat, currentPoint.Lng)
            if err := conn.WriteJSON(lastResponse); err != nil {
                log.Printf("Error writing cached JSON to websocket: %v", err)
                return
            }
            continue
        }

        streetInfo, err := getStreetInfo(locationData.Latitude, locationData.Longitude)
        if err != nil {
            log.Printf("Error getting street info: %v", err)
            continue
        }

        response := LocationResponse{
            Location: struct {
                Latitude  float64 `json:"latitude"`
                Longitude float64 `json:"longitude"`
            }{
                Latitude:  locationData.Latitude,
                Longitude: locationData.Longitude,
            },
            StreetInfo: streetInfo,
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

    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        currentPoint := Point{Lat: testLocation.Latitude, Lng: testLocation.Longitude}

        if !firstRun && currentPoint == lastLocation {
            log.Printf("Using cached response for test location: {lat: %f, lng: %f}", 
                currentPoint.Lat, currentPoint.Lng)
            if err := conn.WriteJSON(lastResponse); err != nil {
                log.Printf("Error writing cached JSON to websocket: %v", err)
                return
            }
            continue
        }

        streetInfo, err := getStreetInfo(testLocation.Latitude, testLocation.Longitude)
        if err != nil {
            log.Printf("Error getting street info: %v", err)
            continue
        }

        response := LocationResponse{
            Location: struct {
                Latitude  float64 `json:"latitude"`
                Longitude float64 `json:"longitude"`
            }{
                Latitude:  testLocation.Latitude,
                Longitude: testLocation.Longitude,
            },
            StreetInfo: streetInfo,
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

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.RequestURI)
		next.ServeHTTP(w, r)
	})
}
