# Engineering Principles

## 1. Purpose
This document serves as the Constitution of the Conduit Notification Platform engineering organization. It outlines our core philosophies, decision-making frameworks, and expectations for all engineers and AI assistants contributing to the codebase. It takes precedence over all other engineering documents. This is mandatory reading for anyone writing code at Conduit.

## 2. Engineering Philosophy
Our core engineering philosophy is built on five pillars:

*   **Simplicity over cleverness:** Code is for humans to read and machines to execute. Clever code is unmaintainable code. We prefer explicit, easy-to-follow logic over highly abstracted, magic-filled frameworks.
*   **Maintainability over abstraction:** Abstractions carry a cognitive load. Do not abstract unless you are solving a concrete, existing problem of duplication that genuinely hinders velocity. Duplication is often cheaper than the wrong abstraction.
*   **Operational excellence over theoretical perfection:** We are building a notification platform that must reliably deliver messages. How the system runs in production, how it degrades, and how we monitor it is more important than theoretical purity.
*   **Boring technology is good technology:** We use Go, PostgreSQL, Redis, and standard HTTP. We use well-understood tools because their failure modes are known. We innovate in the product domain, not in the infrastructure domain.
*   **Ship it, then iterate:** Perfect is the enemy of shipped. Deliver a safe, working subset of a feature, merge it, observe it in production, and iterate based on real feedback.

## 3. Decision Making Principles
Engineering is a continuous series of trade-offs. Use the following guidelines to navigate them:

*   **When to build vs buy:** Buy if it's a commodity (e.g., identity providers, standard managed databases). Build if it provides a core competitive advantage for the Conduit platform (e.g., specific notification routing logic).
*   **When to abstract vs duplicate:** Apply the Rule of Three. Write it out the first time. Copy and paste it the second time. Abstract it the third time—but only if the domain logic is fundamentally identical, not just coincidentally similar.
*   **When to optimize vs ship:** Write correct code first. Measure performance only if there is a known latency or throughput requirement, or if telemetry reveals a bottleneck. Premature optimization is the root of all evil.
*   **How to evaluate trade-offs:** Every architectural proposal or PR must answer: 
    1. What is the blast radius if this fails? 
    2. What is the operational cost (on-call burden, monitoring)? 
    3. How easily can this decision be reversed?
*   **Reversible vs irreversible decisions:** Make reversible decisions quickly and locally. Make irreversible decisions (e.g., database schema choices, public API contracts, core dependency changes) slowly, deliberately, and with broad consensus (ADRs required).

