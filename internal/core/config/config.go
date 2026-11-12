// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package config models the settings of ship-it and merges their layers
// (ADR-0009): built-in defaults < user file < repository .github/ship.json.
// Every key a file may hold is implemented: a key that is accepted but has no
// effect would mislead. Unknown keys are errors, with their path.
//
// Allowed imports: standard library without I/O, other core packages.
package config

import (
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/pullrequest"
	"github.com/GregoireF/ship-it/internal/core/risk"
)

// CurrentVersion is the version of the file format (decision N9).
const CurrentVersion = 1

// SchemaURL is the JSON Schema of .github/ship.json, for editor completion.
const SchemaURL = "https://raw.githubusercontent.com/GregoireF/ship-it/main/schemas/ship.schema.json"

// PublishMode says whether `ship` publishes after committing.
type PublishMode string

// Publish modes.
const (
	// PublishAuto pushes and opens the pull request when the remote is on
	// GitHub.
	PublishAuto PublishMode = "auto"
	// PublishNever stops after the local commit, as --local does.
	PublishNever PublishMode = "never"
)

// PublishSettings drive the publication that follows a commit.
type PublishSettings struct {
	Mode PublishMode `json:"mode" doc:"Whether ship pushes and opens the pull request after committing: \"auto\", or \"never\" (as --local; ship publish publishes later)." enum:"auto,never"`
	// Update is how a branch behind its base is brought up to date.
	Update string `json:"update" enum:"rebase,merge,none" doc:"How a branch behind its base is brought up to date before publishing: \"rebase\" replays it on the base (the push then uses a lease), \"merge\" merges the base into it (no rewrite, a normal push), \"none\" publishes it as it is. A checkpoint comes first; on a conflict ship cancels and publishes as it is."`
	// Overlaps turns on the warning about open pull requests that change the
	// same lines (#42).
	Overlaps bool `json:"overlaps" doc:"Before pushing, and in ship status, warn about the open pull requests into the same base that change the same or adjacent lines as the branch: a likely conflict for whoever merges second. It never stops a publication."`
}

// Values of ship.publish.update.
const (
	UpdateRebase = "rebase"
	UpdateMerge  = "merge"
	UpdateNone   = "none"
)

// GitSettings name what ship uses of git.
type GitSettings struct {
	Remote string `json:"remote" pattern:"^[A-Za-z0-9_][A-Za-z0-9._-]*$" patternHint:"a remote name such as origin (letters, digits, \".\", \"_\" and \"-\")" doc:"Remote ship fetches from and pushes to, and whose GitHub repository it works with. In a fork, pull requests still open on the parent repository."`
}

// DefaultRemote is the remote of a repository without ship.git.remote.
const DefaultRemote = "origin"

// TidySettings drive `ship tidy` and its run at the start of `ship`.
type TidySettings struct {
	Auto         bool `json:"auto" doc:"Run ship tidy at the start of ship: leave a branch whose pull request was merged for the updated base, with the work in progress. ship tidy itself always runs."`
	DeleteMerged bool `json:"deleteMerged" doc:"Delete the local branches whose pull request was merged and that hold nothing more (ship undo recreates them)."`
}

// LabelTaxonomy names the label set `ship sync` maintains.
type LabelTaxonomy string

// Label taxonomies.
const (
	// TaxonomyShipIt is the kind/ priority/ triage/ lifecycle/ size/ set of docs/design/git-github-standards.md.
	TaxonomyShipIt LabelTaxonomy = "ship-it"
	// TaxonomyNone leaves the repository labels alone.
	TaxonomyNone LabelTaxonomy = "none"
)

// Settings are the effective settings, every field resolved.
type Settings struct {
	Ship   ShipSettings   `json:"ship"`
	GitHub GitHubSettings `json:"github"`
	UI     UISettings     `json:"ui"`
}

// UISettings shape the terminal interface (docs/reference/cli.md). AI agents
// (MCP) see none of it.
type UISettings struct {
	Language     string `json:"language" doc:"Language of messages: \"auto\" reads SHIP_LANG, then the locale (LC_ALL, LC_MESSAGES, LANG). SHIP_LANG, when set, wins over this setting." enum:"auto,en,fr"`
	ReviewScreen bool   `json:"reviewScreen" doc:"Show the review screen of ship in a terminal; false asks one confirmation instead (the plan is printed first)."`
	Accessible   bool   `json:"accessible" doc:"Plain interface for screen readers: no full-screen review, one question per line, as --plain and SHIP_ACCESSIBLE=1."`
	Color        string `json:"color" doc:"Colours of the review screen: \"auto\" follows the terminal, \"always\" keeps them when the terminal is not detected as colour-capable, \"never\" removes them. NO_COLOR, when set, always wins." enum:"auto,always,never"`
}

