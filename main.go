package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

var locationData struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func main() {
    // Load environment variables from .env file
    if err := godotenv.Load(); err != nil {
        log.Fatalf("Error loading .env file: %v", err)
    }

    setupMQTT()

    // Configuración del servidor HTTP
    r := mux.NewRouter()
    
    // Rutas
    r.HandleFunc("/", HomeHandler).Methods("GET")
    r.HandleFunc("/api/location", GetLocationHandler).Methods("GET")

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
    opts.AddBroker(os.Getenv("MQTT_BROKER"))
    opts.SetClientID("go_mqtt_client")
    opts.SetUsername(os.Getenv("MQTT_USERNAME"))
    opts.SetPassword(os.Getenv("MQTT_PASSWORD"))

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
    if token := client.Subscribe(os.Getenv("MQTT_TOPIC"), 0, nil); token.Wait() && token.Error() != nil {
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

func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Printf("%s %s", r.Method, r.RequestURI)
        next.ServeHTTP(w, r)
    })
}
