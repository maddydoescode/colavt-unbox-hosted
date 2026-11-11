package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/adeithe/go-twitch/api"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joeyak/go-twitch-eventsub/v3"
	"golang.org/x/oauth2"
)

type OverlayConn struct {
	conn *websocket.Conn

	broadcaster *Broadcaster
	rawClient   *http.Client
	client      *api.Client
	redemptions *Redemptions
	eventsub    *EventSubService

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
}

func NewOverlayConn(conf *oauth2.Config, conn *websocket.Conn) (*OverlayConn, error) {
	ctx, cancel := context.WithCancel(context.Background())

	overlayConn := &OverlayConn{conn: conn, ctx: ctx}
	overlayConn.cancel = func() {
		overlayConn.cleanup()
		cancel()
	}

	err := overlayConn.getBroadcasterDetails()
	if err != nil {
		return nil, err
	}

	err = overlayConn.createTwitchClient(conf)
	if err != nil {
		return nil, err
	}

	// get reward id from db and enable reward
	rewardId, err := overlayConn.getBroadcasterRewardId()
	if err != nil {
		return nil, err
	}
	overlayConn.broadcaster.RewardID = rewardId

	overlayConn.redemptions = NewRedemptions(overlayConn.ctx)

	err = overlayConn.setupEventSubService()
	if err != nil {
		return nil, err
	}

	return overlayConn, nil
}

