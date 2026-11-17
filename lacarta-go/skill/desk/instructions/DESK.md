# Desk context (read by every reviewer)

Edit this file to tell every reviewer what they should know about your projects: the stack, the conventions that
matter, where the sensitive parts are, the language comments should be written in, and anything your team wants
reviews to stay away from.

Until you do, reviewers follow these defaults:

- Review the **change**, not the whole repository: start from the diff, read surrounding code only to judge it.
- Prefer a few findings you can stand behind over many guesses. Every finding cites `file:line` at the head commit.
- Say what is good in one line when it is true; never invent problems to fill a review.
- Write comments in the language of the merge request's description; keep code, identifiers and tool names as they are.
