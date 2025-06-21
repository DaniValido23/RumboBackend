package main

import (
	"encoding/json"
	"log"
	"math"
	"os"
	"strings"

	"github.com/dhconnelly/rtreego"
)

var streetIndex *rtreego.Rtree

type indexedStreet struct {
	sd       *StreetData
	envelope rtreego.Rect
}

func (is *indexedStreet) Bounds() rtreego.Rect {
	return is.envelope
}

func loadStreetData() {
	file, err := os.Open("geobase.json")
	if err != nil {
		log.Printf("Error opening geobase.json: %v", err)
		return
	}
	defer file.Close()

	var geojsonCollection GeoJSONFeatureCollection
	if err := json.NewDecoder(file).Decode(&geojsonCollection); err != nil {
		log.Printf("Error decoding geobase.json: %v", err)
		return
	}

	allStreetsMutex.Lock()
	defer allStreetsMutex.Unlock()

	allStreets = []StreetData{}
	streetIndex = rtreego.NewTree(2, 25, 50)
	for _, feature := range geojsonCollection.Features {
		if feature.Geometry.Type == "LineString" {
			var polylines []Point
			for _, coord := range feature.Geometry.Coordinates {
				if len(coord) == 2 {
					polylines = append(polylines, Point{Lat: coord[1], Lng: coord[0]})
				}
			}

			street := StreetData{
				ID:            feature.Properties.ID_TRC,
				StreetName:    strings.TrimSpace(feature.Properties.ODONYME),
				Polylines:     polylines,
				Occupancy:     0,
				Capacity:      feature.Capacity,
				LastOccupancy: -1,
			}
			allStreets = append(allStreets, street)
			minLat, minLng := polylines[0].Lat, polylines[0].Lng
			maxLat, maxLng := minLat, minLng
			for _, pt := range polylines {
				if pt.Lat < minLat {
					minLat = pt.Lat
				}
				if pt.Lng < minLng {
					minLng = pt.Lng
				}
				if pt.Lat > maxLat {
					maxLat = pt.Lat
				}
				if pt.Lng > maxLng {
					maxLng = pt.Lng
				}
			}
			rect, _ := rtreego.NewRect(rtreego.Point{minLat, minLng}, []float64{maxLat - minLat, maxLng - minLng})
			streetIndex.Insert(&indexedStreet{sd: &allStreets[len(allStreets)-1], envelope: rect})
		}
	}
	log.Printf("Loaded %d streets from geobase.json", len(allStreets))
}

func recalculateStreetOccupancy(currentLocations map[string]Point) []int {
	log.Printf("Recalculating street occupancy for %d cars...", len(currentLocations))
	newOccupancy := make(map[int]int)
	affectedStreetIDs := make(map[int]bool)

	allStreetsMutex.RLock()
	streetsSnapshot := make([]StreetData, len(allStreets))
	copy(streetsSnapshot, allStreets)
	allStreetsMutex.RUnlock()

	for _, location := range currentLocations {
		if location.Lat == 0.0 && location.Lng == 0.0 {
			continue
		}
		nearestStreetInfo := findNearestStreetInSnapshot(location.Lat, location.Lng, streetsSnapshot)
		if nearestStreetInfo.StreetName != "" {
			if nearestStreetInfo.ID_TRC != 0 {
				newOccupancy[nearestStreetInfo.ID_TRC]++
				affectedStreetIDs[nearestStreetInfo.ID_TRC] = true
			}
		}
	}

	allStreetsMutex.Lock()
	for i := range allStreets {
		oldOccupancy := allStreets[i].Occupancy
		streetID := allStreets[i].ID
		allStreets[i].Occupancy = newOccupancy[streetID]
		if oldOccupancy != allStreets[i].Occupancy {
			affectedStreetIDs[streetID] = true
		}
	}
	allStreetsMutex.Unlock()

	log.Println("Street occupancy recalculated.")

	var ids []int
	for id := range affectedStreetIDs {
		ids = append(ids, id)
	}
	return ids
}

func findNearestStreetInSnapshot(lat, lng float64, _ []StreetData) StreetInfo {
	if streetIndex == nil {
		log.Printf("Spatial index not initialized, fallback to brute force")
		return StreetInfo{}
	}
	originPt := rtreego.Point{lat, lng}
	const k = 5
	results := streetIndex.NearestNeighbors(k, originPt)
	if len(results) == 0 {
		log.Printf("Warning: No street envelope found near point (%f, %f)", lat, lng)
		return StreetInfo{}
	}
	origin := Point{Lat: lat, Lng: lng}
	minDist := math.MaxFloat64
	var nearest StreetInfo
	for _, item := range results {
		is := item.(*indexedStreet)
		for _, pt := range is.sd.Polylines {
			d := calculateDistance(origin, pt)
			if d < minDist {
				minDist = d
				nearest = StreetInfo{
					ID_TRC:     is.sd.ID,
					StreetName: is.sd.StreetName,
					Polylines:  is.sd.Polylines}
			}
		}
	}
	return nearest
}

func calculateDistance(p1, p2 Point) float64 {
	const R = 6371000
	lat1Rad := p1.Lat * math.Pi / 180
	lat2Rad := p2.Lat * math.Pi / 180

	deltaLat := lat2Rad - lat1Rad/180
	deltaLng := p2.Lng - p1.Lng/180

	// Haversine formula
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLng/2)*math.Sin(deltaLng/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	distance := R * c
	return distance
}

func calculateColorFromOccupancy(occupancy int) string {
	if occupancy > 5 {
		occupancy = 5
	}

	percentage := float64(occupancy) / 5.0 * 100.0

	switch {
	case percentage == 0:
		return "#00FF00"
	case percentage <= 25:
		return "#7FFF00"
	case percentage <= 50:
		return "#FFFF00"
	case percentage <= 75:
		return "#FF7F00"
	default:
		return "#FF0000"
	}
}
