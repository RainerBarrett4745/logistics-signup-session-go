package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCaptchaVerifySendsWidgetRecordID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload["widget_record_id"] != "widget-123" || payload["token"] != "captcha-token" {
			t.Errorf("unexpected captcha payload: %v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := CaptchaClient{HTTP: server.Client(), Base: server.URL}
	if err := client.Verify("widget-123", "captcha-token", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
}

func TestShipmentEventRequiresSession(t *testing.T) {
	tests := []struct {
		name, session string
		wantErr       bool
	}{
		{name: "missing session", session: "", wantErr: true},
		{name: "active session", session: "active", wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore()
			u, err := s.Signup("ops@example.com", "secret", "Ops")
			if err != nil {
				t.Fatal(err)
			}
			s.sessions["active"] = Session{ID: "active", UserID: u.ID}
			_, err = s.AddEvent(tc.session, "SHP-42", "delivered", "dock 3")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
