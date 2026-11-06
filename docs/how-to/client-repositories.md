# Work on client repositories

For freelancers and agencies: give each client's repositories their own settings, without committing anything to them.

## 1. One context per client, in your user file

Your user file (`ship config set … --user` writes it; on Windows `%APPDATA%\ship-it\config.json`, on Linux `~/.config/ship-it/config.json`):

```jsonc
{
  "contexts": [
    {
      "match": { "owner": "acme-corp" },
      "extends": [],
      "ship": { "issues": { "keyword": "Refs", "trackerKeys": ["ACME"] }, "publish": { "mode": "never" } }
    },
    { "match": { "path": "~/clients/bank/**" }, "ship": { "pullRequest": { "maxLines": 200 } } }
  ]
}
```

A context applies when every criterion of `match` holds: the GitHub `owner` or `host` of the repository's remote (`origin`, or `ship.git.remote`), or the folder it is in (`path`).

- `"keyword": "Refs"` links issues without closing them: the client closes their own issues.
- `"trackerKeys": ["ACME"]` carries Jira or Linear tickets from the branch name (`feat/ACME-123-login`) into titles: `✨ feat(auth): add login (ACME-123)`.
- `"publish": { "mode": "never" }`: ship commits locally, you publish after your own review with `ship publish`.

## 2. Or the agency preset, in the repository

When the client agrees to a file in their repository, `"extends": ["ship-it:agency"]` in `.github/ship.json` gives the same safe defaults to everyone working on it.

## 3. Check what applies

```sh
ship config explain ship.issues.keyword
```

It shows the value of every layer (default, your user file, the context that matched, the repository) and which one wins.
