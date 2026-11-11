package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/adeithe/go-twitch/api"
	"github.com/joeyak/go-twitch-eventsub/v3"
)

type EventSubService struct {
	mu        sync.Mutex
	once      sync.Once
	ids       sync.Map
	listeners map[string]*Listener
	client    *twitch.Client
	sessionId string
}

type Listener struct {
	broadcaster *Broadcaster
	// ch          chan twitch.EventChannelChannelPointsCustomRewardRedemptionAdd
	callback func(event twitch.EventChannelChannelPointsCustomRewardRedemptionAdd)
}

func NewEventSubService() *EventSubService {
	return &EventSubService{listeners: make(map[string]*Listener)}
}

func (e *EventSubService) InitService() {
	e.mu.Lock()
	// defer e.mu.Unlock()

	events := twitch.NewClient()

	events.OnError(func(err error) {
		log.Printf("main: error twitch eventsub: %v\n", err)
	})

	events.OnWelcome(func(message twitch.WelcomeMessage) {
		log.Printf("eventsub: received welcome message")
		e.sessionId = message.Payload.Session.ID
		log.Println("eventsub: connected")

		e.mu.Unlock() // FIXME: dangerous? hacky? who knows
	})

	events.OnEventChannelChannelPointsCustomRewardRedemptionAdd(func(event twitch.EventChannelChannelPointsCustomRewardRedemptionAdd) {
		if ln, ok := e.listeners[event.BroadcasterUserId]; ok {
			ln.callback(event)
		}
	})

	e.client = events
	go e.client.Connect()
}

func (e *EventSubService) Add(broadcaster *Broadcaster, hc *http.Client, client *api.Client, callback func(event twitch.EventChannelChannelPointsCustomRewardRedemptionAdd)) error {
	e.once.Do(e.InitService)
	e.mu.Lock()
	defer e.mu.Unlock()

	ln := &Listener{callback: callback, broadcaster: broadcaster}
	e.listeners[broadcaster.UserID] = ln

	log.Printf("eventsub: subscribe request with session_id: %s\n", e.sessionId)

	// go-twitch-eventsub doesn't let us pass in a custom http client
	// so the access token can expire
	//
	// we perform the subscribe request ourselves using the oauth2 http client
	// req := twitch.SubscribeRequest{
	// 	SessionID:   e.sessionId,
	// 	ClientID:    conf.ClientID,
	// 	AccessToken: tok.AccessToken,
	// 	Event:       twitch.SubChannelChannelPointsCustomRewardRedemptionAdd,
	// 	Condition:   map[string]string{"broadcaster_user_id": broadcaster.UserID},
	// }

	b, err := json.Marshal(twitch.SubscriptionRequest{
		Type:      twitch.SubChannelChannelPointsCustomRewardRedemptionAdd,
		Version:   "1",
		Condition: map[string]string{"broadcaster_user_id": broadcaster.UserID},
		Transport: twitch.SubscriptionTransport{
			Method:    "websocket",
			SessionID: e.sessionId,
		},
	})
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(b)
	req, err := http.NewRequest(http.MethodPost, "https://api.twitch.tv/helix/eventsub/subscriptions", buf)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Client-Id", conf.ClientID)
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 202 {
		return fmt.Errorf("could not subscribe to event: %s: %s", resp.Status, string(body))
	}

	m := map[string]any{}
	err = json.Unmarshal(body, &m)
	if err != nil {
		return err
	}

	id := m["data"].([]any)[0].(map[string]any)["id"].(string)
	_, ok := e.ids.LoadOrStore(broadcaster.UserID, id)
	if ok {
		return fmt.Errorf("could not add to eventsub: already stored in ids")
	}

	// // enable existing custom reward
	// call := client.ChannelPoints.Rewards.Modify(broadcaster.RewardID, broadcaster.UserID)
	// call.IsEnabled(true)

	// _, err = call.Do(context.Background())
	// if err != nil {
	// 	log.Printf("clean: error twitch api: %v\n", err)
	// 	return err
	// }

	return nil
}

func (e *EventSubService) Del(broadcaster *Broadcaster, hc *http.Client, client *api.Client) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// disable existing custom reward
	call := client.ChannelPoints.Rewards.Modify(broadcaster.RewardID, broadcaster.UserID)
	call.IsEnabled(false)

	_, err := call.Do(context.Background())
	if err != nil {
		log.Printf("eventsub: error twitch api: %v\n", err)
		return err
	}

	// delete subscription
	id, ok := e.ids.LoadAndDelete(broadcaster.UserID)
	if !ok {
		return fmt.Errorf("could not remove from eventsub: not stored in ids")
	}

	req, err := http.NewRequest(http.MethodDelete, "https://api.twitch.tv/helix/eventsub/subscriptions", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Client-Id", conf.ClientID)

	q := req.URL.Query()
	q.Add("id", id.(string))
	req.URL.RawQuery = q.Encode()

	resp, err := hc.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 204 {
		return fmt.Errorf("could not unsubscribe from event: %s: %s", resp.Status, string(body))
	}

	delete(e.listeners, broadcaster.UserID)
	return nil
}