// Values of ui.language and ui.color.
const (
	LanguageAuto = "auto"
	ColorAuto    = "auto"
	ColorAlways  = "always"
	ColorNever   = "never"
)

// ShipSettings drive the delivery.
type ShipSettings struct {
	Publish     PublishSettings     `json:"publish"`
	Git         GitSettings         `json:"git"`
	Commits     CommitsSettings     `json:"commits"`
	Branches    BranchesSettings    `json:"branches"`
	PullRequest PullRequestSettings `json:"pullRequest"`
	Issues      IssueSettings       `json:"issues"`
	Secrets     SecretsSettings     `json:"secrets"`
	Gates       GatesSettings       `json:"gates"`
	Tidy        TidySettings        `json:"tidy"`
	Logs        LogsSettings        `json:"logs"`
	Stack       StackSettings       `json:"stack"`
}

// StackSettings drive ship stack (ADR-0006, revision of 2026-11-02).
type StackSettings struct {
	Rebase    string `json:"rebase" doc:"Who rebases the layers above a merged one: \"github\" (GitHub's own rebase, which creates signed replacement commits; ship stack sync only follows it) or \"local\" (ship stack sync rebases on this machine with your signing key, then pushes with a lease). A classic chain of pull requests is always rebased locally." enum:"local,github"`
	MaxLayers int    `json:"maxLayers" minimum:"2" doc:"Most layers ship stack splits the work into; the smaller areas are grouped in the top layer beyond it."`
}

// Values of ship.stack.rebase.
const (
	StackRebaseLocal  = "local"
	StackRebaseGitHub = "github"
)

// LogsSettings bound the technical journal of each run
// (docs/reference/logs.md §2 and §5). Logs never leave the machine.
type LogsSettings struct {
	KeepRuns      int `json:"keepRuns" minimum:"1" doc:"Number of past runs whose journal is kept in each log directory (.git/ship/logs, or the user log directory)."`
	MaxBytes      int `json:"maxBytes" minimum:"1048576" doc:"Total size in bytes kept in each log directory; the oldest runs go first. At least 1 MiB."`
	BlackBoxBytes int `json:"blackBoxBytes" minimum:"65536" doc:"Size in bytes of the in-memory black box (DEBUG records of the run), written to disk only when a run fails. At least 64 KiB."`
}

// Default log limits.
const (
	DefaultKeepRuns      = 20
	DefaultMaxBytes      = 10 << 20
	DefaultBlackBoxBytes = 1 << 20
)

// CommitsSettings shape the commit messages ship-it writes and accepts. A
// project's commitlint still wins where its rules apply (rule zero,
// docs/reference/conventions.md).
type CommitsSettings struct {
	Gitmoji          string      `json:"gitmoji" doc:"Where the gitmoji goes: \"auto\" follows the repository's last 50 commits (ship-it's \"first\" when fewer than 5 are conventional), \"first\" (✨ feat: x), \"after-type\" (feat: ✨ x) or \"none\" (feat: x)." enum:"auto,first,after-type,none"`
	SubjectMaxLength int         `json:"subjectMaxLength" minimum:"20" doc:"Longest subject, in characters (after \"type(scope): \"; a tracker ticket at the end does not count)."`
	HeaderMaxLength  int         `json:"headerMaxLength" minimum:"30" doc:"Longest title, gitmoji included (✨ feat(scope): subject). The planner drops the scope of a longer one."`
	BodyWidth        int         `json:"bodyWidth" minimum:"40" doc:"Column at which commit bodies (the why) are wrapped."`
	Types            CommitTypes `json:"types" doc:"Changes to the built-in commit types, by type: {\"emoji\", \"branchPrefix\", \"label\", \"release\"} changes a type or adds one (a new type needs an emoji and a branch prefix), false removes a built-in type. label is the kind/ label of its pull requests (\"\": none); release its version increment (major, minor, patch or none; a breaking change is always major). ship types lists the result."`
	Scopes           []string    `json:"scopes" itemPattern:"^[a-z0-9][a-z0-9._/-]*$" doc:"Scopes a title may use; empty: any scope. The planner drops a scope that is not listed."`
	ScopeRequired    bool        `json:"scopeRequired" doc:"Refuse a title without a scope."`
	ScopeFrom        string      `json:"scopeFrom" enum:"folders,none" doc:"Where the planner takes the scope from: \"folders\", the deepest folder the changed files share, or \"none\" (you write it)."`
	Signoff          string      `json:"signoff" enum:"auto,always,never" doc:"Add a Signed-off-by trailer (DCO) with the committer's identity, as git commit -s does. \"auto\": when the repository asks for it, that is when most of its last 50 commits carry one, its CONTRIBUTING file mentions the DCO or Signed-off-by, or it configures the DCO app (.github/dco.yml)."`
	Sign             string      `json:"sign" enum:"auto,always,never" doc:"Sign commits: \"auto\" follows git's commit.gpgSign, \"always\" signs (git commit -S) and refuses to commit without a signing key (user.signingKey), \"never\" does not sign (--no-gpg-sign). Rebases and merges made before a publication follow it too."`
}

