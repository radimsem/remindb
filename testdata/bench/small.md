---
name: Agent preferences
type: feedback
---

# Communication Style

Short responses only. No unnecessary preamble or trailing summaries.
When referencing code, include file path and line number.

# Code Review Preferences

Always check for security vulnerabilities in user input handling.
Prefer explicit error returns over panic/recover patterns.
Flag any function longer than 50 lines for potential extraction.

# Testing Guidelines

Use table-driven tests with descriptive subtest names.
Integration tests hit a real database, never mock the store layer.
Run benchmarks before and after performance-sensitive changes.
