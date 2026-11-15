// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package moduletelemetry

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// EventName is the OTLP log record name for module usage events.
const EventName = "terraform.module.usage"

// Limits from the proposal's "payload-size/value-type limits" requirement.
// The proposal leaves exact numbers open, so these are proposals.
const (
	MaxDataEntries = 32
	MaxKeyLength   = 64
	MaxValueLength = 256
)

var dataKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Event is one module usage event, ready to be encoded.
type Event struct {
	EventID    string
	InstanceID string
	Source     string
	Version    string
	Action     Action
	Operation  string // e.g. "apply"
	Time       time.Time
	Data       map[string]string
}

// ValidateData checks author-supplied data against the limits. Sensitivity
// checks happen earlier in configuration handling; this only enforces shape.
func ValidateData(data map[string]string) error {
	if len(data) > MaxDataEntries {
		return fmt.Errorf("telemetry data has %d entries, maximum is %d", len(data), MaxDataEntries)
	}
	for k, v := range data {
		if len(k) == 0 || len(k) > MaxKeyLength || !dataKeyPattern.MatchString(k) {
			return fmt.Errorf("invalid telemetry data key %q: use lowercase letters, digits, and underscores", k)
		}
		if len(v) > MaxValueLength {
			return fmt.Errorf("telemetry data value for %q exceeds %d bytes", k, MaxValueLength)
		}
	}
	return nil
}

// NewID returns a random RFC 4122 version 4 UUID string. It is used for both
// event IDs and pseudonymous module-instance IDs.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

type otlpAnyValue struct {
	StringValue string `json:"stringValue"`
}

type otlpKeyValue struct {
	Key   string       `json:"key"`
	Value otlpAnyValue `json:"value"`
}

// EncodeOTLPLogs encodes the event as an OTLP/HTTP JSON ExportLogsServiceRequest.
// See https://opentelemetry.io/docs/specs/otlp/#otlphttp.
func EncodeOTLPLogs(ev Event, serviceVersion string) ([]byte, error) {
	if err := ValidateData(ev.Data); err != nil {
		return nil, err
	}

	attrs := []otlpKeyValue{
		kv("event.id", ev.EventID),
		kv("module.instance_id", ev.InstanceID),
		kv("module.source", ev.Source),
		kv("module.version", ev.Version),
		kv("module.lifecycle.action", string(ev.Action)),
		kv("terraform.operation", ev.Operation),
	}

	keys := make([]string, 0, len(ev.Data))
	for k := range ev.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		attrs = append(attrs, kv("module.telemetry.data."+k, ev.Data[k]))
	}

	req := map[string]any{
		"resourceLogs": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []otlpKeyValue{
						kv("service.name", "terraform"),
						kv("service.version", serviceVersion),
					},
				},
				"scopeLogs": []any{
					map[string]any{
						"scope": map[string]any{"name": "github.com/hashicorp/terraform/internal/moduletelemetry"},
						"logRecords": []any{
							map[string]any{
								"timeUnixNano": strconv.FormatInt(ev.Time.UnixNano(), 10),
								"eventName":    EventName,
								"attributes":   attrs,
							},
						},
					},
				},
			},
		},
	}
	return json.Marshal(req)
}

func kv(k, v string) otlpKeyValue {
	return otlpKeyValue{Key: k, Value: otlpAnyValue{StringValue: v}}
}
