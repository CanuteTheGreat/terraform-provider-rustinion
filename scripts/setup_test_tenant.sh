#!/usr/bin/env bash
# Bootstraps a throwaway tenant, group, admin-scoped API key, one
# tenant member, and one enrolled device directly in a running
# rustinion-web database — the acceptance test fixture this provider's
# real, non-mocked acceptance tests (internal/provider/*_test.go, gated
# behind TF_ACC=1) authenticate with. There's no signup/API-key-creation
# flow reachable without an admin session and a browser, so this goes
# straight to the database with the exact same hashing scheme
# customer_api.rs itself uses (plain SHA-256 hex — see web/src/tokens.rs's
# own `hash_token`), the same way a real admin's "Create API key" button
# would populate the row, just without the UI in between. The tenant
# member exists purely so rustinion_on_call_schedule/rustinion_on_call_
# override acceptance tests have a real user_id to reference — inserted
# directly rather than via the real rustinion_user resource deliberately:
# creating one for real sends a genuine password-set email (silently
# skipped only when the server has no SMTP configured at all), a side
# effect an unrelated acceptance test for a different resource has no
# business triggering just to get a user id. The device exists purely so
# rustinion_synthetic_check's own `location` block has a real device_id
# to reference — real device enrollment needs an agent + enrollment
# token, a genuinely separate, heavier flow not worth triggering just for
# this fixture, same reasoning as the user row above.
#
# Usage: DATABASE_URL=postgres://rustinion:devpassword@localhost:5433/rustinion ./scripts/setup_test_tenant.sh
# Prints `export` lines to stdout — eval them, e.g.:
#   eval "$(DATABASE_URL=... ./scripts/setup_test_tenant.sh)"
set -euo pipefail

DATABASE_URL="${DATABASE_URL:-postgres://rustinion:devpassword@localhost:5433/rustinion}"

RAW_KEY=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
KEY_HASH=$(printf '%s' "$RAW_KEY" | sha256sum | cut -d' ' -f1)
SUFFIX=$(head -c 8 /dev/urandom | od -An -tx1 | tr -d ' \n')

TENANT_ID=$(psql "$DATABASE_URL" -qtA -c \
  "insert into tenants (name) values ('tf-acc-test-${SUFFIX}') returning id;")

GROUP_ID=$(psql "$DATABASE_URL" -qtA -c \
  "insert into groups (tenant_id, name) values ('${TENANT_ID}', 'tf-acc-test') returning id;")

psql "$DATABASE_URL" -qtA -c \
  "insert into api_keys (tenant_id, name, key_hash, scope) values ('${TENANT_ID}', 'tf-acc-test', '${KEY_HASH}', 'admin');" \
  > /dev/null

USER_ID=$(psql "$DATABASE_URL" -qtA -c \
  "insert into users (email) values ('tf-acc-test-${SUFFIX}@example.com') returning id;")
psql "$DATABASE_URL" -qtA -c \
  "insert into user_roles (user_id, tenant_id, group_id) values ('${USER_ID}', '${TENANT_ID}', null);" \
  > /dev/null

DEVICE_ID=$(psql "$DATABASE_URL" -qtA -c \
  "insert into devices (tenant_id, group_id, device_secret_hash, hostname, last_seen_at)
   values ('${TENANT_ID}', '${GROUP_ID}', 'x', 'tf-acc-test-device-${SUFFIX}', now()) returning id;")

echo "export RUSTINION_API_KEY=${RAW_KEY}"
echo "export RUSTINION_TEST_TENANT_ID=${TENANT_ID}"
echo "export RUSTINION_TEST_GROUP_ID=${GROUP_ID}"
echo "export RUSTINION_TEST_USER_ID=${USER_ID}"
echo "export RUSTINION_TEST_DEVICE_ID=${DEVICE_ID}"
