# SSO And RBAC Plan

## Current State

Implemented:

- local password login
- OIDC login flow
- JWT session cookies
- role checks for `admin`, `editor`, `viewer`
- self-service ABAC path for user updates

## Enterprise Target

- OIDC required in production
- local password login disabled after SSO cutover
- break-glass admin credential retained only under operational approval
- role mapping owned by identity / platform admins

## Deployment Flags

- `AF_SSO_REQUIRED=true`
- `AF_PASSWORD_LOGIN_DISABLED=true`
- `AF_OIDC_ISSUER`
- `AF_OIDC_CLIENT_ID`
- `AF_OIDC_CLIENT_SECRET`
- `AF_OIDC_REDIRECT_URI`
- optional `AF_OIDC_LOGOUT_URL`

## Role Model

- `admin`
  - policy, pricing, key, and user administration
- `editor`
  - operational read/write where explicitly allowed
- `viewer`
  - read-only operational visibility

## Rollout Plan

1. configure OIDC in staging
2. validate login, callback, logout, and refresh
3. map a pilot admin group and viewer group
4. set `AF_SSO_REQUIRED=true`
5. set `AF_PASSWORD_LOGIN_DISABLED=true`
6. retain break-glass credentials in the vault, not in shared runbooks
