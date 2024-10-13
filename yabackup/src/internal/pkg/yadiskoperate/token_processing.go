package yadiskoperate

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/yandex"
	"os"
	"time"
	"ybg/internal/types"
)

const FILE_PATH_TOKEN = "/data/tokenInfo.json"

func GetCheckCodeUrl(clientId string) string {
	return "https://oauth.yandex.ru/authorize?response_type=code&client_id=" + clientId
}

func CreateToken(clientId string, clientSecret string, code string) (types.TokenInfo, error) {

	config := oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		Endpoint:     yandex.Endpoint,
	}

	tokenValue, err := config.Exchange(context.Background(), code)
	if err != nil {
		return *new(types.TokenInfo), nil
	}
	tokenInfo := types.TokenInfo{AccessToken: tokenValue.AccessToken,
		RefreshToken: tokenValue.RefreshToken,
		Expiry:       tokenValue.Expiry}
	return tokenInfo, err
}

func RefreshToken(clientId string, clientSecret string, tokenInfo types.TokenInfo) (*types.TokenInfo, error) {

	config := oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		Endpoint:     yandex.Endpoint,
	}

	token := oauth2.Token{
		AccessToken:  tokenInfo.AccessToken,
		RefreshToken: tokenInfo.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(-24 * time.Hour)),
	}

	source := config.TokenSource(context.Background(), &token)
	newToken, err := source.Token()
	if err != nil {
		return nil, err
	}

	if newToken.AccessToken == tokenInfo.AccessToken && newToken.RefreshToken == tokenInfo.RefreshToken {
		return nil, fmt.Errorf("can not refresh token")
	}

	return &types.TokenInfo{AccessToken: newToken.AccessToken,
		RefreshToken: newToken.RefreshToken,
		Expiry:       newToken.Expiry}, nil
}

func writeToken(tokenInfo types.TokenInfo) error {
	jsonData, err := json.Marshal(tokenInfo)
	if err != nil {
		return err
	}

	err = os.WriteFile(FILE_PATH_TOKEN, jsonData, 0644)
	if err != nil {
		return err
	}

	return nil
}

func readToken() (types.TokenInfo, error) {
	plan, _ := os.ReadFile(FILE_PATH_TOKEN)
	var data types.TokenInfo
	err := json.Unmarshal(plan, &data)
	return data, err
}

func isTokenEmpty(tokenInfo types.TokenInfo) bool {
	return tokenInfo.AccessToken == "" || tokenInfo.RefreshToken == ""
}

func isTokenValid(tokenInfo types.TokenInfo) bool {
	token := oauth2.Token{AccessToken: tokenInfo.AccessToken,
		RefreshToken: tokenInfo.RefreshToken,
		Expiry:       tokenInfo.Expiry}
	return token.Valid()
}
