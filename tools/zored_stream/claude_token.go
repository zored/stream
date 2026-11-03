package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"os/user"
	"strings"
	"time"
)

const (
	oauthTokenURL = "https://platform.claude.com/v1/oauth/token"
	oauthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	tokenSkew     = 5 * time.Minute
	securityStdin = 4032
)

var errKeychain = errors.New("keychain")

type oauthTokens struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	ExpiresIn             int64  `json:"expires_in"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
}

func (u *usage) claudeToken(force bool) (string, error) {
	doc, oauth, err := readCredentials()
	if err != nil {
		return "", err
	}
	access, _ := oauth["accessToken"].(string)
	expires, _ := oauth["expiresAt"].(json.Number).Int64()
	if access != "" && !force && time.UnixMilli(expires).After(time.Now().Add(tokenSkew)) {
		return access, nil
	}
	refresh, _ := oauth["refreshToken"].(string)
	if refresh == "" {
		return "", fmt.Errorf("%w: no claudeAiOauth.refreshToken", errKeychain)
	}
	var scopes []string
	if list, ok := oauth["scopes"].([]any); ok {
		for _, s := range list {
			scopes = append(scopes, fmt.Sprint(s))
		}
	}
	tokens, err := u.refreshTokens(refresh, scopes)
	if err != nil {
		return "", fmt.Errorf("token refresh: %w", err)
	}
	now := time.Now()
	oauth["accessToken"] = tokens.AccessToken
	if tokens.RefreshToken != "" {
		oauth["refreshToken"] = tokens.RefreshToken
	}
	oauth["expiresAt"] = now.Add(time.Duration(tokens.ExpiresIn) * time.Second).UnixMilli()
	if tokens.RefreshTokenExpiresIn > 0 {
		oauth["refreshTokenExpiresAt"] = now.Add(time.Duration(tokens.RefreshTokenExpiresIn) * time.Second).UnixMilli()
	}
	doc["claudeAiOauth"] = oauth
	if err := writeCredentials(doc); err != nil {
		return "", err
	}
	log.Printf("usage: refreshed the Claude Code token (rotated refresh token: %t), valid for %s",
		tokens.RefreshToken != "" && tokens.RefreshToken != refresh, time.Duration(tokens.ExpiresIn)*time.Second)
	return tokens.AccessToken, nil
}

func (u *usage) refreshTokens(refresh string, scopes []string) (oauthTokens, error) {
	body, err := json.Marshal(map[string]string{
		"grant_type": "refresh_token", "refresh_token": refresh, "client_id": oauthClientID, "scope": strings.Join(scopes, " "),
	})
	if err != nil {
		return oauthTokens{}, err
	}
	resp, err := u.http.Post(oauthTokenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return oauthTokens{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return oauthTokens{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return oauthTokens{}, fmt.Errorf("%s: %.200s", resp.Status, data)
	}
	var tokens oauthTokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return oauthTokens{}, err
	}
	if tokens.AccessToken == "" || tokens.ExpiresIn <= 0 {
		return oauthTokens{}, errors.New("no access_token/expires_in in the response")
	}
	return tokens, nil
}

func readCredentials() (map[string]any, map[string]any, error) {
	out, err := exec.Command("/usr/bin/security", "find-generic-password", "-s", usageKeychain, "-w").Output()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", errKeychain, err)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", errKeychain, err)
	}
	oauth, ok := doc["claudeAiOauth"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("%w: no claudeAiOauth", errKeychain)
	}
	if _, ok := oauth["expiresAt"].(json.Number); !ok {
		return nil, nil, fmt.Errorf("%w: no claudeAiOauth.expiresAt", errKeychain)
	}
	return doc, oauth, nil
}

func writeCredentials(doc map[string]any) error {
	me, err := user.Current()
	if err != nil {
		return err
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	payload := hex.EncodeToString(data)
	command := fmt.Sprintf("add-generic-password -U -a %q -s %q -X %q\n", me.Username, usageKeychain, payload)
	cmd := exec.Command("/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader(command)
	if len(command) > securityStdin {
		cmd = exec.Command("/usr/bin/security", "add-generic-password", "-U", "-a", me.Username, "-s", usageKeychain, "-X", payload)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: write: %v: %s", errKeychain, err, out)
	}
	return nil
}
