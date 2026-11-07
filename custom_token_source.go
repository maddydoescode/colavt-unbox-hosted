package main

import (
	"fmt"
	"log"

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
