---
description: Implement a cfdev Terraform provider GitHub issue
---

# Implement cfdev Terraform provider issue

Implement GitHub issue `$ARGUMENTS`. Invoke this command as `/issue <issue-number>`,
for example `/issue 1`.

## Load and validate the issue

1. Require exactly one positive integer issue number. If it is missing or invalid, stop
   and show the `/issue <issue-number>` usage.
2. Read the issue from GitHub with:

   ```sh
   gh issue view "$ARGUMENTS" --json number,title,body,state,labels,url
   ```

   Stop if the issue cannot be read or is closed.
3. Read relevant sections of `specs/d1-migrations-apply.md` when the issue concerns D1
   migration behavior.
4. Confirm the repository directory is named `terraform-provider-cfdev`. If it is not,
   stop and report that the user must rename it before implementation begins. Use
   `adrianhall/cfdev` consistently.
5. Validate repository state, current branch, outstanding changes, tool versions, and
   applicable prerequisites or acceptance criteria from completed preceding issues.
   Stop and ask for direction rather than overwriting unrelated work.

## Implement the issue

1. Create a branch named `issues/<number>` from the repository's default branch, unless
   the current branch already belongs to this issue.
2. Write failing tests first for the issue's seam (TDD).
3. Implement the smallest coherent change that meets every acceptance criterion.
4. Run seam tests and confirm they pass green.
5. Write any additional unit tests required to meet the quality gate of at least 80% line
   coverage for provider code.
6. Run formatting, lint/static checks, validation, the complete test suite, and
   coverage. Target at least 80% line coverage for provider code and document any
   exclusions.
7. Review the complete diff.
8. Create the conventional commit specified by the issue. Include `Refs #<number>` in
   its commit body.

Do not skip tests, quality gates, coverage reporting, or conventional commits. Do not
silently weaken an acceptance criterion. If a gate cannot pass, stop and report the
failure and the decision needed.

## Create the pull request

Immediately after the conventional commit succeeds:

1. Push the branch: `git push -u origin HEAD`.
2. Create a GitHub pull request using `gh pr create` with:
   - title: `(#<issue-number>) <GitHub issue title>`
   - a body summarizing the implementation, validation commands, coverage result, and
     any exclusions or follow-ups
   - a separate closing instruction: `Closes #<issue-number>`
3. Report the issue URL, commit SHA, pull-request URL, validation commands, coverage
   result, and blockers.

Run one issue per invocation; do not automatically begin another issue.
