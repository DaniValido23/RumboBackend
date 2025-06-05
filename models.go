package main

import "sync"

var carLocations = make(map[string]Point)
var carLocationsMutex sync.RWMutex

var allStreets []StreetData
var allStreetsMutex sync.RWMutex

type GeoJSONGeometry struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

type GeoJSONProperties struct {
	ID_TRC   int    `json:"ID_TRC"`
	TYP_VOIE string `json:"TYP_VOIE"`
	NOM_VOIE string `json:"NOM_VOIE"`
	ODONYME  string `json:"ODONYME"`
}

type GeoJSONFeature struct {
	Type       string            `json:"type"`
	Geometry   GeoJSONGeometry   `json:"geometry"`
	Properties GeoJSONProperties `json:"properties"`
	Capacity   int               `json:"capacity"`
}

type GeoJSONFeatureCollection struct {
	Type     string           `json:"type"`
	Name     string           `json:"name"`
	Features []GeoJSONFeature `json:"features"`
}

type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type StreetInfo struct {
	ID_TRC     int     `json:"id_trc"`
	StreetName string  `json:"streetName"`
	Polylines  []Point `json:"polylines"`
}

type StreetData struct {
	ID            int     `json:"id"`
	StreetName    string  `json:"street_name"`
	Occupancy     int     `json:"occupancy"`
	Capacity      int     `json:"capacity"`
	LastOccupancy int     `json:"-"`
	Polylines     []Point `json:"polylines"`
}

type StreetUpdateMessage struct {
	ODONYME    string `json:"ODONYME"`
	ID_TRC     int    `json:"ID_TRC"`
	CarsNumber int    `json:"CarsNumber"`
	HexColor   string `json:"HexColor"`
}
