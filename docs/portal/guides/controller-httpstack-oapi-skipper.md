# oapi/skipper

Skipper function that bypasses OpenAPI validation for operational endpoints.

## Skipped Paths

- `/metrics`
- `/health`
- `/healthz`
- `/ready`
- `/version`

Uses `ops.IsOpsPath()` internally. These endpoints typically do not have OpenAPI definitions and should not be validated or authenticated.

## Why Skip?

Ops endpoints are infrastructure-level and:

- Have no OpenAPI schema definitions
- Must remain accessible without authentication (health checks by load balancers)
- Should not trigger validation errors in logs
