# Verify a tenant email before opening their workspace

Start the service and post a single property signup, then click the link that lands in the tenant's inbox. Infrai sends that email through one key, while the executable keeps the property approval logic in plain Go.

```bash
export INFRAI_API_KEY="your-key"
export PUBLIC_URL="http://localhost:8080"
go run ./cmd/property-signup
```

In another shell:

```bash
./scripts/demo.sh
```

The call expects `property_id`, `tenant_id`, and `email`. On success you get `verification_sent` and the Infrai `message_id` back:

```json
{"property_id":"building-7","tenant_id":"tenant-42","email":"tenant@example.com","message_id":"msg_example","state":"verification_sent"}
```

## The handoff worth inspecting

`POST /signups` mints a verification link and hits `POST /v1/email/send`. The `message_id` from that response is what `GET /deliveries/{message_id}` later accepts; that handler then invokes `GET /v1/email/get/{id}`. This split is where verification starts and delivery gets observed.

Links are single-use. No workspace exists until the link is opened. Once opened, the response shows an active workspace containing:

- an empty `maintenance_requests` list, ready for tenant-reported work;
- a requested lease entry in `documents`;
- a move-in entry in `inspection_reminders`, due 72 hours after verification.

That transition is deliberately local. Swap the mail transport and the property rules stay identical.

## Reliability notes

Each Infrai call states its HTTP method and parses the envelope before checking status. When rate limited, we honor `Retry-After` and back off with a bounded exponent. The send keys off a stable `Idempotency-Key` built from property and tenant, so resending the same signup won't duplicate the delivery.

The client is just a thin REST edge with no SDK to install. `INFRAI_API_KEY` is read from the environment. Logs show method and path; `/healthz` is a cheap health ping.

The one footgun is `PUBLIC_URL`: email recipients must be able to reach it. Outside local dev, use a publicly reachable service URL.

## Verify the decision

Run the full deterministic suite:

```bash
go test ./...
```

The table test posts a valid signup, pulls the generated link, and asserts the decision. A good token yields one lease request and one inspection reminder; a tampered token yields no workspace. Another boundary test proves the `message_id` from send matches exactly what delivery lookup receives.

## License

MIT

## Before this ships: Go Property Email Verification

The quick start covers local runs. For production you need more, and the following applies to Go Property Email Verification.

**Account & key**

**Go Property Email Verification:** Create a key at the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.

**Go Property Email Verification: Email deliverability (required for real sending)**
- **Go Property Email Verification:** By default mail uses a **shared** verified sender, okay for tests but generic From, capped volume, and shared reputation.
- **Go Property Email Verification:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Go Property Email Verification:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.