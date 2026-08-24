# repository-history: Added Requirements

These requirements extend the history-preservation capability to the results API
import. They are scoped to that import; the batch pipeline's equivalents are
declared by its own change.

## ADDED Requirements

### Requirement: Imported API history is preserved, not squashed

The results API SHALL be imported with its full commit history — approximately
150 commits dating to 2021-12-29 — preserving original authorship, author dates,
and commit messages.

#### Scenario: Commit count preserved

- **WHEN** the imported history is counted after extraction
- **THEN** it SHALL match the pre-extraction analysis of commits touching the
  moved paths

#### Scenario: Original authorship preserved

- **WHEN** an imported API file is annotated line by line
- **THEN** lines SHALL be attributed to their original authors and commits, not
  to the import or merge commit

### Requirement: Rename tracking survives the API import

Historical renames within the moved paths SHALL remain traceable through the
import, including the restructure that moved the handlers into their own package.

#### Scenario: Rename-tracked lineage resolves

- **WHEN** the history of an imported handler that was renamed before the
  migration is followed
- **THEN** it SHALL resolve through the merge into that file's pre-rename history

### Requirement: The API extraction is limited to the moved paths and relocates them

The extraction SHALL retain only the API's own paths, and SHALL relocate them
under a single directory in this repository so that the graft cannot collide with
existing files.

#### Scenario: No unrelated paths retained

- **WHEN** the extracted repository's tracked files are listed before the graft
- **THEN** every file SHALL belong to the API's path set

#### Scenario: No collision with existing files

- **WHEN** the extracted history is grafted
- **THEN** the merge SHALL be conflict-free, because no imported path exists in
  this repository beforehand

#### Scenario: Extraction runs against a disposable clone

- **WHEN** the extraction is performed
- **THEN** it SHALL run against a fresh, disposable clone and SHALL NOT be run
  against a working checkout

### Requirement: Source repository release tags are not imported

Tags from `ossf/scorecard-webapp` SHALL be removed before the graft. They both
collide with this repository's release namespace and carry deployment meaning
that does not apply here.

#### Scenario: No source tags present

- **WHEN** this repository's tags are listed after the import
- **THEN** no `ossf/scorecard-webapp` release tag SHALL be present

### Requirement: Historical pull-request references are disambiguated to their source

Pull-request references in imported commit subjects SHALL be rewritten during
extraction so they resolve to the repository the pull request was opened against.

#### Scenario: References resolve to the source repository

- **WHEN** an imported commit subject that carried a pull-request reference is
  read after the import
- **THEN** the reference SHALL identify `ossf/scorecard-webapp`

#### Scenario: No bare references remain

- **WHEN** imported commit subjects are inspected after extraction
- **THEN** none SHALL carry an unqualified pull-request reference

### Requirement: Path rewriting happens at the tip, not across history

Rewriting the imported code's module path and internal file references SHALL be a
single commit at the tip of the import. Historical commits SHALL retain the paths
they were authored with.

#### Scenario: Historical commits keep their original paths

- **WHEN** an imported historical commit is inspected
- **THEN** it SHALL contain the module path it was authored with

#### Scenario: The rewrite is reviewable as one diff

- **WHEN** the path rewrite is proposed
- **THEN** it SHALL be a single commit reviewable as a diff
