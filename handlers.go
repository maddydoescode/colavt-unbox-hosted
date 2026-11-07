package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"text/template"
)

func getItems(w http.ResponseWriter, r *http.Request) {
	channelId, ok := r.Context().Value(ChannelIdKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	item, err := GetItems(channelId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(item)
	w.WriteHeader(http.StatusOK)
}

var validMimeTypes = []string{"image/png", "image/jpeg"}
var validRarityTypes = []string{"common", "uncommon", "rare", "epic", "legendary"}

func postItems(w http.ResponseWriter, r *http.Request) {
	isBroadcaster, ok := r.Context().Value(IsBroadcasterKey).(bool)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if !isBroadcaster {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelId, ok := r.Context().Value(ChannelIdKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 32<<20+512)

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	if len(name) == 0 {
		http.Error(w, "name empty", http.StatusBadRequest)
		return
	}

	description := r.FormValue("description")
	if len(description) == 0 {
		http.Error(w, "description empty", http.StatusBadRequest)
		return
	}

	rarity := r.FormValue("rarity")
	if !slices.Contains(validRarityTypes, rarity) {
		http.Error(w, "invalid rarity type", http.StatusBadRequest)
		return
	}

	imageFile, _, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "no image provided", http.StatusBadRequest)
		return
	}
	defer imageFile.Close()

	imageFileData, err := io.ReadAll(imageFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	imageMimeType := http.DetectContentType(imageFileData)
	if !slices.Contains(validMimeTypes, imageMimeType) {
		http.Error(w, "invalid mimetype", http.StatusBadRequest)
		return
	}
	imageFileExtension := strings.Split(imageMimeType, "/")[1]

	imageFileDataHashed := md5.Sum(imageFileData)
	hashString := fmt.Sprintf("%x", imageFileDataHashed)
	imageFileDir := "data/" + hashString[0:2] + "/" + hashString[2:4] + "/"
	imageFileLocation := imageFileDir + hashString + "." + imageFileExtension

	nameCount, err := CountItemByName(name, channelId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	imageUrlCount, err := CountItemByImageURL(imageFileLocation, channelId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if nameCount != 0 || imageUrlCount != 0 {
		http.Error(w, "name or image already exists", http.StatusBadRequest)
		return
	}

	err = os.MkdirAll(imageFileDir, 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = os.WriteFile("./"+imageFileLocation, imageFileData, 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	newItem := Item{
		ChannelID:   channelId,
		Name:        name,
		Description: description,
		ImageURL:    imageFileLocation,
		Rarity:      rarity,
	}
	err = CreateItem(newItem)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func deleteItems(w http.ResponseWriter, r *http.Request) {
	isBroadcaster, ok := r.Context().Value(IsBroadcasterKey).(bool)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if !isBroadcaster {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("item_id")
	if id == "" {
		http.Error(w, "no item id given", http.StatusBadRequest)
		return
	}

	err := DeleteItem(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getAllInventory(w http.ResponseWriter, r *http.Request) {
	channelId, ok := r.Context().Value(ChannelIdKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	item, err := getAllInventoryItems(channelId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(item)
	w.WriteHeader(http.StatusOK)
}

func getInventory(w http.ResponseWriter, r *http.Request) {
	channelId, ok := r.Context().Value(ChannelIdKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userId, ok := r.Context().Value(UserIdKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	item, err := GetInventoryItems(channelId, userId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(item)
	w.WriteHeader(http.StatusOK)
}

func deleteInventory(w http.ResponseWriter, r *http.Request) {
	isBroadcaster, ok := r.Context().Value(IsBroadcasterKey).(bool)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if !isBroadcaster {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("inventory_item_id")
	if id == "" {
		http.Error(w, "no item id given", http.StatusBadRequest)
		return
	}

	err := DeleteInventoryItem(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// func postInventory(w http.ResponseWriter, r *http.Request) {

// }

// func deleteInventory(w http.ResponseWriter, r *http.Request) {

// }

func getBroadcaster(w http.ResponseWriter, r *http.Request) {
	channelId, ok := r.Context().Value(ChannelIdKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	isBroadcaster, ok := r.Context().Value(IsBroadcasterKey).(bool)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if !isBroadcaster {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	broadcaster, err := GetBroadcasterByID(channelId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(broadcaster)
	w.WriteHeader(http.StatusOK)
}

// func getUser(w http.ResponseWriter, r *http.Request) {
// 	userId, ok := r.Context().Value(UserIdKey).(string)
// 	if !ok {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	opaqueUserId, ok := r.Context().Value(OpaqueUserIdKey).(string)
// 	if !ok {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	user, err := GetUserForOpaqueID(userId)
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}
// }

// func postUser(w http.ResponseWriter, r *http.Request) {
// 	userId, ok := r.Context().Value(UserIdKey).(string)
// 	if !ok {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	opaqueUserId, ok := r.Context().Value(OpaqueUserIdKey).(string)
// 	if !ok {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	user := User{OpaqueUserID: opaqueUserId, UserID: userId}
// 	err := CreateUser(user)
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	w.WriteHeader(http.StatusOK)
// }

func getNewOverlayToken(w http.ResponseWriter, r *http.Request) {
	channelId, ok := r.Context().Value(ChannelIdKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	isBroadcaster, ok := r.Context().Value(IsBroadcasterKey).(bool)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if !isBroadcaster {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	overlayToken, err := UpdateOverlayTokenForID(channelId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(overlayToken)
	w.WriteHeader(http.StatusOK)
}

// func postBroadcaster(w http.ResponseWriter, r *http.Request) {
// 	isBroadcaster, ok := r.Context().Value(IsBroadcasterKey).(bool)
// 	if !ok {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	if !isBroadcaster {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	broadcasterId, ok := r.Context().Value(UserIdKey).(string)
// 	if !ok {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	count, err := CountBroadcasterByID(broadcasterId)
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	if count != 0 {
// 		http.Error(w, "broadcaster already exists", http.StatusBadRequest)
// 		return
// 	}

// 	err = CreateBroadcaster(newBroadcaster)
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}
// 	w.WriteHeader(http.StatusOK)
// }

// func getNewBroadcasterToken(w http.ResponseWriter, r *http.Request) {

// }

// func deleteBroadcaster(w http.ResponseWriter, r *http.Request) {
// 	channelId, ok := r.Context().Value(ChannelIdKey).(string)
// 	if !ok {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	isBroadcaster, ok := r.Context().Value(IsBroadcasterKey).(bool)
// 	if !ok {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	if !isBroadcaster {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	err := DeleteBroadcasterByID(channelId)
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusBadRequest)
// 		return
// 	}

// 	w.WriteHeader(http.StatusOK)
// }

func getOverlay(w http.ResponseWriter, r *http.Request) {
	// channelId := r.PathValue("channel_id")
	// broadcaster, err := GetBroadcasterByID(channelId)
	// if err != nil {
	// 	http.Error(w, err.Error(), http.StatusInternalServerError)
	// 	return
	// }

	// overlayToken := r.PathValue("overlay_token")
	// if broadcaster.OverlayToken != overlayToken {
	// 	http.Error(w, "invalid overlay token", http.StatusInternalServerError)
	// 	return
	// }

	tmpl, err := template.ParseFiles("./static/html/unbox.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	tmpl.Execute(w, nil)
}

// func getPubSub(w http.ResponseWriter, r *http.Request) {
// 	time.Sleep(2 * time.Second)

// 	channelId, ok := r.Context().Value(ChannelIdKey).(string)
// 	if !ok {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	broadcaster, err := GetBroadcasterByID(channelId)
// 	if err != nil {
// 		log.Printf("pubsub: no channel_id specified: %v\n", err)
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	if broadcaster.UserID == "" {
// 		// broadcaster not authorized for the extension
// 		log.Printf("pubsub: channel is not authorized for extension\n")
// 		return
// 	}

// 	// userId, ok := r.Context().Value(UserIdKey).(string)
// 	// if !ok {
// 	// 	log.Printf("pubsub: unauthed access\n")
// 	// 	http.Error(w, "unauthorized", http.StatusUnauthorized)
// 	// 	return
// 	// }

// 	claims := jwt.MapClaims{
// 		"exp":        time.Now().Add(time.Minute * 2).Unix(),
// 		"user_id":    twitchExtOwnerId,
// 		"role":       "external",
// 		"channel_id": broadcaster.UserID,
// 		"pubsub_perms": jwt.MapClaims{
// 			// "listen": "whisper-" + userId,
// 			"send": []string{"broadcast"},
// 		},
// 	}

// 	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims, nil)
// 	tokenStr, err := token.SignedString(JWTSigningKey)
// 	if err != nil {
// 		log.Printf("pubsub: error signing jwt string: %v\n", err)
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}
// 	log.Println(tokenStr)

// 	j, err := json.Marshal(map[string]any{
// 		"target":         []string{"broadcast"},
// 		"broadcaster_id": broadcaster.UserID,
// 		"message":        "test message!!",
// 	})
// 	if err != nil {
// 		log.Printf("pubsub: error json marshal: %v\n", err)
// 		return
// 	}

// 	req, err := http.NewRequestWithContext(
// 		context.Background(), "POST", "https://api.twitch.tv/helix/extensions/pubsub", bytes.NewBuffer(j),
// 	)
// 	if err != nil {
// 		log.Printf("event: error create request: %v\n", err)
// 		return
// 	}

// 	req.Header.Add("Client-Id", conf.ClientID)
// 	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", tokenStr))
// 	req.Header.Add("Content-Type", "application/json")

// 	_, err = http.DefaultClient.Do(req)
// 	if err != nil {
// 		log.Printf("pubsub: error do post request: %v\n", err)
// 		return
// 	}

// 	// j, err := json.Marshal(map[string]string{
// 	// 	"token": tokenStr,
// 	// })
// 	// if err != nil {
// 	// 	log.Printf("pubsub: error marshal json: %v\n", err)
// 	// 	http.Error(w, err.Error(), http.StatusInternalServerError)
// 	// 	return
// 	// }

// 	// json.NewEncoder(w).Encode(j)
// }
