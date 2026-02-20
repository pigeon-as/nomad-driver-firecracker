// Package utils provides small helpers for taking addresses of literal values.
package utils

// String returns a pointer to the provided string.
func String(s string) *string { return &s }

// Bool returns a pointer to the provided bool.
func Bool(b bool) *bool { return &b }
