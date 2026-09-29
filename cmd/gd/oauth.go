package main

import (
	"fmt"
	"os"
	"strings"

	"gd/internal/config"
)

// cmdOAuth manages the user's own Google OAuth client (client_id +
// client_secret pair). rclone's shared Drive client_id stops working during
// 2026, so users who bring their own keep authorizing and refreshing tokens
// past that date. Without a pair gd keeps using rclone's built-in client.
//
//	gd oauth set <client_id> <client_secret>   store the pair in ~/.gd/gd.json
//	gd oauth show                              which client is in effect
//	gd oauth clear                             remove the stored pair
func cmdOAuth(rest []string) error {
	if len(rest) == 0 {
		return fmt.Errorf(oauthUsage)
	}
	switch rest[0] {
	case "set":
		return oauthSet(rest[1:])
	case "show":
		return oauthShow()
	case "clear":
		return oauthClear()
	default:
		return fmt.Errorf("unknown oauth command %q\n\n%s", rest[0], oauthUsage)
	}
}

const oauthUsage = `usage: gd oauth set <client_id> <client_secret> | show | clear

rclone's shared Google Drive client_id stops working during 2026. Store your
own OAuth client to keep authorizing and refreshing tokens past that date:
create one in Google Cloud Console (Drive API enabled, OAuth client of type
"Desktop app"), then run ` + "`gd oauth set`" + ` and re-auth each account with
` + "`gd reauth <acc>`" + `. See https://rclone.org/drive/#making-your-own-client-id
Environment variables GD_CLIENT_ID / GD_CLIENT_SECRET also work and take
priority over the stored pair.`

func oauthSet(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: gd oauth set <client_id> <client_secret>")
	}
	return saveOAuthPair(args[0], args[1])
}

// saveOAuthPair validates and stores the pair in gd.json, then explains what
// existing accounts need. Kept separate from the CLI wiring so the wizard
// can call it after collecting input.
func saveOAuthPair(id, secret string) error {
	id, secret = strings.TrimSpace(id), strings.TrimSpace(secret)
	if id == "" || secret == "" {
		return fmt.Errorf("both client_id and client_secret are required")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.OAuthApp = &config.OAuthApp{ClientID: id, ClientSecret: secret}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println("own Google OAuth client stored in ~/.gd/gd.json")
	if envID := os.Getenv("GD_CLIENT_ID"); envID != "" || os.Getenv("GD_CLIENT_SECRET") != "" {
		fmt.Println("note: GD_CLIENT_ID/GD_CLIENT_SECRET are set in this shell and take priority over the stored pair")
	}
	fmt.Println()
	fmt.Println("IMPORTANT: existing accounts must re-authorize. A refresh token is")
	fmt.Println("bound to the client_id that minted it, so old tokens cannot renew")
	fmt.Println("through your new client:")
	for _, a := range cfg.Accounts {
		fmt.Println("  gd reauth " + a.Name)
	}
	fmt.Println()
	fmt.Println("verify what is in effect anytime with: gd oauth show")
	return nil
}

func oauthShow() error {
	id, secret, source, err := config.ResolveOAuthClient()
	if err != nil {
		return err
	}
	switch source {
	case "env":
		fmt.Println("source:   env (GD_CLIENT_ID / GD_CLIENT_SECRET)")
	case "config":
		fmt.Println("source:   file (~/.gd/gd.json)")
	default:
		fmt.Println("source:   built-in (rclone's shared client_id, retires during 2026)")
		fmt.Println("recommended: gd oauth set <client_id> <client_secret>")
	}
	if id == "" {
		fmt.Println("client_id: (not set)")
		fmt.Println("secret:    (not set)")
		return nil
	}
	fmt.Println("client_id:", id)
	fmt.Println("secret:   ", config.MaskSecret(secret))
	return nil
}

func oauthClear() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.OAuthApp == nil {
		fmt.Println("nothing to clear: no stored OAuth client in gd.json")
		fmt.Println("(GD_CLIENT_ID / GD_CLIENT_SECRET env vars, if any, are untouched)")
		return nil
	}
	cfg.OAuthApp = nil
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println("stored OAuth client removed from gd.json")
	if envID := os.Getenv("GD_CLIENT_ID"); envID != "" {
		fmt.Println("note: GD_CLIENT_ID is still set in this shell and stays in effect")
	}
	fmt.Println("until accounts re-authorize (`gd reauth <acc>`), their refresh tokens")
	fmt.Println("remain bound to the client_id that minted them")
	return nil
}
