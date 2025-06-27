package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func setupMQTT() {
	broker := os.Getenv("MQTT_BROKER")
	if broker == "" {
		log.Fatal("MQTT_BROKER environment variable not set")
	}
	username := os.Getenv("MQTT_USERNAME")
	if username == "" {
		log.Fatal("MQTT_USERNAME environment variable not set")
	}
	password := os.Getenv("MQTT_PASSWORD")
	if password == "" {
		log.Fatal("MQTT_PASSWORD environment variable not set")
	}
	topic := os.Getenv("MQTT_TOPIC")
	if topic == "" {
		log.Fatal("MQTT_TOPIC environment variable not set")
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID("go_mqtt_client")
	opts.SetUsername(username)
	opts.SetPassword(password)

	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		log.Printf("Received message from topic: %s\n", msg.Topic())

		topicParts := strings.Split(msg.Topic(), "/")
		var carID string
		if len(topicParts) >= 4 && topicParts[2] == "devices" {
			carID = topicParts[3]
		} else {
			log.Printf("Could not extract Car ID from topic: %s", msg.Topic())
			return
		}

		var payload struct {
			LocationSolved struct {
				Location struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				} `json:"location"`
			} `json:"location_solved"`
		}
		if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
			log.Printf("Error unmarshalling payload for car %s: %v", carID, err)
			return
		}

		newLocation := Point{
			Lat: payload.LocationSolved.Location.Latitude,
			Lng: payload.LocationSolved.Location.Longitude,
		}

		carLocationsMutex.Lock()
		carLocations[carID] = newLocation
		currentLocationsCopy := make(map[string]Point, len(carLocations))
		for k, v := range carLocations {
			currentLocationsCopy[k] = v
		}
		carLocationsMutex.Unlock()

		log.Printf("New location received for car %s - Latitude: %f, Longitude: %f", carID, newLocation.Lat, newLocation.Lng)

		affectedIDs := recalculateStreetOccupancy(currentLocationsCopy)
		broadcastStreetIDUpdates(affectedIDs)
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Error connecting to MQTT broker: %v", token.Error())
	}

	if token := client.Subscribe(topic, 0, nil); token.Wait() && token.Error() != nil {
		log.Fatalf("Error subscribing to topic %s: %v", topic, token.Error())
	}
	log.Printf("Subscribed to MQTT topic: %s", topic)
}
