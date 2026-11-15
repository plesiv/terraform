// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package moduletelemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func boolp(b bool) *bool    { return &b }
func strp(s string) *string { return &s }

func TestResolvePolicy(t *testing.T) {
	tests := map[string]struct {
		root    RootSettings
		env     map[string]string
		want    Policy
		wantErr bool
	}{
		"defaults":           {want: Policy{Enabled: true}},
		"env disables":       {env: map[string]string{EnvEnabled: "false"}, want: Policy{Enabled: false}},
		"root beats env":     {root: RootSettings{Enabled: boolp(true)}, env: map[string]string{EnvEnabled: "false"}, want: Policy{Enabled: true}},
		"env non-registry":   {env: map[string]string{EnvAllowNonRegistry: "1"}, want: Policy{Enabled: true, AllowNonRegistry: true}},
		"root endpoint":      {root: RootSettings{Endpoint: strp("https://org.example.com/v1/logs")}, want: Policy{Enabled: true, EndpointOverride: "https://org.example.com/v1/logs"}},
		"env endpoint":       {env: map[string]string{EnvEndpoint: "https://env.example.com/v1/logs"}, want: Policy{Enabled: true, EndpointOverride: "https://env.example.com/v1/logs"}},
		"root endpoint http": {root: RootSettings{Endpoint: strp("http://x.example.com")}, wantErr: true},
		"bad env bool":       {env: map[string]string{EnvEnabled: "maybe"}, wantErr: true},
		"empty env is unset": {env: map[string]string{EnvEnabled: ""}, want: Policy{Enabled: true}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ResolvePolicy(tc.root, env(tc.env))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestValidateEndpoint(t *testing.T) {
	ok := []string{"https://a.example.com/v1/logs"}
	bad := []string{"http://a.example.com", "https://", "https://user:pw@a.example.com", "not a url", "webhook:https://x"}
	for _, s := range ok {
		if err := ValidateEndpoint(s); err != nil {
			t.Errorf("%q: unexpected error %v", s, err)
		}
	}
	for _, s := range bad {
		if err := ValidateEndpoint(s); err == nil {
			t.Errorf("%q: expected error", s)
		}
	}
}

func TestEffectiveEndpointAndShouldReport(t *testing.T) {
	p := Policy{Enabled: true, EndpointOverride: "https://root.example.com"}
	if got := p.EffectiveEndpoint("https://mod.example.com"); got != "https://root.example.com" {
		t.Errorf("override not applied: %q", got)
	}
	p.EndpointOverride = ""
	if got := p.EffectiveEndpoint("https://mod.example.com"); got != "https://mod.example.com" {
		t.Errorf("module endpoint not used: %q", got)
	}

	reg := "registry.terraform.io/contoso/avm/azurerm"
	if !(Policy{Enabled: true}).ShouldReport(reg) {
		t.Error("registry source should report by default")
	}
	if (Policy{Enabled: true}).ShouldReport("git::https://github.com/x/y.git") {
		t.Error("git source should not report by default")
	}
	if !(Policy{Enabled: true, AllowNonRegistry: true}).ShouldReport("./local") {
		t.Error("non-registry should report when allowed")
	}
	if (Policy{Enabled: false, AllowNonRegistry: true}).ShouldReport(reg) {
		t.Error("disabled policy must not report")
	}
}

func TestIsRegistrySource(t *testing.T) {
	tests := map[string]bool{
		"registry.terraform.io/contoso/avm/azurerm": true,
		"contoso/avm/azurerm":                       true,
		"./modules/x":                               false,
		"../x":                                      false,
		"/abs/path":                                 false,
		"git::https://github.com/a/b.git":           false,
		"github.com/a/b":                            false,
		"app.terraform.io/org/name/aws":             false,
		"":                                          false,
	}
	for src, want := range tests {
		if got := IsRegistrySource(src); got != want {
			t.Errorf("IsRegistrySource(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestCanonicalSourceStripsCredentials(t *testing.T) {
	got := CanonicalSource("git::https://user:tok3n@github.com/a/b.git?ref=v1")
	if strings.Contains(got, "tok3n") || strings.Contains(got, "user:") {
		t.Errorf("credentials leaked: %q", got)
	}
	if want := "git::https://github.com/a/b.git?ref=v1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestClassify(t *testing.T) {
	rec := &InstanceRecord{InstanceID: "i"}
	cases := []struct {
		prev    *InstanceRecord
		removed bool
		want    Action
		ok      bool
	}{
		{nil, false, ActionCreated, true},
		{rec, false, ActionUpdated, true},
		{rec, true, ActionDeleted, true},
		{nil, true, "", false},
	}
	for _, c := range cases {
		got, ok := Classify(c.prev, c.removed)
		if got != c.want || ok != c.ok {
			t.Errorf("Classify(prev=%v, removed=%v) = (%q,%v), want (%q,%v)", c.prev != nil, c.removed, got, ok, c.want, c.ok)
		}
	}
}

func TestValidateData(t *testing.T) {
	if err := ValidateData(map[string]string{"tenant_id": "abc"}); err != nil {
		t.Errorf("valid data rejected: %v", err)
	}
	if err := ValidateData(map[string]string{"Tenant": "x"}); err == nil {
		t.Error("uppercase key accepted")
	}
	if err := ValidateData(map[string]string{"k": strings.Repeat("a", MaxValueLength+1)}); err == nil {
		t.Error("oversized value accepted")
	}
	big := map[string]string{}
	for i := 0; i <= MaxDataEntries; i++ {
		big[strings.Repeat("k", 1)+string(rune('a'+i%26))+strings.Repeat("x", i)] = "v"
	}
	if err := ValidateData(big); err == nil {
		t.Error("too many entries accepted")
	}
}

func TestNewIDFormat(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 36 || id[14] != '4' {
		t.Errorf("not a v4 UUID: %q", id)
	}
}

func TestEncodeOTLPLogs(t *testing.T) {
	ts := time.Unix(1794250000, 0)
	body, err := EncodeOTLPLogs(Event{
		EventID: "e1", InstanceID: "i1", Source: "registry.terraform.io/contoso/avm/azurerm",
		Version: "1.2.3", Action: ActionUpdated, Operation: "apply", Time: ts,
		Data: map[string]string{"tenant_id": "t", "subscription_id": "s"},
	}, "1.99.0")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ResourceLogs []struct {
			ScopeLogs []struct {
				LogRecords []struct {
					TimeUnixNano string `json:"timeUnixNano"`
					EventName    string `json:"eventName"`
					Attributes   []struct {
						Key   string `json:"key"`
						Value struct {
							StringValue string `json:"stringValue"`
						} `json:"value"`
					} `json:"attributes"`
				} `json:"logRecords"`
			} `json:"scopeLogs"`
		} `json:"resourceLogs"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	rec := doc.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	if rec.EventName != EventName || rec.TimeUnixNano != "1794250000000000000" {
		t.Errorf("bad record header: %+v", rec)
	}
	attrs := map[string]string{}
	for _, a := range rec.Attributes {
		attrs[a.Key] = a.Value.StringValue
	}
	want := map[string]string{
		"event.id": "e1", "module.lifecycle.action": "updated", "module.version": "1.2.3",
		"module.telemetry.data.tenant_id": "t", "module.telemetry.data.subscription_id": "s",
	}
	for k, v := range want {
		if attrs[k] != v {
			t.Errorf("attr %s = %q, want %q", k, attrs[k], v)
		}
	}
}

func TestSendBestEffortBehaviour(t *testing.T) {
	var gotCT string
	ok := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	client := ok.Client()
	if err := Send(context.Background(), client, ok.URL, []byte("{}")); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if gotCT != "application/json" {
		t.Errorf("content type = %q", gotCT)
	}

	fail := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fail.Close()
	if err := Send(context.Background(), fail.Client(), fail.URL, []byte("{}")); err == nil {
		t.Error("expected error on HTTP 500")
	}

	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example.com", http.StatusFound)
	}))
	defer redirect.Close()
	if err := Send(context.Background(), redirect.Client(), redirect.URL, []byte("{}")); err == nil {
		t.Error("expected redirect to be refused")
	}

	if err := Send(context.Background(), nil, "http://insecure.example.com", []byte("{}")); err == nil {
		t.Error("expected plain http to be refused")
	}
}
