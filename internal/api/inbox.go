package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// InboxPath is the proposed pending-orders endpoint (docs/INBOX_SIGNING.md;
// the site's real API is not decided yet).
const InboxPath = "/api/sync/inbox"

// MaxInboxResponse bounds the JSON answer (the string itself is at most
// 64 KB; the rest is the JSON around it).
const MaxInboxResponse = 80 << 10

// InboxResult is the site's pending-orders list.
type InboxResult struct {
	NotModified bool   // 304: same as etag
	Inbox       string // the signed GFIN1 string, unchecked here
	ETag        string
}

// Inbox asks for the guild's pending web orders. etag is the ETag of the
// list GF Sync last wrote ("" for none); the site answers 304 when it has
// not changed. The string is returned as sent: the caller verifies the
// signature before using it.
func (c *Client) Inbox(ctx context.Context, token, etag string) (*InboxResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.endpoint(InboxPath), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.Name+"/"+c.Version)
	req.Header.Set("Authorization", "Bearer "+token)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxInboxResponse+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotModified {
		return &InboxResult{NotModified: true, ETag: etag}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		he := responseError(resp, data, token)
		if he.Status == 401 || he.Status == 403 {
			return nil, fmt.Errorf("%w (%v)", ErrUnauthorized, he)
		}
		return nil, he
	}
	if len(data) > MaxInboxResponse {
		return nil, fmt.Errorf("api: inbox answer larger than %d bytes", MaxInboxResponse)
	}
	var out struct {
		Inbox string `json:"inbox"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.Inbox == "" {
		return nil, fmt.Errorf("api: bad inbox answer")
	}
	return &InboxResult{Inbox: out.Inbox, ETag: oneLine([]byte(resp.Header.Get("ETag")), 128)}, nil
}
