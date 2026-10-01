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
	case "setup":
		return oauthSetup(rest[1:])
	case "show":
		return oauthShow()
	case "clear":
		return oauthClear()
	default:
		return fmt.Errorf("unknown oauth command %q\n\n%s", rest[0], oauthUsage)
	}
}

const oauthUsage = `usage: gd oauth setup | set <client_id> <client_secret> | show | clear

rclone's shared Google Drive client_id stops working during 2026. ` + "`gd oauth setup`" + `
walks you through creating your own in Google Cloud Console (opens the exact
pages, then stores the pair and re-authorizes every account). To skip the
walkthrough, create a "Desktop app" client yourself and run ` + "`gd oauth set <id> <secret>`" + `.
See https://rclone.org/drive/#making-your-own-client-id. Environment variables
GD_CLIENT_ID / GD_CLIENT_SECRET also work and take priority over the stored pair.`

func oauthSet(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: gd oauth set <client_id> <client_secret>")
	}
	return saveOAuthPair(args[0], args[1])
}

// Cloud Console URLs used by the walkthrough. They carry a project hint
// (?project=...) so every page opens inside the same project.
const (
	gcpNewProjectURL  = "https://console.cloud.google.com/projectcreate"
	gcpEnableAPIURL   = "https://console.cloud.google.com/apis/library/drive.googleapis.com?project="
	gcpConsentURL     = "https://console.cloud.google.com/auth/audience/create?project="
	gcpConsentEditURL = "https://console.cloud.google.com/auth/audience/edit?project="
	gcpCredsOauthURL  = "https://console.cloud.google.com/apis/credentials/oauthclient?project="
)

// oauthSetup walks the user through creating their own Google OAuth client:
// it opens the exact Console pages (carrying a shared project hint), accepts
// the pasted pair, stores it, and re-authorizes every account so no manual
// `gd reauth` calls remain. Google has no API for creating OAuth clients, so
// the console steps are inherently interactive; this keeps them to one short
// session.
func oauthSetup(args []string) error {
	if len(args) > 0 && args[0] != "-h" && args[0] != "--help" {
		return fmt.Errorf("usage: gd oauth setup")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fmt.Println("This creates your own Google OAuth client (rclone's shared one retires during 2026).")
	fmt.Println("A browser will open for each step; nothing is entered anywhere but the Console itself.")

	var project string
	if cfg.OAuthApp != nil && cfg.OAuthApp.ClientID != "" {
		if p := oauthProjectFromID(cfg.OAuthApp.ClientID); p != "" {
			fmt.Printf("\nA client is already stored (%s). Reusing its project %q.\n", config.MaskSecret(cfg.OAuthApp.ClientID), p)
			project = p
		} else {
			fmt.Println("\nA client is already stored; running again replaces it.")
		}
	}
	if project == "" {
		fmt.Println("\nStep 1: create (or pick) a Google Cloud project. Any name works, " +
			"no billing, no Google Cloud subscription.")
		fmt.Println("Copy the project ID from the page (or its URL when done) and paste it here.")
		openURL(gcpNewProjectURL)
		project = strings.TrimSpace(ask("project ID", ""))
		if project == "" {
			return fmt.Errorf("a project ID is required; get it from the Console page and paste it here")
		}
	}

	fmt.Println("\nStep 2: enable the Google Drive API for the project (one click on the page).")
	openURL(gcpEnableAPIURL + project)
	if !askYN("opened and enabled?", true) {
		return fmt.Errorf("aborted at step 2")
	}

	fmt.Println("\nStep 3: OAuth consent screen. Choose External, fill only the required fields,")
	fmt.Println("add yourself as a test user, then press \"PUBLISH APP\" and confirm.")
	fmt.Println("Publishing without verification is fine: tokens then never expire (in Testing")
	fmt.Println("mode Google kills refresh tokens every 7 days), and the unverified-app warning")
	fmt.Println("is bypassed once per account with \"Advanced -> Go to app\".")
	openURL(gcpConsentURL + project)
	if !askYN("consent screen published to Production?", true) {
		fmt.Println("opening the edit page for an existing consent screen instead")
		openURL(gcpConsentEditURL + project)
	}

	fmt.Println("\nStep 4: create credentials of type \"Desktop app\" and copy the")
	fmt.Println("client ID and client secret from the page.")
	openURL(gcpCredsOauthURL + project)
	id := strings.TrimSpace(ask("client_id", ""))
	secret := strings.TrimSpace(ask("client_secret", ""))
	if id == "" || secret == "" {
		return fmt.Errorf("both client_id and client_secret are required; rerun `gd oauth setup` to continue")
	}
	if err := saveOAuthPair(id, secret); err != nil {
		return err
	}

	if len(cfg.Accounts) == 0 {
		fmt.Println("\nNo accounts yet: future `gd add` will use this client automatically.")
		return nil
	}
	fmt.Printf("\nStep 5: re-authorize %d account(s). A refresh token is bound to the client_id", len(cfg.Accounts))
	fmt.Println(" that minted it, so each account needs one browser Allow click.")
	return reauthAll()
}

// oauthProjectFromID extracts the GCP project number/id embedded in a
// Desktop-app client_id ("1234567890-abc...apps.googleusercontent.com"),
// so a re-run of the walkthrough opens the same project.
func oauthProjectFromID(clientID string) string {
	dash := strings.Index(clientID, "-")
	if dash <= 0 {
		return ""
	}
	prefix := clientID[:dash]
	for _, r := range prefix {
		if r < '0' || r > '9' {
			return "" // not a numeric project number: leave the hint off
		}
	}
	return prefix
}

// openURL opens the browser when possible; otherwise it prints the URL so a
// headless user (or an agent's user) can open it by hand.
func openURL(url string) {
	fmt.Println("open:", url)
	if openBrowser(url) {
		return
	}
	fmt.Println("(could not open a browser automatically; use the URL above)")
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
	fmt.Println("or re-authorize them all in one go: gd reauth --all")
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
		fmt.Println("recommended: gd oauth setup (walkthrough) or gd oauth set <id> <secret>")
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