// Values of ship.commits.scopeFrom.
const (
	ScopeFromFolders = "folders"
	ScopeFromNone    = "none"
)

// Values of ship.commits.signoff and ship.commits.sign.
const (
	ModeAuto   = "auto"
	ModeAlways = "always"
	ModeNever  = "never"
)

// BranchesSettings shape the branch names ship-it proposes. Their prefix
// comes from ship.commits.types.<type>.branchPrefix.
type BranchesSettings struct {
	MaxWords  int    `json:"maxWords" minimum:"1" doc:"Words kept from the subject in a branch name (stop words such as \"the\" are dropped first)."`
	MaxLength int    `json:"maxLength" minimum:"8" doc:"Longest description part of a branch name, after the prefix and the issue number."`
	Issue     string `json:"issue" enum:"first,none" doc:"\"first\" starts the branch description with the issue number or tracker ticket (feat/42-add-login), \"none\" leaves it out."`
}

// Values of ship.branches.issue.
const (
	BranchIssueFirst = "first"
	BranchIssueNone  = "none"
)

// GatesSettings drive the project checks run before a push.
type GatesSettings struct {
	Enabled bool `json:"enabled" doc:"Run the project's own checks for each ecosystem it uses (the package.json scripts lint, typecheck and test; gofmt, go vet and go test; cargo fmt, clippy and test; and so on for Python, PHP, Ruby, JVM and .NET) before every push. ship check gates runs them on demand, and beforeCommit before the commit too. false: no check runs."`
	// AllowSkip lets --skip-gates through; false makes the checks mandatory.
	AllowSkip bool `json:"allowSkip" doc:"Accept --skip-gates (troubleshooting). false: the checks always run before a push."`
	// BeforeCommit runs the gates before the commit too (ship, ship_deliver).
	BeforeCommit bool `json:"beforeCommit" doc:"Also run the checks before the commit of ship and ship_deliver: a red check refuses the commit. Off by default, so that a commit stays instant and the checks run once, before the push."`
}

// SecretsSettings tune the secret scan that runs before every commit. For an
// AI agent (MCP), ignorePaths, disabledRules and allow count only as
// .github/ship.json holds them in HEAD: it cannot weaken its own scan.
type SecretsSettings struct {
	IgnorePaths []string `json:"ignorePaths" doc:"Glob patterns of files never scanned for secrets (false positives); dir/** for a whole directory."`
	// Rules are tried after the built-in ones.
	Rules []SecretRule `json:"rules" uniqueBy:"id" doc:"Extra rules: an id, a Go regular expression (RE2) and the literal keywords a line must contain for the pattern to run (compared without case when the pattern has (?i)). The value reported is the first capture group, else the whole match."`
	// DisabledRules: the ids are checked against secrets.BuiltinRules (config_test.go).
	DisabledRules []string `json:"disabledRules" itemEnum:"private_key,github_token,aws_key,anthropic_key,openai_key,groq_key,google_key,stripe_key,slack_token,npm_token,url_with_password,secret_assignment,sensitive_file" doc:"Built-in rules turned off, by id."`
	Allow         []string `json:"allow" itemPattern:"^[0-9a-f]{64}$" patternHint:"fingerprints as ship prints them (64 lower-case hexadecimal characters)" doc:"Findings accepted, by the fingerprint ship prints with each one (PBKDF2-HMAC-SHA256 of the value, salted with the rule id), never by value: a value in a file would leak it."`
}

