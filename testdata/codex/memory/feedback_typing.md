---
name: Type annotation preferences
description: User corrections on typing patterns and Pydantic usage
type: feedback
---

# Type Annotation Feedback

Never use `dict[str, Any]` for API responses. Always define a Pydantic model, even for simple payloads.

**Why:** A vendor changed their API response shape silently. The `dict[str, Any]` extractor kept running with missing fields, producing null columns in Snowflake that went unnoticed for two weeks.

**How to apply:** Every vendor API endpoint gets a Pydantic `BaseModel` in `schemas/`. Extractors parse responses through the model immediately after HTTP fetch. Validation errors go to the dead letter queue with the raw payload attached.

---

Use `typing.Protocol` instead of ABC for dependency boundaries between operators and loaders.

**Why:** ABC forces inheritance and makes testing harder. Protocol allows structural subtyping — any object with the right methods satisfies it without inheriting from anything.

**How to apply:** Define Protocols in the consumer package. Operators and loaders implement them implicitly. Test doubles are plain classes that match the Protocol shape.
