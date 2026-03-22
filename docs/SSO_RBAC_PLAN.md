# SSO and RBAC Plan

## Purpose

This document defines the intended enterprise authentication and authorization posture for AgentFabric.

Use it to answer:

- what is implemented today
- what the enterprise target state is
- which flags control rollout
- how roles should be used during deployment and operations

## Current State

Implemented today:

- local password login
- OIDC login flow
- JWT session cookies
- role checks for `admin`, `editor`, and `viewer`
- self-service ABAC path for user updates

## Enterprise Target

Recommended enterprise target:

- OIDC required in production
- local password login disabled after SSO cutover
- break-glass admin credential retained only under operational approval
- role mapping owned by identity and platform administrators

## Deployment Flags

- `AF_SSO_REQUIRED=true`
- `AF_PASSWORD_LOGIN_DISABLED=true`
- `AF_OIDC_ISSUER`
- `AF_OIDC_CLIENT_ID`
- `AF_OIDC_CLIENT_SECRET`
- `AF_OIDC_REDIRECT_URI`
- optional `AF_OIDC_LOGOUT_URL`

## Role Model

### `admin`

- policy, pricing, key, and user administration
- release and operational governance changes
- platform-level configuration review

### `editor`

- operational read or write where explicitly allowed
- governance workflow support without full platform ownership

### `viewer`

- read-only operational visibility
- audit, trace, and cost review

## Recommended Rollout Plan

1. configure OIDC in staging
2. validate login, callback, logout, and refresh
3. map a pilot admin group and viewer group
4. set `AF_SSO_REQUIRED=true`
5. set `AF_PASSWORD_LOGIN_DISABLED=true`
6. retain break-glass credentials in the vault, not in shared runbooks

## Break-Glass Guidance

Break-glass access should:

- be tightly controlled
- be stored in the approved secret-management path
- be reviewed after every use
- never be treated as the normal admin path

## Production Recommendation

For serious production deployment:

- prefer OIDC-backed access
- restrict local password login
- document role ownership and role mapping
- include auth posture in every release review

## Related Documents

Use this plan with:

- [PRODUCTION_CHECKLIST.md](PRODUCTION_CHECKLIST.md)
- [REFERENCE_DEPLOYMENT.md](REFERENCE_DEPLOYMENT.md)
- [INSTALL_SINGLE_TENANT.md](INSTALL_SINGLE_TENANT.md)
- [INSTALL_MULTI_TENANT.md](INSTALL_MULTI_TENANT.md)
