package app

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tkrajina/gpxgo/gpx"
)

const (
	ActivityUrl  = "https://www.strava.com/api/v3/activities/%s"
	StreamsUrl   = "https://www.strava.com/api/v3/activities/%s/streams?keys=latlng,altitude,time"
	xsiSchemaLoc = "http://www.topografix.com/GPX/1/1 http://www.topografix.com/GPX/1/1/gpx.xsd http://www.garmin.com/xmlschemas/GpxExtensions/v3 http://www.garmin.com/xmlschemas/GpxExtensionsv3.xsd http://www.garmin.com/xmlschemas/TrackPointExtension/v1 http://www.garmin.com/xmlschemas/TrackPointExtensionv1.xsd"
)

var (
	DebugSerializeHTTPResponse = false
)

type StravaActivity struct {
	Id      int `json:"id"`
	Athlete struct {
		Id int `json:"id"`
	} `json:"athlete"`
	Name           string     `json:"name"`
	Distance       float64    `json:"distance"`
	MovingTime     int        `json:"moving_time"`
	ElapsedTime    int        `json:"elapsed_time"`
	ElevationGain  float32    `json:"total_elevation_gain"`
	Type           string     `json:"type"`
	StartDate      string     `json:"start_date"`
	StartLatLon    [2]float64 `json:"start_latlng"`
	EndLatLon      [2]float64 `json:"end_latlng"`
	Description    string     `json:"description"`
	Calories       float64    `json:"calories"`
	RelativeEffort float64    `json:"suffer_score"`
}

type StravaStreamPoint struct {
	Time        float64
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

type RawStream struct {
	Type         string          `json:"type"`
	Data         json.RawMessage `json:"data"`
	OriginalSize int             `json:"original_size"`
}

type StravaClient struct {
	client http.Client
	Token  string
}

func NewStravaClient(token string) StravaClient {
	debug := os.Getenv("DEBUG_STRAVA_RESPONSE_BODY")
	if debug != "" && strings.ToLower(debug) != "false" {
		DebugSerializeHTTPResponse = true
	}

	return StravaClient{
		client: http.Client{},
		Token:  token,
	}
}

func (c *StravaClient) GetActivity(activityId string) (StravaActivity, error) {
	url := fmt.Sprintf(ActivityUrl, activityId)
	body, err := c.performRequest("GET", url, nil)
	if err != nil {
		return StravaActivity{}, fmt.Errorf("error fetching activity: %w", err)
	}

	var activity StravaActivity
	json.NewDecoder(body).Decode(&activity)
	return activity, nil
}

func (c *StravaClient) DownloadActivity(activityId string, path string, metadata GpxMetadata) error {
	streamPoints, err := c.getActivityStream(activityId)
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

func (c *StravaClient) getActivityStream(activityId string) ([]StravaStreamPoint, error) {
	url := fmt.Sprintf(StreamsUrl, activityId)
	body, err := c.performRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error fetching activity streams: %w", err)
	}

	var activityStreams []RawStream
	err = json.NewDecoder(body).Decode(&activityStreams)
	if err != nil {
		return nil, fmt.Errorf("error decoding streams: %w", err)
	}

	originalSize := activityStreams[0].OriginalSize
	streamPoints := make([]StravaStreamPoint, originalSize)
	for _, rawStream := range activityStreams {
		var streamData any
		err := json.Unmarshal(rawStream.Data, &streamData)
		if err != nil {
			return nil, fmt.Errorf("error decoding stream data: %w", err)
		}

		streamLength := len(streamData.([]any))
		if streamLength != originalSize {
			return nil, fmt.Errorf("error validating stream \"%s\": size (%d) does not match size of first stream (%d)", rawStream.Type, streamLength, originalSize)
		}

		if streamLength != rawStream.OriginalSize {
			return nil, fmt.Errorf("error validating stream \"%s\": size (%d) does not match original_size metadata (%d)", rawStream.Type, streamLength, rawStream.OriginalSize)
		}

		for i, item := range streamData.([]any) {
			if rawStream.Type == "latlng" {
				latLngData := item.([]any)
				streamPoints[i].Latitude = latLngData[0].(float64)
				streamPoints[i].Longitude = latLngData[1].(float64)
			} else {
				switch rawStream.Type {
				case "distance":
					streamPoints[i].Distance = item.(float64)
				case "time":
					streamPoints[i].Time = item.(float64)
				case "altitude":
					streamPoints[i].Altitude = item.(float64)
				case "heartrate":
					streamPoints[i].HeartRate = item.(float64)
				case "temp":
					streamPoints[i].Temperature = item.(float64)
				default:
					return nil, fmt.Errorf("unrecognized stream type: %s", rawStream.Type)
				}
			}
		}
	}
	return streamPoints, nil
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

		timestamp := time.Unix(int64(streamPoint.Time), int64(streamPoint.Time*1_000_000_000))
		gpxPoint := gpx.GPXPoint{Point: point, Timestamp: timestamp, Extensions: extension}
		trackSegment.AppendPoint(&gpxPoint)
	}

	gpxTrack := gpx.GPXTrack{Name: metadata.Name, Type: metadata.Type, Segments: []gpx.GPXTrackSegment{trackSegment}}
	gpx := gpx.GPX{XmlSchemaLoc: xsiSchemaLoc, Attrs: gpx.NewGPXAttributes(xmlNsAttrs), Version: "1.1", Creator: "strava-hooks.fly.dev", Time: &metadata.Time, Tracks: []gpx.GPXTrack{gpxTrack}}
	return gpx, nil
}

func (c *StravaClient) performRequest(method string, url string, body io.Reader) (io.Reader, error) {
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	request.Header["Authorization"] = []string{fmt.Sprintf("Bearer %s", c.Token)}
	response, err := c.client.Do(request)
	if err != nil {
		slog.Error("unknown http exception", "method", method, "url", url, "err", err)
		return nil, fmt.Errorf("http request failed: unknown error: %w", err)
	}

	// saves response body to file for debugging when flag is set
	var bodyReader io.Reader = response.Body
	if DebugSerializeHTTPResponse {
		bodyPath := "debug.txt"
		slog.Debug("serializing response body for debugging", "method", method, "url", url, "body_path", bodyPath)

		body, err := io.ReadAll(response.Body)
		if err != nil {
			slog.Error("http response body failed on read", "method", method, "url", url, "err", err)
			return nil, fmt.Errorf("http request failed, error reading body: %w", err)
		}

		os.WriteFile(bodyPath, body, 0644)
		bodyReader = bytes.NewReader(body)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		slog.Error("http response received with bad status_code", "method", method, "url", url, "status_code", response.StatusCode)
		return nil, fmt.Errorf("http request failed, invalid status %d", response.StatusCode)
	}

	return bodyReader, nil
}
