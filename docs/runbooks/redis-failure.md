# Runbook: Redis Failure

## Overview

GoGate uses Redis for sliding-window rate limiting. When Redis becomes unavailable,
the gateway's behaviour depends on the `rate_limit.fail_open` configuration.

## Failure Modes

| `fail_open` | Redis Down Behaviour | `/ready` Response |
|---|---|---|
| `true` (default) | Rate limiting disabled; all traffic allowed through | 200 OK |
| `false` | Rate-limited routes return 503; `skip_auth` routes unaffected | 503 Service Unavailable |

## Detection

### Symptoms
- `rate limiter unavailable` in gateway logs (fail-closed mode)
- `X-RateLimit-*` response headers missing
- `/ready` endpoint returns 503 (fail-closed mode)
- Prometheus: `gateway_upstream_errors_total` may not change, but rate limit headers vanish

### Confirm Redis is down

```bash
# Direct Redis check
redis-cli -h <redis-host> -p 6379 ping

# Check gateway readiness
curl -s http://gateway:8080/ready | jq .

# Check gateway logs
kubectl logs -l app=gogate --tail=50 | grep -i redis
```

## Response Procedures

### Fail-Open Mode (default)

**Impact:** Low. Traffic flows normally. Rate limiting is temporarily disabled.

1. **Acknowledge** — Rate limiting is off. Tenants/IPs can exceed their RPM limits.
2. **Investigate Redis** — Check Redis pod/container health, memory, connectivity.
3. **Restore Redis** — Restart the Redis instance or failover to replica.
4. **Verify** — Rate limit headers reappear in responses:
   ```bash
   curl -sI http://gateway:8080/api/v1/users | grep X-RateLimit
   ```
5. **Monitor** — Watch for traffic spikes that occurred during the outage window.

### Fail-Closed Mode

**Impact:** High. All rate-limited routes return 503.

1. **Acknowledge** — Routes with `rate_limit_rpm > 0` are rejecting all traffic.
2. **Immediate mitigation options:**
   - **Option A:** Restore Redis as fast as possible.
   - **Option B:** Temporarily switch to `fail_open: true` and redeploy:
     ```yaml
     rate_limit:
       fail_open: true
     ```
   - **Option C:** Set `rate_limit_rpm: 0` on critical services to bypass rate limiting.
3. **Restore Redis** and revert any temporary config changes.
4. **Post-incident:** Evaluate whether `fail_open: true` should be the default.

## Common Redis Issues

| Issue | Diagnosis | Fix |
|---|---|---|
| Connection refused | `redis-cli ping` fails | Check Redis process is running |
| OOM killed | `dmesg` / container events show OOM | Increase `maxmemory` or scale Redis |
| Network partition | Gateway logs show timeout errors | Check network/security groups between gateway and Redis |
| Auth failure | `NOAUTH` in logs | Verify `REDIS_PASSWORD` env var matches Redis `requirepass` |
| Slow commands | Redis `SLOWLOG GET` shows entries | Check for blocking commands; consider Redis Cluster |

## Prevention

- **Always run Redis with `maxmemory` and `maxmemory-policy`** — GoGate's rate limit keys
  use `allkeys-lru` eviction safely since expired keys are cleaned up by TTL.
- **Use Redis Sentinel or Cluster** for high availability in production.
- **Monitor Redis** — Set alerts on memory usage, connection count, and latency.
- **Use `fail_open: true`** unless your compliance requirements mandate fail-closed.
- **Test Redis failure** regularly in staging to validate expected behaviour.

## Verification After Recovery

```bash
# 1. Redis is responding
redis-cli -h <redis-host> ping
# Expected: PONG

# 2. Gateway readiness
curl -s http://gateway:8080/ready | jq .
# Expected: {"status":"ok","service":"api-gateway"}

# 3. Rate limit headers present
curl -sI http://gateway:8080/api/v1/auth/login | grep X-RateLimit
# Expected: X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset
```
