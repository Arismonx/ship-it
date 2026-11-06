# Ship your work as a stack of pull requests

Use this when one piece of work is too large to review as one pull request, and its parts depend on each other: the API first, then the screen that uses it, then the docs. Each part becomes a layer: a pull request based on the one below it, reviewed on its own. On github.com the layers form a [native stack](https://docs.github.com/en/pull-requests/get-started/about-stacked-prs); GitHub merges them from the bottom up.

## ship stack or gh stack?

GitHub has its own command-line tool for stacks, the official [`gh stack`](https://github.com/github/gh-stack) extension. It manages a stack you build branch by branch: `gh stack add` commits on a new branch on top, `gh stack modify` reorders, folds or drops branches, and `gh stack sync` rebases and pushes. If you already cut your work into branches as you go, or you reshape a long-lived stack often, use `gh stack`.

`ship stack` starts earlier, from work you have not committed yet. It splits that work into layers for you and gives every layer what `ship` gives a single pull request: your conventions, the project's checks, the secret scan and the repository's rulesets. One `ship undo` removes the whole stack. An AI agent that stacks through ship gets draft pull requests, as it does for one pull request. Both tools make the same native stack on GitHub, so a reviewer sees no difference.

## Ship the stack

From the default branch, with the work uncommitted:

```sh
ship stack
```

ship splits the work into layers by area, the way the size warning does (the largest area at the bottom). Layers by folder are only a starting point: the review screen lets you reorder them and move files from one to another. ship shows them on a review screen like the one of `ship`, the bottom layer first:

```text
⛵ ship stack · 3 layers on main, bottom first

› Layer 1 · 2 files, 310 lines
    Title   ✨ feat(api): add the cart
            ✓ valid · 25/72
    Branch  feat/add-the-cart
            ✓ valid
    Files   api/cart.go
            api/cart_test.go

  Layer 2 · 1 file, 120 lines
    Title   ✨ feat(web): add the cart page
            ✓ valid · 30/72
    Branch  feat/add-the-cart-page
            ✓ valid
    Files   web/cart.ts
```

On this screen you can:

- move between lines with `↑` and `↓`, and between layers with `Tab`;
- edit a title or a branch: both are checked as you type, and a branch you did not touch follows its title;
- reorder the layers: on a layer, `Shift+↑` or `Shift+↓` (`K` or `J`) moves it up or down the list;
- move a file: on a file, `Shift+↑` or `Shift+↓` moves it to the layer above or below, and `1` to `9` to that layer. The titles you did not edit follow the new files;
- press `Enter` to ship, `Ctrl+L` to commit only, `Esc` to cancel with nothing changed, `?` for the keys.

Two layers cannot share a branch, and a layer left without files cannot ship. With `--plain`, `SHIP_ACCESSIBLE=1` or `ui.accessible`, ship asks for each title and branch on its own line instead; choose the files with `--layer`. With `ui.reviewScreen` set to `false`, or when stderr is not a terminal, it lists the layers and asks one question:

```text
Stack of 3 layers on main, bottom first:
  1. ✨ feat(api): add cart
     feat/add-cart · 2 files, 50 lines · api/
  2. ✨ feat(web): add cart
     feat/add-cart-2 · 1 file, 12 lines · web/
  3. 📝 docs: add cart docs
     chore/add-cart-docs · 1 file, 3 lines · docs/
Ship these 3 layers? [Y/n]
```

Once you accept, ship takes a checkpoint, then for each layer creates a branch based on the previous one and commits its files. It checks every layer branch and its commits against the repository's GitHub rulesets: if one layer would be refused, nothing is pushed. As for a single pull request, a branch ship created and never pushed is renamed to the name a ruleset requires, and `ship undo` gives the old name back. ship then runs the project's checks once, pushes every branch, opens one pull request per layer (each targets the branch below it) and groups them in a stack. With `--yes` or from an agent, ship asks nothing and the pull requests are drafts, as for a single pull request.

Each pull request gets what a single one gets: the `kind/` and `size/` labels the repository has, and the linked issue (`--issue`, or a number in a layer's branch name). Only the bottom layer closes the issue (`Closes #12`): it is the only one based on the default branch, the only place where GitHub reads a closing keyword. The layers above say `Refs #12`.

At most `ship.stack.maxLayers` layers are made (5 by default); beyond that, the smaller areas share the top layer. Work that touches a single area cannot be stacked: ship says so.

## Choose the layers yourself

Give each layer, bottom first, as files or folders separated by commas:

```sh
ship stack --layer api/ --layer web/,docs/cart.md
```

Files that no layer names stay uncommitted. `ship stack --dry-run` shows every step without changing anything; `ship stack --local` commits the layers without publishing them (publish later with `ship stack publish` from the top layer).

## Add a layer on top

On the top layer of a stack, run `ship` as usual: the new work becomes a new branch based on that layer, its pull request targets it, and it joins the stack.

## See where the stack stands

```sh
ship stack status
```

It lists the layers from the top down, with their pull requests and checks; `▸` marks the branch you are on. `--offline` skips GitHub.

## After a layer is merged

Once GitHub has merged the bottom layer:

```sh
ship stack sync
```

ship fetches, deletes the merged layers (here and on GitHub), and rebases the others on the layer below them that stays. What does the rebase depends on `ship.stack.rebase`:

- `github` (the default): ship leaves the rebase to GitHub and moves your local branches to GitHub's result. GitHub [creates signed replacement commits](https://github.blog/changelog/2026-10-06-stacked-pull-requests-generally-available) that keep their author, and keeps the approvals. A layer with commits you never pushed is left alone, with a warning.
- `local`: ship rebases on your machine and pushes with a lease, so your commits stay signed with your own key and nobody's work is overwritten.

A sync needs a clean working tree. A checkpoint comes first, and `ship undo` puts the local branches back. Because the sync changed GitHub too, the undo asks your consent (or `ship undo --yes`), then puts each remote branch back where the sync found it, under a lease: only where nobody pushed since. Where someone did, their commits stay and ship warns (`SHIP-W1015`). In a classic chain, the pull requests the sync retargeted point at their old base again, and their "Part n of N" lines come back.

## Merge the stack

```sh
ship stack merge
```

ship shows which pull requests the merge takes, asks for confirmation, then asks GitHub to merge them through the asynchronous merge API and waits for the answer. Merging the top layer merges every layer below it, as one. GitHub's rules always apply: ship never asks to bypass them, and a merge queue stays in charge when the repository has one. Without a terminal, `--yes` is needed. AI agents cannot merge: the MCP server has no merge tool.

## On GitHub Enterprise Server

Where the server has no stacks API, the same commands make a classic chain: each pull request targets the branch of the one below, and its description ends with a line such as "Part 2 of 3, after #41." When a layer is added on top or merged away, ship rewrites that line in every open pull request of the chain. `ship stack merge` then merges the bottom layer only; run `ship stack sync` after each merge, which also points the next pull request at the default branch. A chain is always rebased locally.

## Undo a stack

`ship undo` right after `ship stack` asks your consent, then closes every pull request of the stack that is not merged (with a comment), dissolves the stack and deletes the remote branches. A second `ship undo` puts the work back uncommitted on the default branch and removes the layer branches.
