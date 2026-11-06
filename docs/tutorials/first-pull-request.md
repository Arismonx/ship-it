# Your first pull request with ship

In this tutorial you make a small change to a project and send it for review as a pull request, and ship handles git for you. It takes about ten minutes.

You learn how to:

- check that your computer is ready;
- see what ship will do before it does anything;
- ship your work;
- know what to do next.

New words are explained in the [glossary](../glossary.md).

## Before you start

You need:

- **git** 2.43 or later. Check with `git --version`.
- **A GitHub account** and a repository you can push to, cloned on your computer. To practise, create an empty repository on GitHub (for example `ship-practice`), clone it, and follow along there.
- **A way for ship to reach GitHub.** If you already use the GitHub CLI (`gh auth login`) or push with HTTPS, you have nothing to do: ship finds the token on its own and never asks for it.
- **ship** itself: download it from the [latest release](https://github.com/GregoireF/ship-it/releases/latest), or build it with Go 1.27:

  ```sh
  go install github.com/GregoireF/ship-it/cmd/ship@latest
  ```

  Put the `ship` binary somewhere on your `PATH` (`go install` already does).

## 1. Check that you are ready

Open a terminal in your project and run:

```console
$ ship doctor
✓ ok     git 2.52.0
✓ ok     Identity: Ada Lovelace <ada@example.com>
✓ ok     Inside a git repository.
⚠ warn   No "origin" remote: ship-it cannot publish yet.
         → git remote add origin <url>

Ready to ship, with warnings.
```

<sub>Real output; the name and e-mail are replaced.</sub>

Fix any `✗` line before you go on; a `⚠` line only limits what ship can do. Every line that is not `✓ ok` comes with the exact command that fixes it (`→ …`). In this example the project has no GitHub remote yet: ship can commit, but not open a pull request. `ship fix` repairs most problems for you, and `ship undo` reverts what it did.

## 2. Make a change

Change any file of your project, or add one. For this tutorial, we added a small file `billing/vat.go`.

## 3. See the plan

Ask ship what it would do. This changes nothing:

```console
$ ship plan
✨ feat(billing): add vat
  branch    feat/add-vat
  files     1
  why       • Dominant category: source code (1 of 1 file).
            • 1 new source file.
```

ship has read your change and proposes:

- a title for the commit and the pull request: `✨ feat(billing): add vat`. It follows the [Conventional Commits](https://www.conventionalcommits.org/) format that tools and reviewers understand;
- a branch, `feat/add-vat`, because you are on `main` and work never goes straight into `main`;
- the files it will include, and why it chose this title.

## 4. Ship it

Run `ship`. A review screen opens in your terminal:

- **Enter** ships the plan as it is.
- **Tab** moves to the title, the branch, "Why?" and "Tested?": change anything you like. "Why?" becomes the description of your pull request, so one sentence about the reason for the change helps your reviewers a lot.
- **Space** removes a file from this delivery. ship includes new files too, so leave out anything you did not mean to share, such as a local note. **Esc** cancels: nothing changes.

When you press Enter, ship:

1. saves a checkpoint of your files, so that everything can be undone;
2. creates the branch and the commit;
3. runs your project's checks (lint, tests) if it has some;
4. pushes the branch and opens the pull request on GitHub.

If a check fails, nothing is pushed: your commit stays on your computer, you fix the problem, and you run `ship publish`.

> Want to see it without GitHub first? `ship --local` stops after the commit:
>
> ```console
> $ ship --yes --local
> ✓ Committed 30d4fff5843f on the new branch feat/add-vat.
>   Changed your mind? ship undo
>   Committed locally. Publish when ready: ship publish
> ```

## 5. Know what to do next

At any time, `ship status` tells you where you are and the one next step:

```console
$ ship status
feat/add-vat
  Changes       nothing to ship
  Upstream      not pushed yet
  ship undo     would undo the delivery ✨ feat(billing): add vat

→ Publish your commits: ship publish
```

Once your pull request is open, `ship status` also shows its checks and reviews, and tells you whether to fix the CI, answer a review, or merge.

## 6. After the merge

When your pull request is merged on GitHub, run `ship` again (or `ship tidy`): it brings you back to an up-to-date `main` and removes the merged branch.

## Contributing to someone else's project

To contribute to a project you cannot push to, fork it on GitHub, clone your fork and work there as above. ship sees that the repository is a fork: it pushes the branch to your fork and opens the pull request on the original project, against its default branch. Before publishing it brings your branch up to date with the original project, through an `upstream` remote if you added one (`git remote add upstream <url>`), else by fetching it directly. `ship undo` closes that pull request and deletes the branch on your fork.

## What you did

You delivered a change as a reviewed pull request: ship chose the title and the branch, checked your work and kept a way back at every step. Next:

- made a mistake? [Undo it](undo-a-mistake.md);
- curious about what happened under the hood? Read [the safety net](../explanation/safety-net.md).