// SecretRule is an extra rule of the secret scan (ship.secrets.rules).
type SecretRule struct {
	ID       string   `json:"id" required:"true" pattern:"^[a-z][a-z0-9_]*$" patternHint:"lower-case letters, digits and _ (such as acme_token), not the id of a built-in rule" doc:"Name shown with each finding; not the id of a built-in rule."`
	Pattern  string   `json:"pattern" required:"true" doc:"Go regular expression (RE2) matched against each added line; (?i) ignores case. It must not match an empty text."`
	Keywords []string `json:"keywords" required:"true" doc:"Literals a line must contain, one at least, for the pattern to run: they keep the scan fast."`
}

// check validates a rule as the scan will compile it.
func (r SecretRule) check() (field, value, allowed string, ok bool) {
	if slices.Contains(strings.Split(builtinSecretRules, ","), r.ID) {
		return "id", r.ID, "an id other than the built-in rules (" + strings.ReplaceAll(builtinSecretRules, ",", ", ") + ")", false
	}
	if r.ID == "" {
		return "id", r.ID, "a name such as acme_token", false
	}
	if len(r.Keywords) == 0 || slices.Contains(r.Keywords, "") {
		return "keywords", stringOf(r.Keywords), "one non-empty keyword at least, which every line the pattern matches contains", false
	}
	re, err := regexp.Compile(r.Pattern)
	if err != nil {
		return "pattern", r.Pattern, "a Go regular expression (RE2): " + err.Error(), false
	}
	if re.MatchString("") {
		return "pattern", r.Pattern, "a pattern that does not match an empty text", false
	}
	return "", "", "", true
}

// builtinSecretRules is the itemEnum of ship.secrets.disabledRules.
var builtinSecretRules = func() string {
	f, _ := reflect.TypeOf(SecretsSettings{}).FieldByName("DisabledRules")
	return f.Tag.Get("itemEnum")
}()

// PullRequestSettings shape the pull requests ship-it opens.
type PullRequestSettings struct {
	Draft         string        `json:"draft" enum:"unattended,always,never" doc:"When a new pull request is a draft: \"unattended\" when nobody reviewed the plan in a terminal (--yes), \"always\", or \"never\". Work in progress commits, ship publish --draft and AI agents always open a draft."`
	Base          string        `json:"base" pattern:"^([A-Za-z0-9_][A-Za-z0-9._/-]*)?$" patternHint:"a branch name such as develop, or \"\" for the default branch" doc:"Base branch of new pull requests, and the branch ship brings a branch up to date with; empty: the default branch of the repository on GitHub (of the parent repository, in a fork). Closing keywords only work on the default branch: on another base, the issue is linked with Refs."`
	Labels        bool          `json:"labels" doc:"Add the kind and size labels the repository already has."`
	LabelPrefixes LabelPrefixes `json:"labelPrefixes"`
	Sizes         Sizes         `json:"sizes" doc:"Size labels, by name (after the size prefix), and the number of changed lines, generated files excluded, under which each applies; 0 marks the size of everything larger. The default is Kubernetes' scale."`
	MaxLines      int           `json:"maxLines" minimum:"1" doc:"Changed lines (generated files excluded) above which the plan warns and suggests a split. Reviews lose effectiveness beyond about 400."`
	// Template is "auto", "ship" or a path in the repository.
	Template       string   `json:"template" doc:"Pull request body: \"auto\" fills the repository's pull request template when it has one (as GitHub finds it on the default branch), \"ship\" always uses ship-it's format, or the path of a template in the repository (for a .github/PULL_REQUEST_TEMPLATE/ directory)."`
	SensitivePaths []string `json:"sensitivePaths" doc:"Paths whose changes ship preview counts as sensitive in its risk score, as glob patterns, on top of continuous integration and dependencies."`
	Sections       Sections `json:"sections"`
	Reviewers      []string `json:"reviewers" itemPattern:"^[A-Za-z0-9](-?[A-Za-z0-9])*$" patternHint:"GitHub logins (letters, digits and single hyphens)" doc:"Logins whose review is requested on a new pull request (never its author). A request GitHub refuses is a warning."`
	TeamReviewers  []string `json:"teamReviewers" itemPattern:"^[A-Za-z0-9][A-Za-z0-9._-]*$" patternHint:"team slugs of the repository's organisation, such as backend" doc:"Teams of the repository's organisation, by slug, whose review is requested on a new pull request. A request GitHub refuses is a warning."`
	AssignSelf     bool     `json:"assignSelf" doc:"Assign a new pull request to its author."`
	Milestone      string   `json:"milestone" enum:"issue,none" doc:"\"issue\" puts a new pull request in the milestone of the issue it closes, \"none\" leaves milestones alone."`
}

