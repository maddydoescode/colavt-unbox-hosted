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
	"github.com/joeyak/go-twitch-eventsub"
	"golang.org/x/oauth2"
)

type CustomTokenSource struct {
	oauth2.TokenSource
	accessToken string
	userId      string
}

func (c *CustomTokenSource) Token() (*oauth2.Token, error) {
	tok, err := c.TokenSource.Token()
	if err != nil {
		log.Printf("oauth2: error token source: %v\n", err)
		return nil, err
	}

	isNewToken := false
	if tok.AccessToken != c.accessToken {
		isNewToken = true
		err = UpdateTokenForID(c.userId, tok)
		if err != nil {
			log.Printf("oauth2: error update token for id %s: %v\n", c.userId, err)
			return nil, err
		}
		c.accessToken = tok.AccessToken
	}

	log.Printf(
		"\noauth2 token (is new token: %t)\n|_ access_token: %s\n|_ refresh_token %s\n|_ expiry: %s\n",
		isNewToken,
		tok.AccessToken,
		tok.RefreshToken,
		fmt.Sprintf("%dd %dh %dm %ds", tok.Expiry.Day(), tok.Expiry.Hour(), tok.Expiry.Minute(), tok.Expiry.Second()),
	)

	return tok, nil
}

func heartbeat(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// log.Println("main: pinging client...")
			writeCtx, writeCancel := context.WithCancel(ctx)

			res := "ping"
			err := wsjson.Write(writeCtx, conn, res)
			writeCancel()

			if err != nil {
				log.Printf("main: error ws heartbeat: %v\n", err)
				conn.Close(websocket.StatusGoingAway, "failed to ping")
				return
			}
			// log.Println("main: done")
		case <-ctx.Done():
			return
		}
	}
}

