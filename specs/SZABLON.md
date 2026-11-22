# SNNN — Change title

- **Status:** draft | closed | delivered | abandoned
- **Date:** YYYY-MM-DD
- **Related:** SNNN, …

## 1. TL;DR

One to three sentences: what changes, for whom, and how we will confirm the result.

## 2. Abstract

3 to 15 sentences in paragraphs:

- current state,
- gap or problem,
- rationale for the change,
- decision history.

## 3. Objective

- **Goals:** what must be achieved.
- **Out of scope:** what we deliberately do not do.
- **Success criteria:** conditions that mean the change is finished.

## 4. Artefact Inventory

| Artifact | Kind | State |
| --- | --- | --- |
| `path/file` | code / data / document / external | exists / to create / to change / to delete / [TBD] |

## 5. Functional Domain Changes

- current state and target state,
- rules and edge cases,
- resilience to external service failures and concurrency,
- impact on data.

## 6. Critical Path

The shortest sequence of steps from the current state to a working change. Ends with the completion condition.

## 7. Control Gates

| # | Gate | Pass condition | Verification | Blocks |
| --- | --- | --- | --- | --- |
| G1 | … | logical or numeric condition | test / command | commit / release |

Also include negative gates, i.e. what the system must not do.

## 8. Expected Outcome

- direct effect,
- measurable effect,
- side effects,
- ability to roll back the change.

## 9. Additional Information

- rejected alternatives,
- technical debt,
- external dependencies,
- sources and open questions.
