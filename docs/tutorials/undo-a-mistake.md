# Undo a mistake

In this tutorial you ship something, change your mind, and take it back. You learn that with ship, a mistake costs one command.

## 1. Ship a change locally

Make any change in a project, then commit it without publishing:

```console
$ ship --yes --local
✨ feat(billing): add vat
  branch    feat/add-vat
  files     1
  why       • Dominant category: source code (1 of 1 file).
            • 1 new source file.
✓ Committed 30d4fff5843f on the new branch feat/add-vat.
  Changed your mind? ship undo
  Committed locally. Publish when ready: ship publish
```

ship created a branch and a commit. Notice the line `Changed your mind? ship undo`.

## 2. Undo it

```console
$ ship undo
↶ Undone: ✨ feat(billing): add vat. Your changes are uncommitted again on main.
  Branch feat/add-vat removed.
  Saved before undoing: ship checkpoint restore 14d6b6519dbe
```

You are back on `main`, with your change still in your files: only the commit and the branch are gone. ship even saved a checkpoint before undoing, in case you change your mind again.

```console
$ ship status
main
  Changes       1 file not shipped
  Upstream      not pushed yet

→ Ship your 1 changed file: ship
```

## 3. What else can be undone

`ship log` lists what ship did, and marks with `↶` what `ship undo` reverts next. You can undo:

- a commit (as above);
- a **publication**: ship closes the pull request with a comment and deletes the branch on GitHub. Because this changes GitHub, ship asks you to confirm first (in a script: `ship undo --yes`);
- `ship tidy`, `ship fix`, `ship config set` and `ship roadmap sync`.

A pull request that is already **merged** cannot be undone this way: the work is in `main` now, and a revert is a new change that teammates review.

## What you learned

Every ship operation is recorded, almost all of them can be reverted, and ship never loses your files. Read [the safety net](../explanation/safety-net.md) to see how it works.