func (c *OverlayConn) callback(event twitch.EventChannelChannelPointsCustomRewardRedemptionAdd) {
	err := c.redemptions.Set(event.ID)
	if err != nil {
		log.Println("event: not accepting events")
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	log.Printf("event: processing event %s\n", event.ID)

	items, err := GetItems(c.broadcaster.UserID)
	if err != nil {
		log.Printf("event: error db get items: %v\n", err)
		// overlayConn.cancel()
		return
	}

	println("trying to write to /ws")

	err = wsjson.Write(c.ctx, c.conn, struct {
		Items     []Item `json:"items"`
		UserLogin string `json:"user_login"`
	}{
		Items:     items,
		UserLogin: event.UserLogin,
	})
	if err != nil {
		log.Printf("event: error wsjson write: %v\n", err)
		// overlayConn.cancel()
		return
	}

	var item Item
	err = wsjson.Read(c.ctx, c.conn, &item)
	if err != nil {
		log.Printf("event: error wsjson read: %v\n", err)
		// overlayConn.cancel()
		return
	}
	log.Printf("ws: received %v\n", err)

	// update users inventory
	inventoryItem := InventoryItem{
		ChannelID: c.broadcaster.UserID,
		ItemID:    item.ID,
		UserID:    event.UserID,
	}
	err = CreateInventoryItem(inventoryItem)
	if err != nil {
		log.Printf("event: error db create inventory item: %v\n", err)
		// overlayConn.cancel()
		return
	}

	// update twitch extension inventory
	err = c.updateExtension(event, inventoryItem)
	if err != nil {
		log.Printf("event: error sending pubsub message: %v\n", err)
	}

	err = c.redemptions.Del(event.ID)
	if err != nil {
		log.Printf("event: process halted %s\n", event.ID)
		return
	}
}

func (c *OverlayConn) updateExtension(
	event twitch.EventChannelChannelPointsCustomRewardRedemptionAdd,
	item InventoryItem,
) error {
	itemJson, err := json.Marshal(item)
	if err != nil {
		return err
	}

	target := []string{"broadcast"}
	log.Printf("pubsub: sending to target %s\n", target)

	claims := jwt.MapClaims{
		"exp":        time.Now().Add(20 * time.Second).Unix(),
		"user_id":    twitchExtOwnerId,
		"role":       "external",
		"channel_id": event.BroadcasterUserId,
		"pubsub_perms": jwt.MapClaims{
			"send": target,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims, nil)
	tokenStr, err := token.SignedString(JWTSigningKey)
	if err != nil {
		return err
	}
	// log.Printf("signed token string: %s\n", tokenStr)

	j, err := json.Marshal(map[string]any{
		"target":         target,
		"broadcaster_id": event.BroadcasterUserId,
		"message":        string(itemJson),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		c.ctx, "POST", "https://api.twitch.tv/helix/extensions/pubsub", bytes.NewBuffer(j),
	)
	if err != nil {
		log.Printf("event: error create request: %v\n", err)
	}

	req.Header.Add("Client-Id", conf.ClientID)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", tokenStr))
	req.Header.Add("Content-Type", "application/json")

	_, err = http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return nil
}

func (c *OverlayConn) setupEventSubService() error {
	c.eventsub = NewEventSubService()
	err := c.eventsub.Add(c.broadcaster, c.rawClient, c.client, c.callback)
	if err != nil {
		return err
	}
	return nil
}

func (c *OverlayConn) cleanup() {
	err := c.eventsub.Del(c.broadcaster, c.rawClient, c.client)
	if err != nil {
		log.Printf("eventsub: error could not disable reward: %v\n", err)
	}
	log.Println("clean: ws connection ended, clearing oustanding redemptions...")

	ids := c.redemptions.Drain()
	for i, id := range ids {
		call := c.client.ChannelPoints.Redemptions.Modify(id, c.broadcaster.UserID, c.broadcaster.RewardID)
		_, err := call.Do(context.Background(), api.SetQueryParameter("status", "CANCELED"))
		if err != nil {
			log.Printf("clean: err twitch api for id %s: %v\n", id, err)
			continue // or return?
		}

		log.Printf("clean: x %s (%d/%d)\n", id, i, len(ids))
	}

	log.Println("clean: done")
}

func (c *OverlayConn) createTwitchClient(conf *oauth2.Config) error {
	tok, err := GetTokenForID(c.broadcaster.UserID)
	if err != nil {
		return err
	}

	reuse := oauth2.ReuseTokenSource(tok, conf.TokenSource(c.ctx, tok))
	src := &CustomTokenSource{
		TokenSource: reuse,
		accessToken: tok.AccessToken,
		userId:      c.broadcaster.UserID,
	}
	c.rawClient = oauth2.NewClient(c.ctx, src)
	c.client = api.New(conf.ClientID, api.WithHTTPClient(c.rawClient))

	return nil
}

func (c *OverlayConn) getBroadcasterRewardId() (string, error) {
	if c.broadcaster.RewardID != "" {
		log.Printf("eventsub.rewardid: found existing reward: %s\n", c.broadcaster.RewardID)

		// enable existing custom reward
		call := c.client.ChannelPoints.Rewards.Modify(c.broadcaster.RewardID, c.broadcaster.UserID)
		call.IsEnabled(true)

		_, err := call.Do(c.ctx)
		if err != nil {
			log.Printf("eventsub.rewardid: error twitch api: %v\n", err)
			return "", err
		}

		return c.broadcaster.RewardID, nil
	}

	// TODO: check if Unbox Item reward exists (title exists already)
	// TODO: custom fields for the channel point instead of hardcoded
	log.Println("eventsub.rewardid: creating new reward")

	call := c.client.ChannelPoints.Rewards.Insert(c.broadcaster.UserID, "Unbox Item", 1)
	rewards, err := call.Do(c.ctx)
	if err != nil {
		log.Printf("eventsub.rewardid: error twitch api: %v\n", err)
		return "", err
	}

	rewardId := rewards.Data[0].RewardID

	err = UpdateRewardIDForID(c.broadcaster.UserID, rewardId)
	if err != nil {
		log.Printf("eventsub.rewardid: error db update reward id for id %s: %v\n", c.broadcaster.UserID, err)
		return "", err
	}

	return rewardId, nil
}

func (c *OverlayConn) getBroadcasterDetails() error {
	// get broadcasterId from /overlay
	var broadcasterId string
	err := wsjson.Read(c.ctx, c.conn, &broadcasterId)
	if err != nil {
		return err
	}

	// get broadcaster details from db
	broadcaster, err := GetBroadcasterByID(broadcasterId)
	if err != nil {
		return err
	}
	c.broadcaster = broadcaster

	return nil
}

func (c *OverlayConn) Heartbeat(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		err := wsjson.Write(c.ctx, c.conn, "ping")
		if err != nil {
			log.Printf("main: error ws heartbeat: %v\n", err)
			c.conn.Close(websocket.StatusGoingAway, "failed to ping")
			return
		}
	}
}