## 4. Code Quality
*   **Readability is the #1 priority:** The author of a piece of code spends hours writing it; dozens of engineers will spend hundreds of hours reading it over its lifetime. Optimize for the reader.
*   **Code is read 10x more than it is written:** Variable names must be descriptive. Functions should do exactly what their names imply. Side effects must be explicit.
*   **Every abstraction must justify its existence:** If an interface has only one implementation (and isn't explicitly used for functional struct mocking in tests), it should not exist.
*   **Delete code freely:** Unused code is a liability, not an asset. If it's dead, delete it. Git has the history if we ever need it back.
*   **No clever tricks:** Avoid bitwise operations unless strictly necessary for performance in a critical path. Avoid reflection in Go. Avoid `unsafe`. Code should be boring.

## 5. Technical Debt Philosophy
*   **All debt must be tracked:** If you take on debt, you must create a Jira ticket or GitHub issue explicitly tracking it, tagged with `tech-debt`.
*   **Intentional debt is acceptable:** We sometimes take on debt to meet a critical market window. This must come with a documented repayment plan (e.g., "We will refactor this in Q3").
*   **Unintentional debt is a code review failure:** Sloppy code, missing tests, or poor variable naming is not "technical debt"—it's a failure of our quality controls.
*   **Debt compounds—pay it early:** Follow the Boy Scout Rule. Always leave the codebase cleaner than you found it. Dedicate 20% of your sprint capacity to addressing technical debt and improving infrastructure.

## 6. Documentation Philosophy
*   **Code should be self-documenting:** The structure, variable names, and types should explain *what* the code does. 
*   **Comments explain WHY, not WHAT:** Use comments to explain the business reason behind a strange rule, or why a specific trade-off was chosen (e.g., `// Using a 5s timeout here because downstream service X drops connections silently after 6s`).
*   **ADRs for significant decisions:** Any change to architecture, data models, or core library additions must have an Architecture Decision Record (ADR) in `docs/architecture/decisions`.
*   **READMEs for every deployable:** Every module or service must have a README detailing how to run it, test it, and deploy it.
*   **API docs via OpenAPI:** HTTP handlers must be documented via OpenAPI specifications. The code is the source of truth, and the spec must reflect it.

## 7. Code Review Expectations
*   **Focus on correctness, clarity, and consistency:** Code reviews are for finding bugs, ensuring business logic is sound, and making sure the code is readable.
*   **Nit-pick formatting only if it affects readability:** Let `gofmt` and `golangci-lint` handle style. Do not block PRs on subjective style preferences.
*   **Approve with comments:** Use "Approve with comments" (or "Nit:") for non-blocking suggestions. Trust the author to address them before merging.
*   **Block only for:** Correctness issues, security vulnerabilities (e.g., logging PII, SQL injection), or blatant architecture violations (e.g., domain layer depending on transport layer).
*   **Review within 24 hours:** PRs are blocking another engineer's work. Prioritize reviewing code over writing new code.

## 8. Ownership Expectations
*   **You build it, you own it, you run it:** Engineers are responsible for the code they write from conception to production operations.
*   **Domain ownership:** Every domain package (`internal/user`, `internal/notification`) must have a clear code owner defined in `CODEOWNERS`.
*   **On-call responsibilities:** If you write backend code, you will eventually go on call. You are responsible for the health of the system you built.
*   **Incident response:** Blame processes, not people. Incidents require a blameless post-mortem focused on systemic improvements, missing alerts, or testing gaps.

## 9. Production Mindset
*   **Every change must be deployable:** Main branch is always deployable. If it's merged, it's ready for production.
*   **Feature flags over long-lived branches:** Merge unfinished work behind feature flags. This prevents merge conflicts and allows safe, continuous integration.
*   **Backward compatibility by default:** Never break an existing API contract or database schema without a phased migration plan (expand/contract pattern).
*   **Rollback plans for every deploy:** Before hitting merge, know exactly how to revert the change if things go wrong. If a database migration is involved, ensure it is backward compatible with the previous application version.
*   **Monitor before, during, and after:** Add Prometheous metrics (`conduit_`) and OpenTelemetry spans for new features. Observe your changes in production dashboards immediately after deployment.

## 10. AI Usage Guidelines
*   **AI-generated code is held to the same standard:** AI (like Copilot, Gemini, or Claude) is an assistant, not a senior engineer. You are entirely responsible for the code it generates.
*   **AI must read and follow engineering standards:** When using agents, ensure they have access to this document and `architecture-principles.md`.
*   **AI must not introduce dependencies:** Never let AI introduce a new Go module or third-party dependency without explicit, human-reviewed justification.
*   **AI code must include tests:** If AI generates business logic, it must also generate table-driven tests for that logic.
*   **Human review is always required:** Never blindly merge AI-generated PRs.
*   **AI should follow established patterns:** AI must mimic the DI patterns, error handling (`DomainError`, etc.), and flat package structures already present in the codebase.

## 11. Anti-Patterns
*   **The God Object:** Having a `common` or `utils` package that becomes a dumping ground for unrelated functions. Group by domain, not by function type.
*   **Logging PII:** Never log passwords, API keys, full email addresses, or unmasked phone numbers. Our `zerolog` configuration drops these, but don't attempt to log them.
*   **Catch-All Error Masking:** Wrapping an error with `InfraError` without logging the original underlying error. The 500 response masks it from the user, but we MUST have the original error in our logs.
*   **Global State:** Using global variables for configurations or database connections. Always use explicit constructor injection (`NewService(repo)`).
*   **Panic-Driven Development:** Using `panic()` for control flow or unexpected input. Only panic during `init()` if the application literally cannot start (e.g., missing database connection). Use normal error returns everywhere else.

## 12. Checklist
Before submitting a PR, run through this checklist:
- [ ] Is the code readable without knowing the entire context?
- [ ] Are all new dependencies absolutely necessary?
- [ ] Are edge cases tested via table-driven tests?
- [ ] Have I avoided logging any PII?
- [ ] Are errors correctly mapped to `DomainError`, `ValidationError`, or `InfraError`?
- [ ] Is telemetry (metrics/spans) updated if necessary?
- [ ] Is the `X-Tenant-ID` context correctly passed through to the repository layer?
- [ ] Will this change break backward compatibility? If so, is there an expand/contract migration?
