package rclone

import (
	"reflect"
	"testing"
)

func TestAuthorizeArgs(t *testing.T) {
	// The user's own OAuth client: credentials go to rclone positionally.
	own := authorizeArgs("my-app.apps.googleusercontent.com", "sekret")
	wantOwn := []string{"authorize", "drive", "my-app.apps.googleusercontent.com", "sekret", "--auth-no-open-browser"}
	if !reflect.DeepEqual(own, wantOwn) {
		t.Fatalf("own client: got %v want %v", own, wantOwn)
	}

	// Built-in fallback: exactly the historical command line.
	builtin := authorizeArgs("", "")
	wantBuiltin := []string{"authorize", "drive", "--auth-no-open-browser"}
	if !reflect.DeepEqual(builtin, wantBuiltin) {
		t.Fatalf("built-in client: got %v want %v", builtin, wantBuiltin)
	}

	// A half-set pair must not leak a lone secret into the command line.
	half := authorizeArgs("my-app.apps.googleusercontent.com", "")
	wantHalf := []string{"authorize", "drive", "--auth-no-open-browser"}
	if !reflect.DeepEqual(half, wantHalf) {
		t.Fatalf("half-set pair: got %v want %v", half, wantHalf)
	}
	half2 := authorizeArgs("", "sekret")
	if !reflect.DeepEqual(half2, wantHalf) {
		t.Fatalf("half-set pair 2: got %v want %v", half2, wantHalf)
	}
}
