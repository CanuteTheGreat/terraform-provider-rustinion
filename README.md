# terraform-provider-rustinion

A [Terraform](https://www.terraform.io/) provider for managing
[Rustinion](https://rustinion.com) monitoring configuration as code, built on
[terraform-plugin-framework](https://github.com/hashicorp/terraform-plugin-framework).
It wraps the customer-facing config-as-code API documented in
`web-api/src/routes/customer_api.rs` in the Rustinion repository — the same REST
surface a tenant's own `read_write`-scoped API key already gets, this
is just a Terraform-shaped client for it.

## Resources

| Resource | Rustinion API |
|---|---|
| `rustinion_alert_rule` | `/api/v1/customer/alert-rules[/:id]` |
| `rustinion_notification_channel` | `/api/v1/customer/notification-channels[/:id]` |
| `rustinion_dashboard` | `/api/v1/customer/dashboards[/:id]` |
| `rustinion_synthetic_check` | `/api/v1/customer/synthetic-checks[/:id]` |
| `rustinion_log_alert_rule` | `/api/v1/customer/log-alert-rules[/:id]` |
| `rustinion_integration` | `/api/v1/customer/integrations[/:id]` |
| `rustinion_status_page` | `/api/v1/customer/status-page` (singleton — no `:id`) |
| `rustinion_escalation_policy` | `/api/v1/customer/escalation-policies[/:id]` |
| `rustinion_escalation_acknowledgement` | `/api/v1/customer/escalations/:rule_kind/:rule_id/acknowledge` |
| `rustinion_on_call_schedule` | `/api/v1/customer/on-call-schedules[/:id]` |
| `rustinion_on_call_override` | `/api/v1/customer/on-call-schedules/:id/overrides[/:override_id]` |
| `rustinion_group` | `/api/v1/customer/groups[/:id]` |
| `rustinion_group_portable_policy` | `/api/v1/customer/groups/:id/portable` (per-group singleton) |
| `rustinion_group_restart_policy` | `/api/v1/customer/groups/:id/restart-policy` (per-group singleton: windows, rings, max at once, drain/verify hooks, `ignored_services` as `policy_json`) |
| `rustinion_group_alert_tuning` | `/api/v1/customer/groups/:id/alert-tuning` (per-group singleton) |
| `rustinion_group_offline_alerts` | `/api/v1/customer/groups/:id/offline-alerts` (per-group singleton) |
| `rustinion_group_host_health` | `/api/v1/customer/groups/:id/host-health` (per-group singleton) |
| `rustinion_group_custom_metrics` | `/api/v1/customer/groups/:id/custom-metrics` (per-group singleton: the agent's DogStatsD/StatsD listener and Prometheus/OpenMetrics scrape targets) |
| `rustinion_alert_routing` | `/api/v1/customer/alert-routing` (organization singleton: severities per destination - email, digest webhook/ntfy, PagerDuty - ntfy priority per severity, digest hour and time zone; push destinations default to critical only) |
| `rustinion_group_package_cleanup` | `/api/v1/customer/groups/:id/package-cleanup` (per-group singleton: remove unneeded packages / old kernels, on by default) |
| `rustinion_device_package_cleanup` | `/api/v1/customer/devices/:id/package-cleanup` (per-device on / off / inherit) |
| `rustinion_group_malware_scan` | `/api/v1/customer/groups/:id/malware-scan` (per-group singleton: signatures-only and excluded scan paths) |
| `rustinion_malware_allowlist_rule` | `/api/v1/customer/malware-allowlist` (fleet-wide false positive: signature + path pattern) |
| `rustinion_slo` | `/api/v1/customer/slos[/:id]` |
| `rustinion_silence` | `/api/v1/customer/silences[/:id]` |
| `rustinion_snmp_target` | `/api/v1/customer/groups/:id/snmp-targets`, `/api/v1/customer/snmp-targets/:id` (community / v3 keys write-only) |
| `rustinion_device_tag` | `/api/v1/customer/devices/:id/tags[/:tag]` (one tag on one device) |
| `rustinion_maintenance_window` | `/api/v1/customer/maintenance-windows[/:id]` (no update; destroy cancels) |
| `rustinion_rum_application` | `/api/v1/customer/rum-applications[/:id]` (exports the browser `client_token`) |
| `rustinion_report_schedule` | `/api/v1/customer/report-schedules[/:id]` |
| `rustinion_user` | `/api/v1/customer/users[/:id]` |
| `rustinion_permission_grant` | `/api/v1/customer/permission-grants[/:id]` |
| `rustinion_permission_role` | `/api/v1/customer/permission-roles[/:id]` |
| `rustinion_permission_role_assignment` | `/api/v1/customer/permission-roles/:role_id/assignments[/:assignment_id]` |
| `rustinion_permission_role_mapping` | `/api/v1/customer/permission-role-mappings[/:id]` |

Every resource supports create/read/update/delete and `terraform
import <resource>.<name> <id>` (the server-assigned UUID) — with a few
exceptions:
- The `rustinion_group_*` settings resources (portable policy, restart
  policy, alert tuning, offline alerts, host health, custom metrics) are per-group singletons: their
  id and import id are the group id, creating one writes the settings,
  and destroying one puts the server defaults back.
- `rustinion_status_page` is a singleton (exactly one status page per
  tenant): its import id is a placeholder, never looked up, since `GET
  /api/v1/customer/status-page` takes no id at all (see that resource's
  own doc comment).
- `rustinion_on_call_override` has no in-place update at all (every
  attribute is `RequiresReplace` — the server's own API is
  create/delete-only for an override, matching the admin UI's own
  separate add/remove actions) and its import id is `schedule_id/
  override_id`, not a bare id, since an override is only ever readable
  through its parent schedule's own `GET` (see that resource's own doc
  comment).
- `rustinion_group` has no rename or freeform manifest edit anywhere in
  this app (admin UI included) — `name` is fixed at creation, and `PUT`
  only ever (re-)applies a starter `template_key`, which this resource
  updates in place. `template_key` is never returned by the API on
  read, so this provider deliberately preserves whatever value you last
  set instead of re-deriving (and fighting you over) one the server
  can't tell it. Deleting a group only succeeds if it has no devices
  and no outstanding enrollment tokens — the same database constraints
  that protect it everywhere else in this app, surfaced as a real
  Terraform error, not silently ignored, if it still has either.
- `rustinion_slo` and `rustinion_silence` have no in-place update at
  all — every attribute is `RequiresReplace`, matching neither API
  having an update endpoint (both are create/read/delete-or-cancel
  only, mirroring their own admin UIs, which have never had an edit
  path for an existing SLO or silence either). Destroying a
  `rustinion_silence` cancels it server-side (`cancelled_at = now()`)
  rather than deleting its row, matching the admin UI's own "cancel"
  action.
- `rustinion_permission_grant` has no `PUT` endpoint behind it, but
  still updates `permission_level` in place: a grant's identity *is*
  its `(user_id, group_id, resource_type)` tuple (enforced server-side
  by two partial unique indexes), so those three are `RequiresReplace`
  and the only mutable field is the level — which `POST` already
  changes, upserting on that same scope and returning the same id.
  That is deliberate rather than incidental: Terraform would otherwise
  destroy-then-create on a level change, and for a `read` grant that
  *restricts* an admin, the window between the two would silently
  restore their unrestricted access. Reference a user with
  `rustinion_user.example.user_id`, **not** `.id` — the latter is that
  resource's `user_roles` row id, while a grant is keyed on the user
  identity. A grant for someone with no role in the tenant is rejected
  (a grant refines access, it never confers membership).
- `rustinion_permission_role_assignment` has no in-place update at all
  (every attribute is `RequiresReplace`) and its import id is
  `role_id/assignment_id`, not a bare id — an assignment has no mutable
  state, it *is* its `(role, user, scope)` tuple, and it is only ever
  readable through its parent role's own list endpoint. Destroying the
  parent `rustinion_permission_role` cascades its assignments away
  server-side, so a plan that removes both at once is not an error.
- `rustinion_permission_role_mapping` has no in-place update either —
  a mapping has no mutable state at all, it *is* its `(source,
  external_group, role_id, group_id)` tuple, so every attribute is
  `RequiresReplace` and the API has no `PUT`. `source` is one of
  `scim`, `oidc` or `saml`. Destroying one does not immediately revoke
  the role assignments it produced: those are removed on each affected
  user's next sync (a SCIM group write, or that user's next OIDC/SAML
  sign-in). That is deliberate server-side
  behavior rather than a provider limitation — revoking access outside
  a sync would do it at a moment nobody is watching and with no audit
  entry for the run.
- `rustinion_escalation_acknowledgement` is not config-as-code at all —
  it models a one-shot *action* (acknowledging a firing escalation),
  not persistent server-side state. `rule_kind`/`rule_id` are
  `RequiresReplace`; `Read` drops the resource from state the moment
  the acknowledgement it recorded no longer holds (the rule cleared and
  fired again, starting a fresh unacknowledged episode), so a later
  `apply` naturally re-acknowledges instead of going stale silently.
  `Delete` is a no-op — there is no server-side "un-acknowledge."

`PUT` on the Rustinion side is a full resource replace, not a partial
patch — see each resource's own doc comment in `internal/provider/` —
so this provider's `Update` always sends every field, matching the
plan's full desired state, same as `terraform apply` always does.
`rustinion_integration` is one exception: its `PUT` only ever updates
`config` (`kind`/`group_id` are fixed at install time — see that
resource's own doc comment), which is why both are `RequiresReplace` in
its schema. `rustinion_escalation_policy`'s `step` block fans out to
every channel in its `channel_ids` list independently — a missing/
disabled channel or a failed delivery on one never blocks the others.
`rustinion_on_call_schedule` never manages overrides (a separate
resource — see above) — see that resource's own doc comment for why.

## Requirements

- Go 1.23+ (to build the provider)
- Terraform CLI 1.5+ (to actually run `terraform plan`/`apply` against it, or to run this provider's acceptance tests, which shell out to it)
- A running Rustinion instance and a `read_write`-scoped API key for one of its tenants (Tenant admin panel -> API keys -> Read + write)

## Building

```sh
go build -o terraform-provider-rustinion .
```

## Installing

Published on the public Terraform Registry as
[`CanuteTheGreat/rustinion`](https://registry.terraform.io/providers/CanuteTheGreat/rustinion):

```hcl
terraform {
  required_providers {
    rustinion = {
      source  = "CanuteTheGreat/rustinion"
      version = "~> 0.1"
    }
  }
}
```

`terraform init` downloads it and verifies the release's GPG-signed
checksums (key `4F1D 6FD9 4A22 32E1 58AE 21DD 642A E4F0 4B94 C1F1`,
"Errant Solutions (Rustinion Terraform provider releases)").

### A local build instead

To try an unreleased change, point Terraform at a local build with a [dev
override](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers):

```hcl
# ~/.terraformrc
provider_installation {
  dev_overrides {
    "registry.terraform.io/CanuteTheGreat/rustinion" = "/absolute/path/to/terraform-provider-rustinion"
  }
  direct {}
}
```

While the override is active `terraform init` is not needed (Terraform
prints a reminder on every command; that is expected).

## Authentication

```hcl
provider "rustinion" {
  base_url = "https://rustinion.com"     # or the RUSTINION_BASE_URL env var
  api_key  = "..."                        # or the RUSTINION_API_KEY env var — sensitive
}
```

Both attributes are optional in the provider block and fall back to
`RUSTINION_BASE_URL`/`RUSTINION_API_KEY` environment variables — the
usual Terraform provider convention for credentials, so a real key
never has to live in checked-in `.tf` files. The key must be
`read_write`-scoped (`0067_api_key_scope.sql` in the main repo); a
`read_only` key is rejected by the API itself with a 403 on the very
first `apply`, surfaced as a normal Terraform error, not a provider
crash.

## Example

```hcl
# Alert content is customisable per channel. Omit these for the
# built-in message format.
resource "rustinion_notification_channel" "ops_webhook" {
  name         = "Internal incident API"
  channel_type = "webhook"
  config       = jsonencode({ url = "https://ops.example.com/incidents" })

  # {{variable}} substitution over the finding that fired. A webhook's
  # payload_template replaces the default {"subject","body","severity"}
  # envelope entirely, which is what makes it possible to POST the exact
  # shape a receiving system expects.
  subject_template = "[{{severity}}] {{reason}}"
  payload_template = jsonencode({
    title  = "{{reason}} on {{hostname}}"
    detail = "{{detail}}"
  })
}

resource "rustinion_notification_channel" "ops_slack" {
  name         = "ops-slack"
  channel_type = "slack"
  config       = jsonencode({ webhook_url = "https://hooks.slack.com/services/..." })
}

resource "rustinion_alert_rule" "high_cpu" {
  group_id = var.group_id
  name     = "High CPU"
  severity = "warning"

  condition {
    metric_type = "cpu_avg_pct"
    comparator  = "gt"
    threshold   = 90
  }
}

# A forecast condition alerts before disk_avg_pct actually crosses 90 -
# once its current linear trend is projected to reach it within
# forecast_within_hours (Datadog calls this a "Forecast Monitor").
resource "rustinion_alert_rule" "disk_capacity_forecast" {
  group_id = var.group_id
  name     = "Disk filling up"
  severity = "warning"

  condition {
    metric_type            = "disk_avg_pct"
    condition_kind         = "forecast"
    comparator             = "gt"
    threshold              = 90
    forecast_within_hours  = 48
  }
}

resource "rustinion_dashboard" "fleet_overview" {
  group_id = var.group_id
  name     = "Fleet overview"

  widget {
    title       = "CPU"
    widget_type = "kpi_tile"
    config      = jsonencode({ metric_type = "cpu_avg_pct", window_minutes = 60 })
  }
}

resource "rustinion_synthetic_check" "health" {
  group_id   = var.group_id
  name       = "Health check"
  check_type = "http"
  target     = "https://example.com/health"
}

resource "rustinion_log_alert_rule" "ssh_failures" {
  group_id     = var.group_id
  name         = "SSH login failures"
  log_severity = "error"
  log_source   = "journal"
}

resource "rustinion_integration" "nginx" {
  kind     = "nginx"
  group_id = var.group_id
  config   = jsonencode({ target = "https://example.com/health" })
}

resource "rustinion_status_page" "public" {
  slug  = "status"
  title = "Example status"

  component {
    synthetic_check_id = rustinion_synthetic_check.health.id
    display_name       = "API"
  }
}

resource "rustinion_on_call_schedule" "primary" {
  group_id      = var.group_id
  name          = "Primary rotation"
  rotation_days = 7

  member {
    user_id = var.oncall_user_id
  }
}

resource "rustinion_on_call_override" "vacation_cover" {
  schedule_id = rustinion_on_call_schedule.primary.id
  user_id     = var.covering_user_id
  starts_at   = "2026-07-01T00:00:00Z"
  ends_at     = "2026-07-08T00:00:00Z"
}

resource "rustinion_notification_channel" "ops_pagerduty" {
  name         = "ops-pagerduty"
  channel_type = "pagerduty"
  config       = jsonencode({ integration_key = "..." })
  # Paging channels (pagerduty, opsgenie, sms, voice, ntfy) get critical findings
  # only unless widened; email/slack/webhook get every severity.
  # severities = ["critical", "warning"]
}

# Emergencies to the phone, everything else in one daily email at 09:00.
resource "rustinion_alert_routing" "this" {
  email_severities       = ["warning", "info"]
  webhook_severities     = ["critical"] # e.g. an ntfy topic, set on the tenant page
  ntfy_priority_critical = "urgent"
  alert_digest_cadence   = "daily"
  summary_digest_cadence = "daily"
  digest_hour            = 9
  digest_timezone        = "America/Los_Angeles"
}

resource "rustinion_notification_channel" "ops_sms" {
  name         = "ops-sms"
  channel_type = "sms"
  config = jsonencode({
    account_sid = var.twilio_account_sid
    auth_token  = var.twilio_auth_token
    from_number = "+15551234567"
    to_numbers  = ["+15557654321"]
  })
}
```

Every channel's `config` — including secrets like the Twilio `auth_token`
above, a Slack webhook URL, a PagerDuty routing key, or a webhook HMAC
secret — is now encrypted at rest server-side (AES-256-GCM, an
app-managed key, not just database access control). This is entirely
transparent to Terraform: the wire shape is unchanged (`config` is still
plain JSON in both the request Terraform sends and the response it reads
back to refresh state), so no schema or attribute changes were needed
here.

```hcl
resource "rustinion_escalation_policy" "primary" {
  group_id            = var.group_id
  name                = "Primary escalation"
  on_call_schedule_id = rustinion_on_call_schedule.primary.id

  step {
    # Fans out to both channels at once.
    channel_ids  = [rustinion_notification_channel.ops_slack.id, rustinion_notification_channel.ops_pagerduty.id]
    wait_minutes = 15
  }
}

resource "rustinion_alert_rule" "disk_full" {
  group_id            = var.group_id
  name                = "Disk almost full"
  escalation_policy_id = rustinion_escalation_policy.primary.id

  condition {
    metric_type = "disk_avg_pct"
    comparator  = "gt"
    threshold   = 90
  }
}

# Acknowledges disk_full's currently-firing escalation, if any — a
# one-shot action, not persistent config; see this resource's own
# section below.
resource "rustinion_escalation_acknowledgement" "ack_disk_full" {
  rule_kind = "alert_rule"
  rule_id   = rustinion_alert_rule.disk_full.id
}

resource "rustinion_group" "web_servers" {
  name         = "Web servers"
  template_key = "linux_web_server"
}

resource "rustinion_slo" "fleet_uptime" {
  group_id          = rustinion_group.web_servers.id
  name              = "Fleet uptime"
  metric_type       = "checkin_freshness"
  target_percentage = 99.9
}

resource "rustinion_silence" "planned_maintenance" {
  name      = "Planned maintenance"
  group_id  = rustinion_group.web_servers.id
  starts_at = "2026-07-01T02:00:00Z"
  ends_at   = "2026-07-01T04:00:00Z"
  reason    = "Scheduled DB upgrade"
}

resource "rustinion_user" "dept_admin" {
  email        = "ops-lead@example.com"
  group_id     = rustinion_group.web_servers.id
  access_level = "admin"
}

# Fine-grained override: this user is read-only across the org (above),
# but may edit alert rules for the web servers group specifically.
resource "rustinion_permission_grant" "alert_rules_for_web" {
  user_id          = rustinion_user.dept_admin.user_id
  group_id         = rustinion_group.web_servers.id
  resource_type    = "alert_rules"
  permission_level = "write"
}

# For anything you'd grant more than once, define it as a role instead
# and assign people to it — then change the role in one place.
resource "rustinion_permission_role" "sre" {
  name        = "SRE"
  description = "On-call engineers"

  permission {
    resource_type    = "alert_rules"
    permission_level = "admin"
  }

  permission {
    resource_type    = "synthetic_checks"
    permission_level = "write"
  }
}

resource "rustinion_permission_role_assignment" "dept_admin_is_sre" {
  role_id = rustinion_permission_role.sre.id
  user_id = rustinion_user.dept_admin.user_id
  # Omit group_id for a tenant-wide assignment.
}

# Better still: let the directory decide who is an SRE. Anyone whose
# OIDC token carries the "sre" group gets the role at sign-in, and
# loses it when they leave the group. Assignments made by hand (above)
# are never touched by a sync.
resource "rustinion_permission_role_mapping" "sre_from_idp" {
  source         = "oidc"
  external_group = "sre"
  role_id        = rustinion_permission_role.sre.id
}

# Same thing for a SAML IdP — read from the assertion's group
# attribute instead of an OIDC claim. A tenant can run more than one
# source at once; each only ever manages its own assignments.
resource "rustinion_permission_role_mapping" "sre_from_saml" {
  source         = "saml"
  external_group = "sre"
  role_id        = rustinion_permission_role.sre.id
}
```

See each resource's own schema (`terraform providers schema
-json`, or the doc comments in `internal/provider/resource_*.go`) for
every attribute — `condition`/`widget`/`step`/`component`/`member`
nested blocks in particular, which mirror the Rust API's own nested
`conditions`/`widgets`/`steps`/`components`/`members` arrays field for
field.

## Running acceptance tests

This provider's acceptance tests (`internal/provider/*_test.go`,
`TestAcc*`) are real, not mocked: they run actual `terraform apply`/
`plan`/`destroy` cycles (via `terraform-plugin-testing`, which drives
this provider's own code in-process — no separate provider binary
install needed) against a real, running `rustinion-web` instance
backed by a real Postgres. They're skipped by a plain `go test ./...`
(the standard Go convention this codebase's own Rust tests follow too:
acceptance-style tests are opt-in, not run by default) unless
`TF_ACC=1` is set.

1. Start the dev Postgres (from the main repo root):
   ```sh
   docker compose up -d postgres
   ```
2. Run a real `rustinion-web` against it, migrations included (from the main repo's `web/` directory):
   ```sh
   DATABASE_URL=postgres://rustinion:devpassword@localhost:5433/rustinion PORT=8090 cargo run --bin rustinion-web
   ```
3. Bootstrap a throwaway tenant, group, `read_write` API key, and one tenant member directly in that
   database (there's no signup/API-key-creation/user-invite flow reachable without an admin session and
   a browser):
   ```sh
   eval "$(DATABASE_URL=postgres://rustinion:devpassword@localhost:5433/rustinion ./scripts/setup_test_tenant.sh)"
   export RUSTINION_BASE_URL=http://localhost:8090
   ```
4. Run the suite:
   ```sh
   TF_ACC=1 go test ./internal/provider/... -run TestAcc -v
   ```

Every acceptance test creates and destroys its own resources; nothing
persists in the tenant `setup_test_tenant.sh` created beyond the test
run itself (Terraform's own `CheckDestroy` in each test confirms the
resource is actually gone server-side afterward, not just removed from
state).

The server's per-key rate limit (`60` requests per rolling `60s`,
`web/src/rate_limit.rs`'s `customer_api_key` quota) applies here exactly
as in production. The provider waits out a `429` for the `Retry-After`
the server sends and retries (up to five minutes per call), so a full
`TestAcc*` run, like any large `terraform apply`, is slower once it drains
the bucket rather than failing partway through.

CI runs the whole acceptance suite on every push to `main` (the
`live-e2e` job in `.forgejo/workflows/ci-cd.yml`, via
`scripts/ci/live-e2e.sh` in the main repo) against a freshly built
`rustinion-web` and a throwaway Postgres.

The `internal/client` package additionally has plain, fast unit tests
(`go test ./internal/client/...`, no `TF_ACC`, no real server) covering
request construction, auth headers, and error/404 handling against an
`httptest` server — a cheap complement to, not a substitute for, the
real acceptance tests above.

## Limitations

Honest, deliberate scope cuts — not oversights:

- **Every resource the Rustinion customer API exposes is now covered**
  (alert rules, notification channels, dashboards, synthetic checks,
  log alert rules, integrations, the tenant's one status page,
  escalation policies, on-call schedules/overrides, groups, SLOs,
  silences, and users). `rustinion_on_call_schedule`'s own acceptance
  test still provisions its test user ahead of time via
  `scripts/setup_test_tenant.sh` rather than a `rustinion_user`
  resource of its own — kept that way deliberately (see that script's
  own comment) since `rustinion_user` sends a real password-set email
  on every create, which isn't a side effect an unrelated acceptance
  test for a different resource should trigger just to get a user id.
- **An escalation policy step notifies through exactly one channel** —
  the schema `escalation_steps` already shipped with (see
  `0051_on_call_and_escalation.sql` in the main repo); there's no
  fan-out to several channels at one escalation level.
- **No SMS/voice escalation channels** — `notification_channels`
  supports email/Slack/webhook/PagerDuty/Opsgenie today; an escalation
  step (or any notification channel) can only ever notify through one
  of those. PagerDuty/Opsgenie themselves can relay onward to SMS/voice
  per their own configuration, but this app has no native SMS/voice
  delivery of its own.
- **`rustinion_group` can't be renamed, and can only be deleted while
  empty** — there is no rename capability anywhere in this app, and a
  group with any devices or outstanding enrollment tokens can't be
  deleted (the same database constraints that already protect it
  everywhere else). `template_key` can be changed freely in place at
  any time — it's the one real update path a group has — but there is
  no way to remove a template's effect once applied, only to apply a
  different one.
- **`rustinion_slo` and `rustinion_silence` have no in-place update at
  all** — neither's underlying API has an update endpoint (both are
  create/read/delete-or-cancel only), matching their own admin UIs,
  which have never had an edit path for an existing SLO or silence
  either.
- **`rustinion_user`'s `email`/`group_id` are fixed at invite time** —
  there is no rename or move-to-a-different-group capability anywhere
  in this app; only `access_level` (admin/read_only) can be changed in
  place. Creating one sends the same one-time password-set email the
  admin UI's own invite does — silently skipped if the server has no
  SMTP configured, never a failure. Destroying it revokes the user's
  sessions if it was their last remaining access anywhere, matching
  the admin UI's own removal behavior.
- **One data source: `rustinion_process_watch_rules`**, looking up a
  group's process-watch rules (including ones the admin UI's own
  "Suggested rules" feature created) without managing them. Every
  other resource this API exposes is also writable — there had been
  no need for a read-only companion at all until this one.
- **One API key manages an entire tenant** — there's no per-resource-
  type scoping narrower than the existing `read_only`/`read_write`
  split (`0067_api_key_scope.sql`), so a `read_write` key configured
  for this provider can create/update/delete any resource type this
  provider manages across every group in its tenant, not just the ones
  a given Terraform configuration happens to touch.
- **Notification channel `config`, synthetic check step `headers`/
  `body`, and integration `config` are not marked `Sensitive`** — a
  Slack webhook URL, PagerDuty routing key, or an `aws_cloudwatch`
  integration's access key inside `config` ends up in Terraform state
  in plain text (marked `Sensitive` only on the notification channel's
  own `config` attribute, which redacts it from CLI output, but state
  files themselves are never encrypted by Terraform itself) — the same
  "state is sensitive" caveat that applies to any Terraform-managed
  secret, not specific to this provider, but worth calling out since
  every channel type but email, and one integration kind
  (`aws_cloudwatch`), holds one.
- **`rustinion_status_page` is a singleton** — there is no
  `rustinion_group`-style "which one" identifier, since the API itself
  has exactly one status page per tenant. Applying this resource
  against a tenant that already has a status page (created via the
  admin UI, say) overwrites it unconditionally on the next `apply`,
  matching the API's own upsert `PUT` semantics rather than failing
  with a conflict — there is no "adopt the existing page without
  changing it" import-only path.
- **No plan-time validation cross-checks between fields** beyond what
  each attribute's own `Validators` cover (e.g. an `http_multistep`
  check with a non-`"direct"` `relay_mode` is caught server-side, with
  a real Terraform error surfaced from the API's own 400 response, not
  rejected earlier at plan time by this provider itself).

See the main repo's `docs/roadmap.md` for the broader Datadog/Zabbix
competitive-gap context this provider was built to close.
