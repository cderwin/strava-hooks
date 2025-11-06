package app

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/tkrajina/gpxgo/gpx"
)

const (
	xsiSchemaLoc = "http://www.topografix.com/GPX/1/1 http://www.topografix.com/GPX/1/1/gpx.xsd http://www.garmin.com/xmlschemas/GpxExtensions/v3 http://www.garmin.com/xmlschemas/GpxExtensionsv3.xsd http://www.garmin.com/xmlschemas/TrackPointExtension/v1 http://www.garmin.com/xmlschemas/TrackPointExtensionv1.xsd"
)

type GpxConfig struct {
	Name           string
	Type           string
	Time           time.Time
	UseHeartRate   bool
	UseTemperature bool
}

func BuildGpx(StreamPoints []StravaStreamPoint, Config GpxConfig) (gpx.GPX, error) {
	xmlNsAttrs := []xml.Attr{{Name: xml.Name{Space: "xmlns", Local: "gpxtpx"}, Value: "http://www.garmin.com/xmlschemas/TrackPointExtension/v1"}}

	trackSegment := gpx.GPXTrackSegment{}
	for _, streamPoint := range StreamPoints {
		point := gpx.Point{Latitude: streamPoint.Latitude, Longitude: streamPoint.Longitude, Elevation: *gpx.NewNullableFloat64(streamPoint.Altitude)}
		extension := gpx.Extension{}
		if Config.UseHeartRate {
			name := xml.Name{Space: "gpxtpx", Local: "hr"}
			node := gpx.ExtensionNode{XMLName: name, Data: fmt.Sprintf("%f", streamPoint.HeartRate)}
			extension.Nodes = append(extension.Nodes, node)
		}

		if Config.UseTemperature {
			name := xml.Name{Space: "gpxtpx", Local: "atemp"}
			node := gpx.ExtensionNode{XMLName: name, Data: fmt.Sprintf("%f", streamPoint.HeartRate)}
			extension.Nodes = append(extension.Nodes, node)
		}

		gpxPoint := gpx.GPXPoint{Point: point, Timestamp: time.Unix(streamPoint.Time, 0), Extensions: extension}
		trackSegment.AppendPoint(&gpxPoint)
	}

	gpxTrack := gpx.GPXTrack{Name: Config.Name, Type: Config.Type, Segments: []gpx.GPXTrackSegment{trackSegment}}
	gpx := gpx.GPX{XmlSchemaLoc: xsiSchemaLoc, Attrs: gpx.NewGPXAttributes(xmlNsAttrs), Version: "1.1", Creator: "strava-hooks.fly.dev", Time: &Config.Time, Tracks: []gpx.GPXTrack{gpxTrack}}
	return gpx, nil
}
