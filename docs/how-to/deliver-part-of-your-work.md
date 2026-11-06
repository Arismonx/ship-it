# Deliver only part of your work

Use this when your working tree mixes several topics, or when ship warns that a change is too large to review well.

## Deliver a folder or some files

```sh
ship --only src/api/            # everything under src/api/
ship --only README.md --only docs/guide.md
```

`--only` can be repeated. The rest stays uncommitted and untouched. `ship plan --only src/api/` shows the plan for that part first.

## Follow the split ship suggests

Above `ship.pullRequest.maxLines` changed lines (400 by default, generated files and lockfiles not counted), the plan lists one `ship --only <folder>/` command per area, largest first. Reviews lose effectiveness on large pull requests, so delivering area by area gives reviewers smaller, focused changes.

Change the threshold for a repository in `.github/ship.json` (`ship.pullRequest.maxLines`), or for one run: `ship --set ship.pullRequest.maxLines=800`.

## Pick files on the review screen

On the review screen, the **Files** list works the same way: **Space** removes a file from this delivery, and the plan is recomputed for the rest.

## Undo a partial delivery

`ship undo` reverts the commit and leaves alone the files you did not deliver.
