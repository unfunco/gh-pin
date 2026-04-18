# gh-pin

[![CI](https://github.com/unfunco/gh-pin/actions/workflows/ci.yaml/badge.svg)](https://github.com/unfunco/gh-pin/actions/workflows/ci.yaml)
[![License: MIT](https://img.shields.io/badge/License-MIT-purple.svg)](https://opensource.org/licenses/MIT)

> [!NOTE]
> 🤖 Developed with AI assistance. On the spectrum from engineering to vibes,
> this sits a bit further toward vibes: it works well and does what I want,
> but edge cases may not all be satisfied. PRs have been reviewed for obvious
> security problems, though more subtle issues may still exist. Thankfully,
> this extension is pretty low-stakes.

GitHub CLI extension that pins GitHub Actions to commit SHAs in your workflows.
It resolves action references against a curated [pin list] and replaces mutable
tags with immutable SHAs.

## Getting started

### Installation

```bash
gh extension install unfunco/gh-pin
```

### Upgrading

```bash
gh extension upgrade gh-pin
```

Or upgrade all installed extensions at once:

```bash
gh extension upgrade --all
```

### Usage

Run `gh pin` from the root of a repository that contains a
`.github/workflows` directory:

```bash
gh pin
```

The command scans your workflow files, resolves GitHub Actions to specific
commits, and rewrites them only when something actually needs to change. It is
safe to run repeatedly.

Already-pinned actions are left alone. Actions found in the shared pin list are
updated to the curated SHA. If an action is not yet in the pin list, `gh pin`
falls back to a live GitHub lookup and can offer to open an issue so that
action can be cached for future runs.

## License

© 2026 [Daniel Morris]\
Made available under the terms of the [MIT License].

[daniel morris]: https://unfun.co
[mit license]: LICENSE.md
[pin list]: https://unfun.co/pins.json
