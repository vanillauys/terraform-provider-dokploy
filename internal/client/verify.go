package client

import "context"

// TestConnection calls one of the connection-test endpoints that the
// verify_connection attribute uses, for example "registry.testRegistry".
// The caller passes the create request of the resource, or for a git
// provider the id of the stored record: every endpoint ignores a key that
// it does not know (probed live, v0.30.8, 2026-09-30).
//
// Success is HTTP 200 with a bare true body, which this method discards. On
// failure the error carries the server message verbatim: every failure
// probed is HTTP 400. A gitea id that does not exist is HTTP 404, which the
// client maps to ErrNotFound without the message. The message of
// destination.testConnection contains the full rclone command line, which
// includes both credentials, so callers must redact it.
func (c *Client) TestConnection(ctx context.Context, endpoint string, body any) error {
	return c.Post(ctx, "/"+endpoint, body, nil)
}
