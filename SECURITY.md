# Security policy

## Supported versions

Only the latest release receives fixes.

## Reporting a vulnerability

Please do not open a public issue. Report privately through the repository's **Security** tab → **Report a vulnerability**, with the version or commit, the input and the impact you expect.

You will get an acknowledgement within 7 days. Fixes are released before details are published, and reporters are credited unless they prefer otherwise.

## In scope

go-adf converts documents that often come from untrusted sources, such as tickets written by others or text written by a language model. In scope are inputs that make it:

- panic, hang, or use time or memory far out of proportion to the input;
- produce ADF that is not valid against Atlassian's schema;
- produce Markdown in which literal text turns into markup, for example a link, an image or raw HTML that was not in the document.