// Values of ship.pullRequest.draft.
const (
	DraftUnattended = "unattended"
	DraftAlways     = "always"
	DraftNever      = "never"
)

// Values of ship.pullRequest.milestone.
const (
	MilestoneIssue = "issue"
	MilestoneNone  = "none"
)

// LabelPrefixes are what the kind and size labels start with.
type LabelPrefixes struct {
	Kind string `json:"kind" doc:"Prefix of the kind labels: kind/feature, kind/bug… The commit types name the kinds (ship.commits.types.<type>.label)."`
	Size string `json:"size" doc:"Prefix of the size labels: size/XS… (ship.pullRequest.sizes)."`
}

// Sections are the parts of ship-it's own pull request body. A repository
// template, when one applies, places the parts itself.
type Sections struct {
	Why     bool `json:"why" doc:"Write the why of the commits (their bodies) in ship's own pull request body."`
	Testing bool `json:"testing" doc:"Write the Testing section (the checks that passed locally, the Tested: notes) in ship's own pull request body."`
	Issue   bool `json:"issue" doc:"Link the issue (Closes #12) in ship's own pull request body."`
	Changes bool `json:"changes" doc:"Sum up the diff in the pull request body: the files added, changed, renamed and removed, and the declarations added and removed."`
}

// Values of ship.pullRequest.template other than a path.
const (
	TemplateAuto = "auto"
	TemplateShip = "ship"
)

// IssueSettings link pull requests to issues.
type IssueSettings struct {
	Keyword string `json:"keyword" doc:"Keyword linking the issue in the pull request body." enum:"Closes,Fixes,Resolves,Refs"`
	// TrackerKeys are the project keys of an external tracker (Jira, Linear).
	TrackerKeys []string `json:"trackerKeys" itemPattern:"^[A-Z][A-Z0-9_]+$" doc:"Project keys of an external tracker (Jira, Linear), such as ACME: a ticket ACME-123 in the branch name ends the commit and pull request titles, \"(ACME-123)\", and the branch keeps it in upper case."`
}

// GitHubSettings are the repository settings `ship sync` maintains (ADR-0014).
type GitHubSettings struct {
	ManagedBy string          `json:"managedBy" doc:"Tool already managing this repository (opentofu, terraform…): ship sync then only reports differences."`
	Labels    LabelsSettings  `json:"labels"`
	Project   ProjectSettings `json:"project"`
}

// ProjectSettings name the GitHub Project (v2) that ship roadmap sync and
// ship publish keep up to date, and its fields. Field and option names are
// compared without regard to case; one that the project lacks is reported,
// never created.
type ProjectSettings struct {
	// URL is checked against forge.ProjectPattern (config_test.go).
	URL         string `json:"url" pattern:"^(https://[A-Za-z0-9.-]+/(orgs|users)/[A-Za-z0-9-]+/projects/[1-9][0-9]*(/views/[0-9]+)?/?|[A-Za-z0-9-]+/[1-9][0-9]*)?$" patternHint:"a project URL (https://github.com/orgs/acme/projects/7) or owner/number (acme/7)" doc:"GitHub Project (v2) to keep up to date: its URL (https://github.com/orgs/acme/projects/7, https://github.com/users/ada/projects/3) or owner/number (acme/7). Empty: no project. The token needs the project scope (gh auth refresh -s project)."`
	StatusField string `json:"statusField" doc:"Single-select field holding the status. Empty: the status is left alone."`
	Todo        string `json:"todo" doc:"Status of an open roadmap issue that has none yet (a status set by hand is kept)."`
	InProgress  string `json:"inProgress" doc:"Status ship publish sets when it opens a pull request that closes an issue of the project."`
	Done        string `json:"done" doc:"Status of a closed roadmap issue."`
	DateField   string `json:"dateField" doc:"Date field that receives the milestone's due date, such as \"Target date\" (for the Roadmap view). Empty: off."`
	PhaseField  string `json:"phaseField" doc:"Single-select field that receives the milestone's title, such as \"Phase\"; its options must exist (the REST API cannot add them). Empty: off."`
}

