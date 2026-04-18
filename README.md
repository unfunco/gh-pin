# gh-pin

[![CI](https://github.com/unfunco/gh-pin/actions/workflows/ci.yaml/badge.svg)](https://github.com/unfunco/gh-pin/actions/workflows/ci.yaml)
[![License: MIT](https://img.shields.io/badge/License-MIT-purple.svg)](https://opensource.org/licenses/MIT)

> [!NOTE]
> 🤖 Developed with AI assistance.

GitHub CLI extension that pins GitHub Actions to commit SHAs in your workflows.
It resolves each GitHub Actions reference against a curated [pin list], and
replaces mutable tags with immutable SHAs.

## Getting started

### Installation

```bash
gh extension install unfunco/gh-pin
```

### Usage

Run `gh pin` from the root of a repository that contains a `.github/workflows`
directory. It will scan all workflow files for GitHub Actions and attempt to pin
them to specific commit SHAs.

```bash
gh pin
```

The command is safe to run repeatedly. Workflow files are only rewritten when an
action reference needs to change.

Actions that are already pinned are left alone. Actions found in the shared pin
list are updated to the curated SHA. If an action is not yet in the pin list,
`gh pin` resolves the ref directly from GitHub when possible and still offers to
open an issue so it can be added to the shared cache for everyone else.

## License

© 2026 [Daniel Morris]\
Made available under the terms of the [MIT License].

[daniel morris]: https://unfun.co
[mit license]: LICENSE.md
[pin list]: https://unfun.co/pins.json
