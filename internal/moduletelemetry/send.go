// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package moduletelemetry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultTimeout bounds each export so telemetry cannot stall an apply.
const DefaultTimeout = 5 * time.Second

// Send POSTs an encoded OTLP/HTTP JSON logs request to endpoint. Redirects
// are refused so the destination cannot change silently. Callers should treat
// any error as non-fatal: export is best-effort.
func Send(ctx context.Context, client *http.Client, endpoint string, body []byte) error {
	if err := ValidateEndpoint(endpoint); err != nil {
		return err
	}
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("telemetry endpoint redirects are not followed")
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("telemetry endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}
