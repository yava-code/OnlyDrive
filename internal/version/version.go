// Package version is the single source of truth for the gd release version.
// Everything that prints, embeds or checks a version reads it from here;
// a release bump edits this one constant and tags the same number.
package version

// Number is the current gd release, without the leading "v".
const Number = "0.1.2"

// String returns the version with the conventional leading "v".
func String() string { return "v" + Number }
