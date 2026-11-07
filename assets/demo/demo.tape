# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Grégoire Favreau
#
# The demo GIF of the README, recorded with vhs (https://github.com/charmbracelet/vhs)
# from a released ship on Linux, in a throwaway repository: plan, review
# screen, commit, undo. Nothing is pushed. To record it again, with Docker,
# from the repository's root, in an image based on ghcr.io/charmbracelet/vhs
# that adds git and the ship binary of the release:
#
#   docker run --rm -v "$PWD:/vhs" <that image> assets/demo/demo.tape

Output assets/demo/demo.gif

Set Shell bash
Set FontSize 20
Set Width 1100
Set Height 620
Set Padding 24
Set TypingSpeed 60ms
Set Theme "GitHub Dark"

# A small Go project with one change in progress.
Hide
Type "export PS1='$ ' GIT_PAGER=cat && cd $(mktemp -d) && git init -q -b main && git config user.name Ada && git config user.email ada@example.com"
Enter
Type "git remote add origin https://github.com/acme/shop.git && printf 'module example.com/shop\n\ngo 1.22\n' > go.mod && git add -A && git commit -qm init"
Enter
Type "printf 'package shop\n\n// VAT is the French VAT rate, in percent.\nconst VAT = 20\n' > vat.go && clear"
Enter
Show

Type "git status --short"
Sleep 300ms
Enter
Sleep 1.5s

Type "ship plan"
Sleep 300ms
Enter
Sleep 3s

Type "ship --local"
Sleep 300ms
Enter
Sleep 4s
Enter
Sleep 4s

Type "git log --oneline"
Sleep 300ms
Enter
Sleep 2.5s

Type "ship undo --yes"
Sleep 300ms
Enter
Sleep 4s

Type "git status --short"
Sleep 300ms
Enter
Sleep 3s
