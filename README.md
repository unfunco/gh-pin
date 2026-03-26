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

Actions not found in the pin list are left unchanged.

[pin list]: https://unfunco.github.io/toolbox/pins.json
