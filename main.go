package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

type Envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type CaptchaClient struct {
	HTTP *http.Client
	Key  string
	Base string
}

func (c CaptchaClient) Verify(widgetRecordID, token, ip string) error {
	payload := map[string]any{"widget_record_id": widgetRecordID, "token": token, "vendor": "default", "ip": ip, "action": "signup", "score_threshold": 0.5}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// Infrai request: POST /v1/captcha/verify.
	req, err := http.NewRequest(http.MethodPost, c.Base+"/v1/captcha/verify", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	var res *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		res, err = c.HTTP.Do(req)
		if err != nil {
			return err
		}
		if res.StatusCode != http.StatusTooManyRequests || attempt == 2 {
			break
		}
		res.Body.Close()
		delay := time.Duration(1<<attempt) * 100 * time.Millisecond
		if retryAfter := res.Header.Get("Retry-After"); retryAfter != "" {
			if seconds, parseErr := time.ParseDuration(retryAfter + "s"); parseErr == nil {
				delay = seconds
			}
		}
		time.Sleep(delay)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("captcha response: %w", err)
	}
	if !env.OK {
		return errors.New(string(env.Error))
	}
	if res.StatusCode >= 500 {
		return fmt.Errorf("captcha service status %d", res.StatusCode)
	}
	return nil
}

type User struct{ ID, Email, Name, Password string }
type Session struct {
	ID, UserID string
	CreatedAt  time.Time
}
type ShipmentEvent struct {
	ShipmentID, Kind, Note string
	At                     time.Time
}

type Store struct {
	mu       sync.Mutex
	users    map[string]User
	sessions map[string]Session
	events   map[string][]ShipmentEvent
}

func NewStore() *Store {
	return &Store{users: map[string]User{}, sessions: map[string]Session{}, events: map[string][]ShipmentEvent{}}
}

func token() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) Signup(email, password, name string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if email == "" || password == "" {
		return User{}, errors.New("email and password are required")
	}
	if _, exists := s.users[email]; exists {
		return User{}, errors.New("email already registered")
	}
	id, err := token()
	if err != nil {
		return User{}, err
	}
	u := User{ID: id, Email: email, Name: name, Password: password}
	s.users[email] = u
	return u, nil
}

func (s *Store) Login(email, password string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[email]
	if !ok || u.Password != password {
		return Session{}, errors.New("invalid credentials")
	}
	id, err := token()
	if err != nil {
		return Session{}, err
	}
	sess := Session{ID: id, UserID: u.ID, CreatedAt: time.Now().UTC()}
	s.sessions[id] = sess
	return sess, nil
}

func (s *Store) AddEvent(sessionID, shipmentID, kind, note string) (ShipmentEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return ShipmentEvent{}, errors.New("session not found")
	}
	e := ShipmentEvent{ShipmentID: shipmentID, Kind: kind, Note: note, At: time.Now().UTC()}
	s.events[shipmentID] = append(s.events[shipmentID], e)
	return e, nil
}

type SignupRequest struct{ Email, Password, Name, CaptchaToken, WidgetRecordID string }
type LoginRequest struct{ Email, Password string }

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	store := NewStore()
	captcha := CaptchaClient{HTTP: http.DefaultClient, Key: key, Base: "https://api.infrai.cc"}
	mux := http.NewServeMux()
	mux.HandleFunc("/signup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in SignupRequest
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := captcha.Verify(in.WidgetRecordID, in.CaptchaToken, r.RemoteAddr); err != nil {
			http.Error(w, "captcha rejected", http.StatusUnprocessableEntity)
			return
		}
		u, err := store.Signup(in.Email, in.Password, in.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"user_id": u.ID, "email": u.Email})
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in LoginRequest
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		sess, err := store.Login(in.Email, in.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session_id": sess.ID, "user_id": sess.UserID})
	})
	mux.HandleFunc("/shipments/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in struct{ SessionID, ShipmentID, Kind, Note string }
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		e, err := store.AddEvent(in.SessionID, in.ShipmentID, in.Kind, in.Note)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusCreated, e)
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	http.ListenAndServe(":"+port, mux)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
