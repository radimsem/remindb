# Security policy

`remindb` is a single-binary Go program that reads your notes and exposes them over MCP. The attack surface is small — a SQLite file, a stdio server, a handful of file parsers — but small isn't none. If you find something, please tell me before telling the world.

## Reporting a vulnerability

Two private channels, pick whichever is easier:

- **GitHub security advisory** (preferred): [open one here](https://github.com/radimsem/remindb/security/advisories/new). Keeps the discussion, the fix, and any CVE threaded in one place.
- **Email**: `security@radimsemerak.cz`. PGP welcome but not required.

Helpful to include:

- A short description and where it lives in the code.
- Steps to reproduce — a failing test or a tiny input file goes a long way.
- The version (`remindb --version`) and the platform.
- Whether you're OK being credited.

Please don't open a public issue, post in discussions, or share a screenshot publicly. The gap between public disclosure and a shipped fix is exactly when users get hurt.

## Threat model

Which transport `remindb serve` runs in determines the trust boundary.

**stdio** (default). Speaks over stdin/stdout to a single trusted parent (Claude Code, Codex, etc.). No network surface.

**HTTP loopback** (`--transport=http`, default address `127.0.0.1:7474`). Reachable only from the same host; the trust boundary is "anyone with a shell on this machine".

**HTTP non-loopback** (`--listen=0.0.0.0:*` or any reachable interface). `remindb serve` **refuses to start** unless one of:

- `REMINDB_AUTH_TOKEN=<secret>` is set — every request must carry `Authorization: Bearer <secret>`; missing or wrong tokens get a constant-time-rejected 401 with `WWW-Authenticate: Bearer realm="remindb"`.
- `--insecure-public` is passed (env: `REMINDB_INSECURE_PUBLIC=true`) — a deliberately ugly opt-out for homelab or reverse-proxy setups where you trust the network layer to gate access. Logs a startup `Warn`.

The bearer-token path is intentionally a single shared secret in an env var — enough to make accidental network exposure non-catastrophic, not a multi-tenant auth system. **mTLS, OAuth, per-user tokens, and rate limiting are explicit non-goals for the current release.** If you need them, terminate them in a reverse proxy in front of `remindb` and run it with `--insecure-public`.
