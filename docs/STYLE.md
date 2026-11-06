# Documentation style

How ship-it's documentation is organised and written. It follows the [Diátaxis](https://diataxis.fr/) framework and the [Google developer documentation style guide](https://developers.google.com/style/highlights).

## One page, one need

Every page answers one kind of need, and says which:

| Kind | The reader wants to… | Folder | Example |
|---|---|---|---|
| **Tutorial** | learn, by doing it once with guidance | `docs/tutorials/` | Your first pull request with ship |
| **How-to guide** | get a task done, already knowing the basics | `docs/how-to/` | Deliver only part of your work |
| **Reference** | look up a fact | `docs/reference/` | Every command and flag |
| **Explanation** | understand why it works this way | `docs/explanation/` | The safety net |

Design notes, decisions and research live in `docs/design/` and `docs/adr/`: they record how ship-it was thought out, and the other pages link to them instead of repeating them.

## Writing rules

- **Talk to the reader**: "you", not "we" or "the user". Be friendly, never cute.
- **Active voice, present tense**: "ship creates a branch", not "a branch will be created".
- **Short sentences, plain English**: many readers are not native speakers. One idea per sentence.
- **Sentence case** for titles and headings: "Undo a mistake", not "Undo A Mistake".
- **Numbered lists for steps**, bulleted lists for everything else.
- **Code font** for commands, flags, keys, file names and messages: `ship undo`, `--local`, `ship.pullRequest.maxLines`.
- **Real output only**: an example of ship's output is copied from a real run, never invented. When the output changes, the example changes.
- **Descriptive links**: "see [the settings reference](reference/settings.md)", never "click here".
- **Explain each new word** the first time, or link it to the [glossary](glossary.md).
- **Say what is not done yet**: a planned feature is marked *planned*, never described as if it existed.
- **Why before how**: the README and explanation pages tell the reason in prose first; a list is for facts a reader scans (steps, options, keys), never for the argument. Setting names and flags belong in the reference and how-to pages, not in the README.
- **No long dashes**: write a colon, a comma, parentheses or a full stop instead of "—".
- **No filler**: no "seamless", "robust", "powerful" or "comprehensive", no lists of three for rhythm, no "it's not X, it's Y". Say what ship does and stop.

## Generated pages

Some reference pages are generated from the code by `go run ./tools/gen` and must not be edited by hand (they start with a "Code generated" comment): the [settings reference](reference/settings.md) today. A test fails when they are out of date.
