---
description: Execute the cfdev Terraform provider implementation plan one task at a time
---

# Implement cfdev Terraform provider

Execute the implementation plan in `specs/tasks/` sequentially, beginning with
`specs/tasks/001.md` and ending with `specs/tasks/016.md`.

Before starting, confirm that the repository directory has been renamed to
`terraform-provider-cfdev`. If it has not, stop and report that the user must rename it
before implementation begins. Treat the renamed repository as canonical and use
`adrianhall/cfdev` consistently.

For each task:

1. Read the task file and the relevant sections of `specs/d1-migrations-apply.md`.
2. Validate current repository state, branch, outstanding changes, tool versions, and
   the acceptance criteria from the previous task.
3. Write failing tests first for the task's seam (TDD).
4. Implement the smallest coherent change.
5. Run the seam tests and confirm they pass green.
6. Run formatting, lint/static checks, validation, the complete test suite, and coverage.
   Target at least 80% line coverage for provider code and document any exclusions.
7. Review the diff and create the conventional commit specified by the task.
8. Report the commit, validation commands, coverage result, and any blockers before
   proceeding.

Do not skip tests, quality gates, coverage reporting, or conventional commits. Do not
silently weaken an acceptance criterion. If a gate cannot pass, stop and report the
failure and the decision needed.

After each task is complete, proceed to the next numbered task unless a blocker or
material ambiguity requires user input. The final two tasks must remain in order:
`015.md` implements GitHub PR/release CI/CD, and `016.md` publishes the five-phase
GitHub Pages documentation site and adds release-triggered documentation builds.
