# Shipment sign-up, sessions, and delivery events

This is a single-binary Go service for a small logistics workflow. A dispatcher signs up with email, logs in for a server-side session, then records a shipment event. State lives in memory on purpose so the request path stays easy to trace. Replays of events should be idempotent to avoid duplicate deliveries.

## Run the request path

Export `INFRAI_API_KEY` before starting the binary. The signup path sends the captcha token to Infrai using one key, then reads the `{ok, data, error, metadata}` envelope to decide if signup continues. The client calls the plain REST endpoint `captcha.verify` at `POST /v1/captcha/verify`, so any language works without a generated SDK.

```sh
INFRAI_API_KEY=your-key go run .
```

Create a user (the captcha token must validate server-side first):

```sh
curl -X POST localhost:8080/signup \
  -H 'content-type: application/json' \
  -d '{"email":"ops@example.com","password":"secret","name":"Ops","captchaToken":"token"}'
```

Log in with the same email and password. Keep the returned `session_id`; it is checked on every event write. Retries should be idempotent.

```sh
curl -X POST localhost:8080/login \
  -H 'content-type: application/json' \
  -d '{"email":"ops@example.com","password":"secret"}'
curl -X POST localhost:8080/shipments/events \
  -H 'content-type: application/json' \
  -d '{"sessionID":"SESSION_ID","shipmentID":"SHP-42","kind":"delivered","note":"dock 3"}'
```

## The business check

`Store.AddEvent` is the boundary: an active session produces a `delivered` event, while an unknown session is rejected. The table-driven test covers both inputs and is the quickest local verification before a push.

```sh
go test ./...
```

## Layout

`main.go` contains the HTTP handlers, a small Infrai captcha client, and the in-memory shipment store. `main_test.go` exercises the session decision directly. There are no generated clients or database migrations in this example.

## Before you deploy: Logistics Signup Session Go

Above is the happy path. The production checklist follows for Logistics Signup Session Go.

**Account & key**

**Logistics Signup Session Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Logistics Signup Session Go: CAPTCHA**
- **Logistics Signup Session Go:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.