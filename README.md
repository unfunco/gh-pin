# gh-pin

GitHub CLI extension that pins GitHub Actions to commit SHAs in your workflows.
It resolves each `uses:` reference against a curated [pin list], and replaces
mutable tags with immutable SHAs.

## Installation

```bash
gh extension install unfunco/gh-pin
```

## Usage

```bash
gh pin
```

If an action is not yet in the pin list, `gh pin` falls back to resolving the
ref directly from GitHub when possible, and still offers to open an issue so it
can be added to the shared cache for faster future runs.

[pin list]: https://unfunco.github.io/toolbox/pins.json
