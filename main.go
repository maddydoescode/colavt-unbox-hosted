package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/twitch"
	_ "modernc.org/sqlite"
)

var JWTSigningKey []byte
var DB *sql.DB
var eventsubService *EventSubService

type ContextKey string

const (
	IsBroadcasterKey ContextKey = "is_twitch_broadcaster"
	UserIdKey        ContextKey = "twitch_user_id"
	ChannelIdKey     ContextKey = "twitch_channel_id"
	OpaqueUserIdKey  ContextKey = "opaque_user_id"
)

var twitchExtOwnerId string

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	dbUrl := os.Getenv("DATABASE_URL")
	// dbInitSql := os.Getenv("DATABASE_INIT_SQL")
	clientId := os.Getenv("OAUTH2_CLIENT_ID")
	clientSecret := os.Getenv("OAUTH2_CLIENT_SECRET")
	jwtSecretKey := os.Getenv("JWT_SECRET_KEY")
	redirectUrl := os.Getenv("OAUTH2_REDIRECT_URL")
	twitchExtOwnerId = os.Getenv("TWITCH_EXT_OWNER_ID")

	// TODO: can we get the secret programmatically?
	b, err := base64.StdEncoding.DecodeString(jwtSecretKey)
	if err != nil {
		log.Fatal(err)
	}
	JWTSigningKey = b

	db, err := sql.Open("sqlite", dbUrl)
	if err != nil {
		log.Fatal(err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	b, err = os.ReadFile("./db/init.sql")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(string(b))
	if err != nil {
		log.Fatal(err)
	}
	DB = db

	conf = &oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		RedirectURL:  redirectUrl,
		Scopes:       []string{"user:read:email", "channel:manage:redemptions"},
		Endpoint:     twitch.Endpoint,
	}
}

func main() {
	eventsubService = NewEventSubService()

	api := http.NewServeMux()

	// api.HandleFunc("GET /items", getItems)
	api.HandleFunc("GET /items", getItems)
	api.HandleFunc("POST /items", postItems)
	api.HandleFunc("DELETE /items/{item_id}", deleteItems)

	api.HandleFunc("GET /inventory", getInventory)
	api.HandleFunc("GET /inventory/all", getAllInventory)
	// api.HandleFunc("POST /inventory/{id}", postInventory)
	api.HandleFunc("DELETE /inventory/{inventory_item_id}", deleteInventory)

	api.HandleFunc("GET /broadcasters", getBroadcaster)
	// api.HandleFunc("POST /broadcasters", postBroadcaster)
	// api.HandleFunc("GET /broadcasters/{id}/token", getNewBroadcasterToken)
	// api.HandleFunc("DELETE /broadcasters/{id}", deleteBroadcaster)

	// api.HandleFunc("GET /users/{opaque_user_id}", getUser)

	api.HandleFunc("GET /overlay/new", getNewOverlayToken)

	// api.HandleFunc("GET /pubsub", getPubSub)

	file := http.NewServeMux()
	file.Handle("GET /", neuter(http.FileServer(http.Dir("./static"))))

	data := http.NewServeMux()
	data.Handle("GET /", neuter(http.FileServer(http.Dir("./data"))))

	oa2 := http.NewServeMux()
	oa2.HandleFunc("GET /authorize", authorize)
	oa2.HandleFunc("GET /callback", callback)

	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", auth(api)))
	mux.Handle("/static/", http.StripPrefix("/static", file))
	mux.Handle("/data/", http.StripPrefix("/data", data))
	mux.Handle("/auth/", http.StripPrefix("/auth", oa2))
	mux.HandleFunc("/ws", ws)

	mux.HandleFunc("/overlay/{channel_id}/{overlay_token}", getOverlay)

	wrappedMux := chain(mux, logging, cors)

	// tlsCertPath := os.Getenv("TLS_CERTIFICATE_PATH")
	// tlsKeyPath := os.Getenv("TLS_KEY_PATH")

	err := http.ListenAndServeTLS("0.0.0.0:8080", "certs/server.crt", "certs/server.key", wrappedMux)
	// err := http.ListenAndServe("0.0.0.0:8080", wrappedMux)
	if err != nil {
		log.Fatal(err)
	}
}

func chain(h http.Handler, m ...func(http.Handler) http.HandlerFunc) http.Handler {
	for _, f := range slices.Backward(m) {
		h = f(h)
	}
	return h
}

func logging(next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		req := fmt.Sprintf("%s %s", r.Method, r.URL)
		next.ServeHTTP(w, r)
		log.Println(r.RemoteAddr, req, "completed in", time.Since(start))
	})
}

func cors(next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Access-Control-Allow-Credentials", "true")
		w.Header().Add("Access-Control-Allow-Headers", "*")
		w.Header().Add("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")

		if r.Method == http.MethodOptions {
			http.Error(w, "No Content", http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func auth(next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := ""
		if after, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			bearer = after
		} else {
			http.Error(w, "no bearer token", http.StatusUnauthorized)
			return
		}

		claims := jwt.MapClaims{}
		_, err := jwt.ParseWithClaims(bearer, claims, func(t *jwt.Token) (any, error) {
			return JWTSigningKey, nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))

		if err != nil {
			http.Error(w, "invalid jwt token", http.StatusUnauthorized)
			return
		}

		// TODO: maybe make a sum list instead of separate if statements?
		userId, ok := claims["user_id"].(string)
		if !ok {
			http.Error(w, "unauthorized user", http.StatusUnauthorized)
			return
		}
		log.Printf("user_id = %s\n", userId)

		// WHY IS THIS NOT RETURNING WITH THE "U" WHEN ITS SENT???
		// log.Println(userId)
		// userId, _ = strings.CutPrefix(userId, "U") // twitch prepends a U here and only here for some reason
		// log.Println(userId)

		channelId, ok := claims["channel_id"].(string)
		if !ok {
			http.Error(w, "unauthorized user", http.StatusUnauthorized)
			return
		}
		log.Printf("channel_id = %s\n", channelId)

		role, ok := claims["role"].(string)
		if !ok {
			http.Error(w, "unauthorized user", http.StatusUnauthorized)
			return
		}

		opaqueUserId, ok := claims["opaque_user_id"].(string)
		if !ok {
			http.Error(w, "unauthorized user", http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, UserIdKey, userId)
		ctx = context.WithValue(ctx, ChannelIdKey, channelId)
		ctx = context.WithValue(ctx, IsBroadcasterKey, role == "broadcaster")
		ctx = context.WithValue(ctx, OpaqueUserIdKey, opaqueUserId)

		next.ServeHTTP(w, r.WithContext(ctx))

	})
}

func neuter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}