// Default names of a project's status field and options. GitHub's default
// options are "Todo", "In Progress" and "Done"; names match without case.
const (
	DefaultStatusField = "Status"
	DefaultTodo        = "Todo"
	DefaultInProgress  = "In progress"
	DefaultDone        = "Done"
)

// LabelsSettings describe the wanted labels.
type LabelsSettings struct {
	Taxonomy LabelTaxonomy `json:"taxonomy" doc:"Label set to maintain." enum:"ship-it,none"`
	// Areas become area/<name> labels.
	Areas []string `json:"areas" doc:"Names of area/ labels, usually the commit scopes."`
}

// Defaults returns the built-in settings: a repository without any file
// works with them (progressive configuration, ADR-0009).
func Defaults() Settings {
	return Settings{
		Ship: ShipSettings{
			Publish: PublishSettings{Mode: PublishAuto, Update: UpdateRebase, Overlaps: true},
			Git:     GitSettings{Remote: DefaultRemote},
			Commits: CommitsSettings{Gitmoji: "auto", SubjectMaxLength: convention.DefaultSubjectMaxLength,
				HeaderMaxLength: convention.DefaultHeaderMaxLength, BodyWidth: convention.DefaultBodyWidth,
				Types: CommitTypes{}, Scopes: []string{}, ScopeFrom: ScopeFromFolders, Signoff: ModeAuto, Sign: ModeAuto},
			Branches: BranchesSettings{MaxWords: convention.DefaultBranchMaxWords,
				MaxLength: convention.DefaultBranchMaxLength, Issue: BranchIssueFirst},
			PullRequest: PullRequestSettings{Draft: DraftUnattended, Labels: true,
				LabelPrefixes: LabelPrefixes{Kind: pullrequest.DefaultKindPrefix, Size: pullrequest.DefaultSizePrefix},
				Sizes:         DefaultSizes(), MaxLines: 400, Template: TemplateAuto,
				Sections:  Sections{Why: true, Testing: true, Issue: true, Changes: true},
				Reviewers: []string{}, TeamReviewers: []string{}, Milestone: MilestoneIssue,
				SensitivePaths: slices.Clone(risk.DefaultSensitivePaths)},
			Issues:  IssueSettings{Keyword: "Closes", TrackerKeys: []string{}},
			Secrets: SecretsSettings{IgnorePaths: []string{}, Rules: []SecretRule{}, DisabledRules: []string{}, Allow: []string{}},
			Gates:   GatesSettings{Enabled: true, AllowSkip: true},
			Tidy:    TidySettings{Auto: true, DeleteMerged: true},
			Logs:    LogsSettings{KeepRuns: DefaultKeepRuns, MaxBytes: DefaultMaxBytes, BlackBoxBytes: DefaultBlackBoxBytes},
			Stack:   StackSettings{Rebase: StackRebaseGitHub, MaxLayers: 5},
		},
		GitHub: GitHubSettings{
			Labels: LabelsSettings{Taxonomy: TaxonomyShipIt, Areas: []string{}},
			Project: ProjectSettings{StatusField: DefaultStatusField, Todo: DefaultTodo, InProgress: DefaultInProgress,
				Done: DefaultDone},
		},
		UI: UISettings{Language: LanguageAuto, ReviewScreen: true, Color: ColorAuto},
	}
}

// ReadOnlySync reports whether `ship sync` must only report differences.
func (s Settings) ReadOnlySync() bool { return strings.TrimSpace(s.GitHub.ManagedBy) != "" }

// Layer is one configuration file.
type Layer struct {
	// Name is shown as the source of the settings it sets: "user",
	// "repository", "ship-it:recommended", "SHIP_PULLREQUEST_MAXLINES"…
	Name string
	// Kind decides the keys the layer may hold (layers.go); empty means it
	// is recognised by Name ("user", "policy", else a repository file).
	Kind LayerKind
	// Path is shown in messages.
	Path string
	Data []byte
}

// Effective is the merged result and, for every key set by a file, the name
// of the layer that set it.
type Effective struct {
	Settings Settings          `json:"settings"`
	Sources  map[string]string `json:"sources"`
	// Policy is set when an organisation policy applies.
	Policy *Policy `json:"policy,omitempty"`
}

// Source returns the layer that set key ("default" when none did).
func (e Effective) Source(key string) string {
	if s, ok := e.Sources[key]; ok {
		return s
	}
	return "default"
}

// Keys returns the keys set by files, sorted.
func (e Effective) Keys() []string {
	keys := make([]string, 0, len(e.Sources))
	for k := range e.Sources {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
