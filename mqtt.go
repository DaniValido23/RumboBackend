package main

import (
	"encoding/json"
	"log"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func setupMQTT() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://eu1.cloud.thethings.industries:1883")
	opts.SetClientID("go_mqtt_client")
	opts.SetUsername("gps-3a@parallel")
	opts.SetPassword("NNSXS.UYPEF3S77QZ7WFOPUTE3QWTFNBDIS7LPTRLJLPA.ZYIZOEME4WTRWUI6KV6VCFJIHBXO5TYXD6JFZS2V4GCCU27FQOQQ")

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

	topic := "v3/gps-3a@parallel/devices/+/location/solved"
	if token := client.Subscribe(topic, 0, nil); token.Wait() && token.Error() != nil {
		log.Fatalf("Error subscribing to topic %s: %v", topic, token.Error())
	}
	log.Printf("Subscribed to MQTT topic: %s", topic)
}
