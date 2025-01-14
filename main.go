package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
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

func main() {
    // Remove loading environment variables from .env file
    // if err := godotenv.Load(); err != nil {
    //     log.Fatalf("Error loading .env file: %v", err)
    // }

    setupMQTT()

    // Configuración del servidor HTTP
    r := mux.NewRouter()
    
    // Rutas
    r.HandleFunc("/", HomeHandler).Methods("GET")
    r.HandleFunc("/api/location", GetLocationHandler).Methods("GET")
    r.HandleFunc("/api/update-test-location", UpdateTestLocationHandler).Methods("POST")
    r.HandleFunc("/ws/location", LocationWebSocketHandler)
    r.HandleFunc("/ws/test-location", TestLocationWebSocketHandler)

    // Middleware
    r.Use(loggingMiddleware)

    // Obtener el puerto del entorno
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080" // Default port if not specified
    }

    // Iniciar servidor
    log.Printf("Servidor iniciado en http://localhost:%s", port)
    log.Fatal(http.ListenAndServe(":"+port, r))
}

func setupMQTT() {
    // Configuración MQTT
    opts := mqtt.NewClientOptions()
    opts.AddBroker("tcp://eu1.cloud.thethings.industries:1883")
    opts.SetClientID("go_mqtt_client")
    opts.SetUsername("gps-3a@parallel")
    opts.SetPassword("NNSXS.UYPEF3S77QZ7WFOPUTE3QWTFNBDIS7LPTRLJLPA.ZYIZOEME4WTRWUI6KV6VCFJIHBXO5TYXD6JFZS2V4GCCU27FQOQQ")

    // Callback para manejar los mensajes
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

    // Crear y conectar el cliente MQTT
    client := mqtt.NewClient(opts)
    if token := client.Connect(); token.Wait() && token.Error() != nil {
        log.Fatalf("Error connecting to MQTT broker: %v", token.Error())
    }

    // Suscribirse al tópico
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

func LocationWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to websocket: %v", err)
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		location := locationData
		if err := conn.WriteJSON(location); err != nil {
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

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if err := conn.WriteJSON(testLocation); err != nil {
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
