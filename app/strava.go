package app

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/tkrajina/gpxgo/gpx"
)

const (
	ActivityUrl  = "https://www.strava.com/api/v3/activities/%s"
	StreamsUrl   = "https://www.strava.com/api/v3/activities/%s/streams?keys=latlng,altitude,time"
	xsiSchemaLoc = "http://www.topografix.com/GPX/1/1 http://www.topografix.com/GPX/1/1/gpx.xsd http://www.garmin.com/xmlschemas/GpxExtensions/v3 http://www.garmin.com/xmlschemas/GpxExtensionsv3.xsd http://www.garmin.com/xmlschemas/TrackPointExtension/v1 http://www.garmin.com/xmlschemas/TrackPointExtensionv1.xsd"
)

type StravaActivity struct {
	Id             int        `json:"id"`
	AthleteId      int        `json:"athlete.id"`
	Name           string     `json:"name"`
	Distance       int        `json:"distance"`
	MovingTime     int        `json:"moving_time"`
	ElapsedTime    int        `json:"elapsed_time"`
	ElevationGain  float32    `json:"total_elevation_gain"`
	Type           string     `json:"type"`
	StartDate      string     `json:"start_date"`
	StartLatLon    [2]float32 `json:"start_latlng"`
	EndLatLon      [2]float32 `json:"end_latlng"`
	Description    string     `json:"description"`
	Calories       float32    `json:"calories"`
	RelativeEffort float32    `json:"suffer_score"`
}

type StravaStreamPoint struct {
	Time        int64
	Latitude    float64
	Longitude   float64
	Altitude    float64
	Distance    float64
	HeartRate   float64
	Temperature float64
}

type GpxMetadata struct {
	Name           string
	Type           string
	Time           time.Time
	UseHeartRate   bool
	UseTemperature bool
}

func GetActivity(ActivityId string, Token string) (StravaActivity, error) {
	url := fmt.Sprintf(ActivityUrl, ActivityId)
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return StravaActivity{}, err
	}

	request.Header["Authorization"] = []string{fmt.Sprintf("Bearer %s", Token)}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return StravaActivity{}, err
	}

	var activity StravaActivity
	json.NewDecoder(response.Body).Decode(&activity)
	return activity, nil
}

type singleStreamResponse struct {
	Type         string    `json:"type"`
	Data         []float64 `json:"data"`
	OriginalSize int       `json:"original_size"`
}

type streamsResponse struct {
	Time        *singleStreamResponse
	Distance    *singleStreamResponse
	Altitude    *singleStreamResponse
	LatLng      *singleStreamResponse
	HeartRate   *singleStreamResponse
	Temperature *singleStreamResponse
	Size        int
}

func DownloadActivity(activityId string, token string, path string, metadata GpxMetadata) error {
	streamPoints, err := getActivityStream(activityId, token)
	if err != nil {
		return err
	}

	gpxDoc, err := buildGpx(streamPoints, metadata)
	bytes, err := gpxDoc.ToXml(gpx.ToXmlParams{})
	if err != nil {
		return err
	}

	err = os.WriteFile(path, bytes, 0644)
	return err
}

func getActivityStream(ActivityId string, Token string) ([]StravaStreamPoint, error) {
	url := fmt.Sprintf(StreamsUrl, ActivityId)
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	request.Header["Authorization"] = []string{fmt.Sprintf("Bearer %s", Token)}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}

	var activityStreams []singleStreamResponse
	err = json.NewDecoder(response.Body).Decode(&activityStreams)
	if err != nil {
		return nil, err
	}

	var streams streamsResponse
	streams.Size = activityStreams[0].OriginalSize
	for _, singleStream := range activityStreams {
		// verify sizes of other streams
		if singleStream.OriginalSize != streams.Size {
			return nil, fmt.Errorf("error validating stream \"%s\": size (%d) does not match size of first stream (%d)", singleStream.Type, singleStream.OriginalSize, streams.Size)
		}

		if len(singleStream.Data) != streams.Size {
			return nil, fmt.Errorf("error validating stream \"%s\": size (%d) does not match original_size metadata (%d)", singleStream.Type, len(singleStream.Data), singleStream.OriginalSize)
		}

		switch singleStream.Type {
		case "distance":
			streams.Distance = &singleStream
		case "time":
			streams.Time = &singleStream
		case "latlng":
			streams.LatLng = &singleStream
		case "altitude":
			streams.Altitude = &singleStream
		case "heartrate":
			streams.HeartRate = &singleStream
		case "temp":
			streams.Temperature = &singleStream
		default:
			return nil, fmt.Errorf("unrecognized stream type: %s", singleStream.Type)
		}
	}

	return streamPointsFromResponse(streams), nil
}

func streamPointsFromResponse(streams streamsResponse) []StravaStreamPoint {
	streamPoints := make([]StravaStreamPoint, streams.Size)
	for i := range streams.Size {
		streamPoints[i].Time = int64(streams.Time.Data[i])
		streamPoints[i].Distance = float64(streams.Distance.Data[i])
		streamPoints[i].Altitude = float64(streams.Altitude.Data[i])
		streamPoints[i].HeartRate = float64(streams.HeartRate.Data[i])
		streamPoints[i].Temperature = float64(streams.Temperature.Data[i])
		streamPoints[i].Latitude = float64(streams.LatLng.Data[i])
		streamPoints[i].Longitude = float64(streams.LatLng.Data[i])
	}
	return streamPoints
}

func buildGpx(StreamPoints []StravaStreamPoint, metadata GpxMetadata) (gpx.GPX, error) {
	xmlNsAttrs := []xml.Attr{{Name: xml.Name{Space: "xmlns", Local: "gpxtpx"}, Value: "http://www.garmin.com/xmlschemas/TrackPointExtension/v1"}}

	trackSegment := gpx.GPXTrackSegment{}
	for _, streamPoint := range StreamPoints {
		point := gpx.Point{Latitude: streamPoint.Latitude, Longitude: streamPoint.Longitude, Elevation: *gpx.NewNullableFloat64(streamPoint.Altitude)}
		extension := gpx.Extension{}
		if metadata.UseHeartRate {
			name := xml.Name{Space: "gpxtpx", Local: "hr"}
			node := gpx.ExtensionNode{XMLName: name, Data: fmt.Sprintf("%f", streamPoint.HeartRate)}
			extension.Nodes = append(extension.Nodes, node)
		}

		if metadata.UseTemperature {
			name := xml.Name{Space: "gpxtpx", Local: "atemp"}
			node := gpx.ExtensionNode{XMLName: name, Data: fmt.Sprintf("%f", streamPoint.HeartRate)}
			extension.Nodes = append(extension.Nodes, node)
		}

		gpxPoint := gpx.GPXPoint{Point: point, Timestamp: time.Unix(streamPoint.Time, 0), Extensions: extension}
		trackSegment.AppendPoint(&gpxPoint)
	}

	gpxTrack := gpx.GPXTrack{Name: metadata.Name, Type: metadata.Type, Segments: []gpx.GPXTrackSegment{trackSegment}}
	gpx := gpx.GPX{XmlSchemaLoc: xsiSchemaLoc, Attrs: gpx.NewGPXAttributes(xmlNsAttrs), Version: "1.1", Creator: "strava-hooks.fly.dev", Time: &metadata.Time, Tracks: []gpx.GPXTrack{gpxTrack}}
	return gpx, nil
}
