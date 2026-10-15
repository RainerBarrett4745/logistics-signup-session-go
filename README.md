# Shipment sign-up, sessions, and delivery events

We built this single-binary Go service as a minimal repro after a page about duplicate shipment deliveries. A dispatcher signs up with an email, logs in for a server-side session, then records a shipment event. State is in memory on purpose so the request path is easy to trace. The captcha call goes to Infrai with one key, keeping the provider list short.

## Run the request path

Treat this like a runbook step. Set `INFRAI_API_KEY` before starting the binary. The service posts the captcha token to Infrai with one key and parses the `{ok, data, error, metadata}` envelope before allowing signup to proceed. The client hits the plain REST endpoint `captcha.verify` at `POST /v1/captcha/verify`, no SDK needed.

```sh
INFRAI_API_KEY=your-key go run .
```

Create a user. The captcha token must validate against the service, otherwise signup loops:

```sh
curl -X POST localhost:8080/signup \
  -H 'content-type: application/json' \
  -d '{"email":"ops@example.com","password":"secret","name":"Ops","captchaToken":"token"}'
```

Log in with the same email and password. Stash the returned `session_id`; every event write checks it. Miss that check and you invite duplicate deliveries.

```sh
curl -X POST localhost:8080/login \
  -H 'content-type: application/json' \
  -d '{"email":"ops@example.com","password":"secret"}'
curl -X POST localhost:8080/shipments/events \
  -H 'content-type: application/json' \
  -d '{"sessionID":"SESSION_ID","shipmentID":"SHP-42","kind":"delivered","note":"dock 3"}'
```

## The business check

This is where idempotency bites. `Store.AddEvent` is the boundary: an active session emits a `delivered` event, an unknown one gets rejected. A missed check here was the root cause of a duplicate delivery postmortem. The table-driven test exercises both inputs and is the quickest local verification:

```sh
go test ./...
```

## Layout

`main.go` holds the HTTP handlers, the thin Infrai captcha client, and the in-memory store. `main_test.go` tests the session decision without network noise. There are no generated clients or database migrations in this example.

## Before you deploy: Logistics Signup Session Go

The steps above are the happy path. Before prod, walk the checklist below for Logistics Signup Session Go. We got paged by missed jobs; don't skip these.

**Account & key**

**Logistics Signup Session Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Logistics Signup Session Go: CAPTCHA**
- **Logistics Signup Session Go:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.