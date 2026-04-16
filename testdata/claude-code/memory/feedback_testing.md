---
name: Testing approach feedback
description: User corrected testing approach — integration tests over mocks, no snapshot tests
type: feedback
---

Do not use snapshot tests for React components. User considers them brittle and low-signal.

**Why:** Snapshot tests broke on every Tailwind class change, creating noise in PRs without catching real bugs.

**How to apply:** Test component behavior (renders correct text, handles click) not structure. Use `screen.getByRole` and `userEvent`, not `toMatchSnapshot`.

---

Prefer integration tests that hit the real database over unit tests with mocked queries.

**Why:** Mocked query tests passed while a real migration broke the checkout flow in production.

**How to apply:** Use a test database with `beforeEach` truncation. Only mock external services (Stripe, email providers), not internal dependencies.
