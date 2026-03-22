# Runbook: JWT Key Rotation

## Overview

GoGate supports zero-downtime JWT key rotation by accepting multiple active signing keys
identified by `kid` (Key ID). This runbook describes how to rotate keys safely.

## Prerequisites

- Access to update the gateway config or environment variables
- Ability to restart gateway instances (rolling restart)
- The new key must be at least 32 bytes for HS256

## Procedure

### Step 1: Generate a new key

```bash
# Generate a 64-byte random key (base64-encoded for storage)
openssl rand -hex 32
```

### Step 2: Add the new key to config (non-primary)

Update `config.yaml` (or environment variables) to include both keys:

```yaml
jwt:
  keys:
    - kid: key-2026-q1           # Existing key (still primary)
      kty: oct
      value: ${JWT_SIGNING_KEY}
      primary: true
    - kid: key-2026-q2           # New key (not yet primary)
      kty: oct
      value: ${JWT_NEW_SIGNING_KEY}
      primary: false
```

### Step 3: Deploy the config change

Perform a rolling restart of all gateway instances. After this step, all instances
can **verify** tokens signed with either key, but the auth service is still issuing
tokens with the old key.

```bash
# Kubernetes
kubectl rollout restart deployment/gogate

# Docker Compose
docker compose up -d --force-recreate gateway
```

### Step 4: Update the auth service to sign with the new key

Switch your auth/token service to issue JWTs with:
- `kid: key-2026-q2` in the JWT header
- The new signing secret

Existing tokens signed with the old key remain valid until they expire.

### Step 5: Wait for old tokens to expire

Wait at least one full token lifetime (e.g. 1 hour if your tokens have `exp: +1h`).
During this window, both keys are active and both old and new tokens are accepted.

### Step 6: Remove the old key

Update the config to remove the retired key and mark the new key as primary:

```yaml
jwt:
  keys:
    - kid: key-2026-q2
      kty: oct
      value: ${JWT_NEW_SIGNING_KEY}
      primary: true
```

Deploy with a rolling restart.

### Step 7: Rotate environment variables

Remove the old `JWT_SIGNING_KEY` env var and rename `JWT_NEW_SIGNING_KEY` to
`JWT_SIGNING_KEY` for consistency.

## Rollback

If the new key causes issues:
1. Revert the auth service to sign with the old key/kid
2. Revert the gateway config to the previous version (old key as primary)
3. Rolling restart

No tokens are invalidated during rollback — the old key was never removed.

## Verification

```bash
# Check gateway health
curl -s http://gateway:8080/health | jq .

# Test with a token signed by the new key
curl -s -H "Authorization: Bearer <new-token>" http://gateway:8080/api/v1/users/me

# Check logs for auth errors
kubectl logs -l app=gogate --tail=100 | grep -i "invalid token"
```
