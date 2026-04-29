# Product Gaps For Next Plan

**Status:** Active product research, non-release-facing, and not part of the v0 public contract.

This file records product gaps noticed while improving documentation. These
items are not current commitments. They are prompts for the next planning cycle
and should stay broad enough to improve the product model rather than one
single tutorial or demo.

## Unified Reviewed Actions

Problem: JVS has several reviewed actions, including restore, workspace
removal, and cleanup. Each is safe, but the language for preview, run, impact,
and next command is still learned command by command.

Why it affects user mind: users need to build one mental model for "JVS shows
me what will change before it changes files." When each command teaches that
model differently, people may over-trust one command or under-trust another.

Possible improvement direction: define a shared reviewed-action vocabulary and
presentation pattern for commands that preview before changing files.

GA blocker: No.

## Adoption Confidence Before First Save

Problem: `jvs init <folder>` adopts an existing folder without moving user
files, but the product has limited room to explain what the first save will
capture before the user creates it.

Why it affects user mind: first-time users are most anxious before the first
save. They need confidence that their folder remains ordinary and that JVS is
clear about what it will manage.

Possible improvement direction: add richer first-run status or a focused
adoption summary that explains managed files, excluded control data, and the
next safe command without adding a new mandatory workflow.

GA blocker: No.

## Copyable History IDs

Problem: human `jvs history` output may show shortened save point IDs. A short
prefix can look like the right value to copy, but later commands can reject it
as ambiguous if more than one save point shares that prefix.

Why it affects user mind: choosing the right save point is already a careful
moment. If a user copies the visible value from history and then hits an
ambiguous-ID error, the product feels like it showed them an answer that was
not actually usable.

Possible improvement direction: explore making human history output show a
copyable unique ID by default, expose a clearer copyable field, or make
ambiguous-ID errors guide users back to the exact history entries they need to
choose between.

GA blocker: No.

## Provenance After Restore

Problem: after restoring an older save point, the newest save point in history
and the current file source can intentionally differ. The JSON fields are
accurate, but the concept is subtle.

Why it affects user mind: users may read "newest" as "what my files currently
match." If that assumption is wrong, they can misunderstand what a later save
will mean.

Possible improvement direction: improve status/history presentation so the
relationship between history head, current file source, and the next save is
visible in a compact form.

GA blocker: No.

## Save Point Comparison

Problem: JVS can open a read-only view of a save point, but it does not provide
a first-class way to compare two save points or compare a viewed file with the
current workspace file. Users currently have to open a view and choose an
external comparison tool, image viewer, or editor themselves.

Why it affects user mind: comparison is often the step where users decide
whether a restore is safe. If JVS makes the saved states visible but leaves the
comparison context implicit, users may be unsure which two files or folders
they are judging.

Possible improvement direction: explore a public comparison workflow that keeps
the two compared states explicit, supports both whole-folder and path-focused
questions, and can hand off to external tools without making users reconstruct
the source and target paths by memory.

GA blocker: No.

## Cleanup Review Confidence

Problem: cleanup preview safely separates protected and reclaimable save point
storage, but users still have to infer why each reclaimable item is safe to
remove.

Why it affects user mind: deletion review is a high-trust moment. Counts and
groups are useful, but users may want a concise explanation that connects each
reclaimable item to the absence of current protections.

Possible improvement direction: add an optional compact explanation for
reclaimable items, using the same stable protection reasons already shown for
protected save points.

GA blocker: No.

## Reproducible Tutorial Fixtures

Problem: scenario tutorials can show realistic workflows, but they often assume
the reader already has project files, scripts, or tools to run. The docs can
tell users to substitute their own tools, but that still leaves the tutorial
partly non-copyable.

Why it affects user mind: onboarding is easiest when every command can be run
as written. If a tutorial depends on local files that do not exist yet, users
may wonder whether a failure is their setup, the example, or JVS.

Possible improvement direction: explore an official small sample project or
tutorial fixture that exercises common filesystem version-control flows without
depending on a domain-specific toolchain.

GA blocker: No.
