package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"uuid"

	"github.com/adeithe/go-twitch/api"
	"golang.org/x/oauth2"
)

var conf *oauth2.Config

func authorize(w http.ResponseWriter, r *http.Request) {
	state := uuid.NewV4().String()

	http.SetCookie(w, &http.Cookie{
		Name:     "state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   60,
		SameSite: http.SameSiteLaxMode,
	})

	url := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie("state")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if state == "" || state != cookie.Value {
		http.Error(w, "state mismatch", http.StatusUnauthorized)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "state",
		MaxAge: -1,
	})

	code := r.URL.Query().Get("code")
	tok, err := conf.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	auth := api.UserToken(conf, tok)
	clientId := os.Getenv("OAUTH2_CLIENT_ID")
	client := api.New(clientId, api.WithDefaultAuthorization(auth))

	call := client.Users.List()
	users, err := call.Do(r.Context())
	if err != nil {
		http.Error(w, "check logs", http.StatusInternalServerError)
		log.Printf("error twitch api call users: %v\n", err)
		return
	}

	broadcaster := Broadcaster{
		UserID:   users.Data[0].UserID,
		Username: users.Data[0].UserName,
	}

	err = CreateBroadcaster(broadcaster, tok)
	if err != nil {
		http.Error(w, "check logs", http.StatusInternalServerError)
		log.Printf("error create broadcaster: %v\n", err)
		return
	}

	// TODO: more programmatic, we also get this in the live-config
	fmt.Fprint(w, "authentication done, close this popup to continue")
}
