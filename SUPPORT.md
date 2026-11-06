# Getting help

ship-it has a single maintainer. There is no paid support and no guaranteed response time.

## Before asking

1. Run `ship doctor`: it checks git (2.43 or later), your identity, the repository, the remote and your GitHub token, and prints the exact command that fixes what it finds.
2. Run `ship fix` for the problems the playbook knows how to repair (`docs/reference/error-playbook.md`). Everything it does can be undone with `ship undo`.
3. Run `ship trace` to see the steps of the last run and where it stopped.

## Where to ask

| You have | Go to |
|---|---|
| A question, or an idea that is not a concrete request yet | [Discussions](https://github.com/GregoireF/ship-it/discussions) |
| A bug, a feature request, a documentation problem | [an issue](https://github.com/GregoireF/ship-it/issues/new/choose) (the forms ask for what we need) |
| A security vulnerability | **never a public issue**: see [SECURITY.md](SECURITY.md) |
| A code of conduct concern | see [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) |

When you report a bug, include `ship version`, `git --version`, your OS, and the output of `ship trace`. ship removes paths, e-mail addresses and tokens from its logs, but read what you paste.
