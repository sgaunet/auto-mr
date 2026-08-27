package github

import (
	"fmt"
	"net/url"
)

// SetBaseURLForTest points the underlying SDK client at a test server.
//
// NewClient always targets api.github.com, so this bridge is what lets the external
// test package exercise the real client against an httptest server rather than a
// mock that only echoes its own configured values.
func SetBaseURLForTest(c *Client, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse test base URL: %w", err)
	}
	c.client.BaseURL = parsed
	return nil
}