func ws(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("main: error accepting ws conn: %v\n", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(context.Background())
	// ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// heartbeat
	go heartbeat(ctx, conn)

	// get broadcaster id from ws peer
	var bid string
	err = wsjson.Read(ctx, conn, &bid)
	if err != nil {
		log.Printf("main: error wsjson read: %v\n", err)
		return
	}

	// get broadcaster details from db
	broadcaster, err := GetBroadcasterByID(bid)
	if err != nil {
		log.Printf("main: error db get broadcaster by id %s: %v\n", bid, err)
		return
	}

	// get oauth2 token from db
	tok, err := GetTokenForID(broadcaster.UserID)
	if err != nil {
		log.Printf("main: error db get token for id %s: %v\n", broadcaster.UserID, err)
		return
	}

	// create token source that updates the db
	reuse := oauth2.ReuseTokenSource(tok, conf.TokenSource(ctx, tok))
	src := &CustomTokenSource{TokenSource: reuse, accessToken: tok.AccessToken, userId: broadcaster.UserID}
	hc := oauth2.NewClient(ctx, src)

	client := api.New(conf.ClientID, api.WithHTTPClient(hc))

	// prevent multiple requests, we want to display them one by one
	var redemptionsMu sync.Mutex
	redemptions := NewRedemptions(ctx)
	eventsubCallback := func(event twitch.EventChannelChannelPointsCustomRewardRedemptionAdd) {
		err := redemptions.Set(event.ID)
		if err != nil {
			log.Printf("event: not accepting events")
			return
		}

		redemptionsMu.Lock()
		defer redemptionsMu.Unlock()

		log.Printf("event: processing event %s\n", event.ID)

		items, err := GetItems(broadcaster.UserID)
		if err != nil {
			log.Printf("event: error db get items: %v\n", err)
			cancel()
			return
		}
		// send data to ws conn
		err = wsjson.Write(ctx, conn, struct {
			Items     []Item `json:"items"` // ItemSequence []Item `json:"item_sequence"`
			UserLogin string `json:"user_login"`
		}{
			Items:     items, // ItemSequence: itemSequence,
			UserLogin: event.UserLogin,
		})
		if err != nil {
			log.Printf("event: error wsjson write: %v\n", err)
			cancel()
			return
		}

		// TODO: probably a deadline ctx?
		var item Item
		err = wsjson.Read(ctx, conn, &item)
		if err != nil {
			log.Printf("event: error wsjson read: %v\n", err)
			cancel()
			return
		}
		log.Printf("ws: received: %v\n", item)

		// update user inventory
		inventoryItem := InventoryItem{
			ChannelID: broadcaster.UserID,
			ItemID:    item.ID,
			UserID:    event.UserID,
		}
		err = CreateInventoryItem(inventoryItem)
		if err != nil {
			log.Printf("event: error db create inventory item: %v\n", err)
			cancel()
			return
		}

		// if this errors it's not a big deal, the user can refresh
		err = sendPubSubMessage(ctx, event, inventoryItem)
		if err != nil {
			log.Printf("event: error sending pubsub message: %v\n", err)
		}

		err = redemptions.Del(event.ID)
		if err != nil {
			log.Printf("event: process halted %s\n", event.ID)
			return
		}
	}

	rewardId, err := getBroadcasterRewardId(ctx, broadcaster, client)
	if err != nil {
		log.Printf("main: could not get reward id: %v\n", err)
		return
	}
	broadcaster.RewardID = rewardId

	err = eventsubService.Add(broadcaster, hc, client, eventsubCallback)
	if err != nil {
		log.Printf("main: error could not enable reward: %v\n", err)
		return
	}

	// cleanup logic (disable custom reward and cancel outstanding redemptions)
	defer func() {
		err := eventsubService.Del(broadcaster, hc, client)
		if err != nil {
			log.Printf("eventsub: error could not disable reward: %v\n", err)
		}

		log.Println("clean: ws connection ended, clearing outstanding redemptions...")

		// cancel all outstanding redemptions
		ids := redemptions.Drain()
		idsCount := len(ids)
		for _, id := range ids {
			call := client.ChannelPoints.Redemptions.Modify(id, broadcaster.UserID, broadcaster.RewardID)
			_, err := call.Do(context.Background(), api.SetQueryParameter("status", "CANCELED"))
			if err != nil {
				log.Printf("clean: error twitch api for id %s: %v\n", id, err)
				return
			}
			idsCount--
			log.Printf("clean: x %s (%d)\n", id, idsCount)
		}

		log.Println("clean: done")
	}()

	<-ctx.Done()
}

func sendPubSubMessage(ctx context.Context, event twitch.EventChannelChannelPointsCustomRewardRedemptionAdd, item InventoryItem) error {
	itemJson, err := json.Marshal(item)
	if err != nil {
		return err
	}

	target := []string{"broadcast"} // TODO: is this opaque user related? need to test with others
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
		ctx, "POST", "https://api.twitch.tv/helix/extensions/pubsub", bytes.NewBuffer(j),
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

func getBroadcasterRewardId(ctx context.Context, broadcaster *Broadcaster, client *api.Client) (string, error) {
	if broadcaster.RewardID != "" {
		// enable existing custom reward
		call := client.ChannelPoints.Rewards.Modify(broadcaster.RewardID, broadcaster.UserID)
		call.IsEnabled(true)

		_, err := call.Do(ctx)
		if err != nil {
			log.Printf("eventsub.rewardid: error twitch api: %v\n", err)
			return "", err
		}

		return broadcaster.RewardID, nil
	}

	// TODO: check if Unbox Item reward exists (title exists already)
	// TODO: custom fields for the channel point instead of hardcoded
	call := client.ChannelPoints.Rewards.Insert(broadcaster.UserID, "Unbox Item", 1)
	rewards, err := call.Do(ctx)
	if err != nil {
		log.Printf("eventsub.rewardid: error twitch api: %v\n", err)
		return "", err
	}

	rewardId := rewards.Data[0].RewardID

	err = UpdateRewardIDForID(broadcaster.UserID, rewardId)
	if err != nil {
		log.Printf("eventsub.rewardid: error db update reward id for id %s: %v\n", broadcaster.UserID, err)
		return "", err
	}

	return rewardId, nil
}
