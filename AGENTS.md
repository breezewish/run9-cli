# AGENTS.md

## Critical Engineering Rules

This project requires extremely high code quality and maintainability. Best engineering practices MUST BE followed at all times. There is zero tolerance for sloppy, unclear, or over-engineered code, once discovered it MUST BE refactored immediately.

The rules below are some typical principles that you **MUST follow**. They are not exhaustive, and you must always use your best judgment to **write the cleanest code possible**.

### Core Principles: Simplicity & Readability

- Boring Code - Obvious, self-explanatory > clever, minimize cognitive load
- Single Responsibility - One function, one job
- Only What's Used - No future-proofing, delete dead code immediately
- Explicit over Implicit - Clear is better than concise
- Meaningful Abstractions - Only when they reduce cognitive load
- Keep DRY - Only if it does not conflict with the above principles
- Avoid reinventing the wheel - Use _reliable_ 3rd-party libraries wisely to reduce code complexity

### Better Maintainability

- Don't treat "looks similar" as "equivalent"
- Abstractions must be meaningful
- Prefer certainty, single source of truth - e.g. don't introduce "optional" unless absolutely necessary

### How to Name Symbols

- Write down 5+ name candidates
- Consider: verb clarity, noun specificity, context
- Choose the name that needs the least explanation

### Language Idioms (Golang)

Best Go idioms must be followed, listed a few most important ones here:

- Prefer channels over mutexes
- Prefer structured concurrency to properly manage goroutine lifecycles (e.g. waitgroup / errgroup)
- Use Go's context as much as possible

### Project Specific Rules

3rd Party Libraries:

- If the library to use is not widely adopted or generally well-known, audit its code first

Avoid over-engineering:

- Remove a-few-lines wrapper functions, like small getters/setters
- Remove meaningless nil checks

Tests:

- Use `testify/require` for assertions
- Treat `go vet ./...` as a blocking check. Every change must keep the repository `go vet`-clean on the CI target; when developing on a non-Linux host, explicitly run `GOOS=linux GOARCH=amd64 go vet ./...` before finishing so Linux-only packages/tests cannot silently break GitHub CI.
- Prefer simple and expanded test assertions over dynamic ones (even if it means more lines of code and not DRY)

## Refactor and Simplification

This project should be continously refactored and simplified by following first principles, to keep the codebase clean and maintainable.
All historical burdens should be removed, also including backwards compatibility code, temporary hacks, old data migration, etc.
Any stale features or stale APIs should be also cleaned up.

When a refactor is completed, you should proactively push local changes to the remote repo to make it visible as soon as possible.
