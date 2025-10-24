package app

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"io"

	"github.com/labstack/echo/v4"
)

const (
	subscriptionsUrl = "https://www.strava.com/api/v3/push_subscriptions"
)

type PushEvent struct {
	ObjectType string `json:"object_type"`
	ObjectId string `json:"object_id"`
	AspectType string `json:"aspect_type"`
	Updates map[string]bool `json:"updates"`
	OwnerId int `json:"owner_id"`
	SubscriptionId int `json:"subscription_id"`
	EventTime int `json:"event_time"`
}

type SubscriptionsResponse struct {
	Id int `json:"id"`
}


func (s *ServerState) handleSubscriptionCallback(c echo.Context) error {
	if c.QueryParam("hub.verify_token") != s.config.VerifyToken {
		slog.Warn("received subscription callback with incorrect verify_token")
		return echo.NewHTTPError(http.StatusBadRequest, "hub.verify_token is incorrect")
	}

	response := struct {
		ChallengeToken string `json:"hub.challenge"`
	}{ChallengeToken: c.QueryParam("hub.challenge")}
	c.JSON(http.StatusOK, response)
	return nil
}

func handlePushEvent(c echo.Context) error {
	var event PushEvent
	c.Bind(&event)
	switch event.ObjectType {
	case "activity":
		slog.Info("webhook received: activity update", "athlete_id", event.OwnerId, "activity_id", event.ObjectId)
	case "athlete":
		slog.Info("webhook received: athlete revoked access", "athlete_id", event.OwnerId)
	default:
		slog.Warn("webhook received: unrecognized object type", "object_type", event.ObjectType)
	}

	return nil
}

func EstablishSubscriptions(config *Config) {
	slog.Info("fetching current subscription info")
	subscriptionsUrlBuilder, err := url.Parse(subscriptionsUrl)
	if err != nil {
		slog.Error("error fetching subscription: failed to parse url", "subscriptions_url", subscriptionsUrl)
		return
	}
	queryParams := subscriptionsUrlBuilder.Query()
	queryParams.Add("client_id", config.StravaClientId)
	queryParams.Add("client_secret", config.StravaClientSecret)
	subscriptionsUrlBuilder.RawQuery = queryParams.Encode()
	resp, err := http.Get(subscriptionsUrlBuilder.String())
	if err != nil {
		slog.Error("error fetching subscription: http request failed", "err", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Error("Error fetching subscription: reading body failed", "err", err)
		}

		var currentSubscriptions []SubscriptionsResponse
		err = json.Unmarshal(body, &currentSubscriptions)
		if err != nil {
			slog.Error("error fetching subscription: decoding response failed", "err", err, "body", string(body))
			return
		}

		if len(currentSubscriptions) > 0 {
			slog.Info("fetched current subscription", "subscription_id", currentSubscriptions[0].Id)
			return
		}
	} else {
		slog.Warn("error fetching subscription: non-200 status code", "status_code", resp.StatusCode)
	}

	slog.Info("no existing subscription found, will attempt to create one")
	formData := url.Values{}
	formData.Add("client_id", config.StravaClientId)
	formData.Add("client_secret", config.StravaClientSecret)
	formData.Add("callback_url", fmt.Sprintf("%s/subscriptions/callback", config.BaseUrl))
	formData.Add("verify_token", config.VerifyToken)
	resp, err = http.PostForm(subscriptionsUrl, formData)
	if err != nil {
		slog.Error("error creating subscription: http request failed", "err", err)
		return
	}
	defer resp.Body.Close()

	var newSubscription SubscriptionsResponse
	err = json.NewDecoder(resp.Body).Decode(&newSubscription)
	if err != nil {
		slog.Error("error creating subscription: decoding response failed", "err", err)
		return
	}

	slog.Info("created new subscription", "subscription_id", newSubscription.Id)
}
