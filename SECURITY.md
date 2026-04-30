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
