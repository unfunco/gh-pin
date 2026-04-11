# gh-pin

[![License: MIT](https://img.shields.io/badge/License-MIT-purple.svg)](https://opensource.org/licenses/MIT)

GitHub CLI extension that pins GitHub Actions to commit SHAs in your workflows.
It resolves each `uses:` reference against a curated [pin list], and replaces
mutable tags with immutable SHAs.

> [!NOTE]
> 🤖 Developed with AI assistance.

## Getting started

### Installation

```bash
gh extension install unfunco/gh-pin
```

### Usage

Run `gh pin` from the root of a repository that contains `.github/workflows/`.

```bash
gh pin
```

Actions that are already pinned are left alone. Actions found in the shared pin
list are updated to the curated SHA. If an action is not yet in the pin list,
`gh pin` resolves the ref directly from GitHub when possible and still offers to
open an issue so it can be added to the shared cache for everyone else.

## License

© 2026 [daniel morris]\
Made available under the terms of the [mit license].

[daniel morris]: https://unfun.co
[mit license]: LICENSE.md
[pin list]: https://unfunco.github.io/toolbox/pins.json
