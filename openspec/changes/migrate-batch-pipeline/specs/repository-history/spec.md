# repository-history: Added Requirements

## ADDED Requirements

### Requirement: Imported commit history is preserved, not squashed

The pipeline SHALL be imported with its full commit history — approximately 455
commits dating to 2020-11-10 — preserving original authorship, author dates, and
commit messages.

#### Scenario: Commit count preserved

- **WHEN** the imported history is counted after extraction
- **THEN** it SHALL match the pre-extraction analysis of commits touching the
  moved paths

#### Scenario: Original authorship preserved

- **WHEN** an imported pipeline file is annotated line by line
- **THEN** lines SHALL be attributed to their original authors and commits, not to
  the import or merge commit

### Requirement: Rename tracking survives the import

Historical renames within the moved paths SHALL remain traceable through the
import, so a file's lineage resolves into its pre-rename history.

#### Scenario: Rename-tracked lineage resolves

- **WHEN** the history of a file that was renamed before the migration is followed
- **THEN** it SHALL resolve through the merge into that file's pre-rename history

### Requirement: Extraction is limited to the moved paths

The extraction SHALL retain only the pipeline tree and the relocated token-pool
server, and SHALL relocate the token-pool server to its destination path as part
of the extraction.

#### Scenario: No unrelated paths retained

- **WHEN** the extracted repository's tracked files are listed before the graft
- **THEN** every file SHALL be within the pipeline tree

#### Scenario: Extraction runs against a disposable clone

- **WHEN** the extraction is performed
- **THEN** it SHALL run against a fresh, disposable clone and SHALL NOT be run
  against a working checkout

### Requirement: Upstream release tags are not imported

Tags from `ossf/scorecard` SHALL be removed before the graft so that this
repository's tag and release namespace is unaffected by the import.

#### Scenario: No upstream tags present

- **WHEN** this repository's tags are listed after the import
- **THEN** no `ossf/scorecard` release tag SHALL be present

### Requirement: Historical pull-request references are disambiguated

Pull-request references in imported commit messages SHALL be rewritten during
extraction to name their originating repository, so they do not auto-link to
unrelated items in this repository.

#### Scenario: Reference resolves to the originating repository

- **WHEN** an imported commit message containing a pull-request reference is
  rendered
- **THEN** the reference SHALL resolve to the corresponding `ossf/scorecard` item

#### Scenario: Rewrite happens before the graft

- **WHEN** the references are rewritten
- **THEN** the rewrite SHALL occur during extraction, before any history is pushed
  or merged into this repository

### Requirement: Import paths are rewritten at the tip only

The module path rewrite from the upstream module to this module SHALL be applied
as a single commit at the tip of the imported history, leaving historical commits
as originally authored.

#### Scenario: Historical commits unmodified

- **WHEN** an imported historical commit is inspected
- **THEN** it SHALL contain the module paths as originally authored

#### Scenario: Tip builds against this module

- **WHEN** the repository is built after the rewrite commit
- **THEN** all imported pipeline packages SHALL resolve against this repository's
  module path

### Requirement: History verified before and after the graft

The extraction SHALL be verifiable in isolation before anything is pushed to this
repository, and the preservation guarantees SHALL be re-verified after the graft.

#### Scenario: Pre-graft review

- **WHEN** extraction completes
- **THEN** the filtered result SHALL be reviewable independently, and nothing SHALL
  be pushed to this repository until its verification gates pass

#### Scenario: Post-graft verification

- **WHEN** the graft completes
- **THEN** rename-tracked lineage and original-author attribution SHALL be
  re-verified against the grafted history
