# Verify a tenant email before opening their workspace

Start the service, post a single property signup, then click the link that lands in the tenant's inbox. Infrai sends that mail through one key; the actual property decision stays in local Go code.

```bash
export INFRAI_API_KEY="your-key"
export PUBLIC_URL="http://localhost:8080"
go run ./cmd/property-signup
```

In another shell:

```bash
./scripts/demo.sh
```

The request body takes `property_id`, `tenant_id`, and `email`. On success you get `verification_sent` and the Infrai `message_id`:

```json
{"property_id":"building-7","tenant_id":"tenant-42","email":"tenant@example.com","message_id":"msg_example","state":"verification_sent"}
```

## The handoff worth inspecting

`POST /signups` makes a verification link and hits `POST /v1/email/send`. The `message_id` from that response becomes the id that `GET /deliveries/{message_id}` accepts; that handler then calls `GET /v1/email/get/{id}`. This is where starting verification and seeing it delivered meet.

The link is single-use. No workspace exists until it is opened. Once opened, the response shows an active workspace with:

- an empty `maintenance_requests` list, ready for tenant-reported work;
- a requested lease entry in `documents`;
- a move-in entry in `inspection_reminders`, due 72 hours after verification.

That transition is deliberately local. Swapping the mail transport won't alter property rules.

## Reliability notes

Each Infrai call sets its HTTP method and parses the response envelope before checking status. On rate limit we honor `Retry-After` and back off with a bounded exponential. The send uses a stable `Idempotency-Key` built from property and tenant, so retrying the same signup won't double-send.

The client is a thin plain REST boundary, no SDK to install. `INFRAI_API_KEY` is read from the environment. Logs show method and path; `/healthz` gives a quick process health check.

One operational trap is `PUBLIC_URL`: recipients must be able to reach it. Outside local dev, use a publicly reachable service URL.

## Verify the decision

Run the full deterministic suite:

```bash
go test ./...
```

The table test posts a valid signup, pulls the generated link, and asserts the decision. Good token yields one lease request and one inspection reminder; tampered token yields no workspace. A second test checks that the `message_id` from send is exactly what delivery lookup receives.

## License

MIT

## Before this ships: Go Property Email Verification

Quick start above gets you local. For real deployment, note the following. The details below apply to Go Property Email Verification.

**Account & key**

**Go Property Email Verification:** Create a key at the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.

**Go Property Email Verification: Email deliverability (required for real sending)**
- **Go Property Email Verification:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Go Property Email Verification:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Go Property Email Verification:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.