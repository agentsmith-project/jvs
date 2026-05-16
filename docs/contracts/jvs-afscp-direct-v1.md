# JVS AFSCP Direct Contract v1

Status: pre-GA internal contract
Contract id: `jvs.afscp.direct.v1`

This contract is the internal JVS surface for trusted AFSCP callers. It only
covers whole-HOME save point operations for the direct fast path. It is not a
public user CLI surface.

## Commands

All commands require `--json` and the paired selector `--control-root` plus
`--home`. `workspace main` is an internal binding for v1 and is not exposed as
an argv field.

```bash
jvs afscp --control-root <control_root_path> --home <payload_home_path> save --message <message> --json
jvs afscp save --control-root <control_root_path> --home <payload_home_path> --message <message> --json
jvs afscp --control-root <control_root_path> --home <payload_home_path> list --json
jvs afscp --control-root <control_root_path> --home <payload_home_path> restore --save-point <save_point_id> --json
jvs afscp --control-root <control_root_path> --home <payload_home_path> status --json
jvs afscp --control-root <control_root_path> --home <payload_home_path> doctor --json
```

## Selector Validation

JVS must fail closed before any operation when:

- `--control-root` and `--home` are not both present.
- Either selector is not a clean absolute path after JVS normalization.
- Either selector cannot be opened as an existing directory.
- The canonical selectors are equal or one contains the other.
- `<payload_home_path>/.jvs` exists as any filesystem entry.
- Once direct metadata is initialized, `workspace main` in the control root is
  bound to exactly one canonical HOME; a mismatch is `JVS_METADATA_INVALID`.

Direct JSON must not echo HOME, control root, raw argv, JuiceFS internal paths,
or any internal path.

## JSON Envelope

Every direct response is one JSON object:

```json
{
  "contract": "jvs.afscp.direct.v1",
  "command": "status",
  "ok": true,
  "status": "succeeded",
  "data": {},
  "error": null
}
```

Required fields:

- `contract`: always `jvs.afscp.direct.v1`.
- `command`: one of `save`, `list`, `restore`, `status`, `doctor`.
- `ok`: boolean success marker.
- `status`: operation status enum.
- `data`: command result object on success, `null` on failure.
- `error`: direct error object on failure, `null` on success.

The direct schema must not include legacy active-contract fields:
`expected_folder_evidence`, `payload_root_hash`, `content_root_hash`, or
`save_profile`.

## Status Enum

- `accepted`
- `running`
- `succeeded`
- `failed`
- `recovery_required`

## Success Data

`save` success data:

- `save_point_id`: created save point id.
- `created_at`: RFC3339 timestamp.
- `message`: operator message.
- `history_head`: save point id that is now the head.

`list` success data:

- `history_head`: current save point id or `null`.
- `save_points`: array of save point records with `save_point_id`,
  optional `created_at`, optional `message`, and optional `history_head`.
- `metadata_state`: operator-safe metadata state.

`restore` success data:

- `restored_save_point_id`: restored save point id.
- `previous_head`: previous history head or `null`.
- `new_head`: new history head or `null`.

`status` success data:

- `repo_id`: control-root repository id string, or `""` when the control root
  has not been initialized by `jvs init`.
- `history_head`: current save point id or `null`.
- `active_operation`: stable operation string. Uses `none` when no operation is
  active; otherwise one of `running`, `failed`, or `recovery_required`.
- `metadata_state`: operator-safe metadata state.
- `recovery`: stable recovery string. Uses `none` when no recovery action is
  projected; otherwise one of `repair_metadata`, `recover_journal`, or
  `cleanup_pending`.

`doctor` success data:

- `healthy`: boolean metadata-health summary.
- `repo_id`: control-root repository id string, or `""` when the control root
  has not been initialized by `jvs init`.
- `findings`: array of operator-safe findings.
- `metadata_state`: operator-safe metadata state.
- `journal`: stable journal string. Uses `clean` when no journal action is
  pending, `invalid` when journal metadata is malformed, or the safe direct
  journal phase such as `save_failed` / `restore_replacing`.
- `recovery`: stable recovery string. Uses `none` when no recovery action is
  projected; otherwise one of `repair_metadata`, `recover_journal`, or
  `cleanup_pending`.

When direct metadata has not yet been initialized under
`<control-root>/afscp-direct-v1`, `status` and `doctor` still return the full
stable shape with `metadata_state=uninitialized`, `active_operation=none`,
`journal=clean`, `recovery=none`, and the control-root `repo_id` when present.

## Error Object

Failure responses use:

```json
{
  "code": "JVS_INVALID_ARGUMENT",
  "message": "operator-safe message",
  "retryable": false
}
```

Stable error codes:

- `JVS_INVALID_ARGUMENT`: exit `2`.
- `JVS_SAVE_POINT_NOT_FOUND`: exit `2`.
- `JVS_METADATA_INVALID`: exit `3`.
- `JVS_JOURNAL_RECOVERY_REQUIRED`: exit `3`.
- `JVS_LOCKED`: exit `4`.
- `JVS_CLONE_UNAVAILABLE`: exit `5`.
- `JVS_CLONE_FAILED`: exit `5`.
- `JVS_INTERNAL`: exit `1`.

## Exit Codes

- `0`: command succeeded and JSON result is complete.
- `1`: internal error or unexpected failure.
- `2`: invalid argument or invalid invocation.
- `3`: metadata invalid or recovery required.
- `4`: lock busy or retryable concurrent mutation.
- `5`: JuiceFS clone or storage operation failure.

AFSCP mapping:

- `0`: operation succeeded or accepted projection from JSON status.
- `2`: terminal failed with validation error.
- `3`: `recovery_required` or terminal failed when recovery is not possible.
- `4`: retryable operation failure or in-progress conflict.
- `5`: operation failure with clone or storage reason.
- Malformed JSON: terminal failed with JVS runner diagnostic.

## Current Implementation Boundary

The direct v1 path owns a metadata-only layout under
`<control-root>/afscp-direct-v1`. The layout binds `workspace main` to exactly
one canonical `--home`; JVS metadata is never written under HOME, and
`<home>/.jvs` remains fail-closed.

Descriptor, ready marker, and journal files carry metadata-only SHA-256
checksums. The checksum input is the JVS metadata object with its own checksum
field omitted. It never reads, hashes, samples, walks, or summarizes HOME or
snapshot payload content. A descriptor / ready / journal checksum mismatch is
metadata invalid and fail-closed for mutations. `status` reports
`metadata_state=invalid` plus `recovery=repair_metadata`; `doctor` reports an
operator-safe finding.

`save` acquires the direct mutation lock under the control-root metadata layout
before mutating direct metadata. Lock contention returns `JVS_LOCKED` / exit
`4` and is retryable. Save writes a `save_intent` journal and then a
`save_running` journal before invoking clone. It uses only strict
`juicefs clone <home> <tmp-payload>`. On success it atomically publishes the
payload to
`<control-root>/afscp-direct-v1/snapshots/<save_point_id>/payload`, then writes
checksummed descriptor, checksummed ready marker, history, and an idle journal
outside the payload. If `juicefs` is unavailable or clone fails, direct save
returns `JVS_CLONE_UNAVAILABLE` or `JVS_CLONE_FAILED`, records a
`save_failed` journal, and does not fall back to copy.

`list` reads history plus descriptors and returns `history_head` and
`save_points`. `status` and `doctor` are metadata-only: they read selector
binding, control-root `repo_id`, and direct metadata projections only. They
validate history-referenced descriptor / ready checksums and journal checksum,
but do not require HOME to exist and do not walk HOME or payload, hash content,
perform capacity scans, fsync trees, compress, or invoke legacy public
save/restore paths.

`restore` acquires the same direct mutation lock before mutating metadata or
HOME contents. It uses only strict
`juicefs clone <snapshot-payload> <home-tmp-payload>` into a randomly named
restore staging directory inside HOME. It validates binding, history,
checksummed descriptor, checksummed ready marker, snapshot payload, and direct
journal state before HOME mutation. A stale non-idle journal returns
`JVS_JOURNAL_RECOVERY_REQUIRED`.

Restore updates `history_head`/`new_head` to the restored save point after the
replacement succeeds. Before entering the replace boundary, restore renames the
current HOME top-level entries into
`<control-root>/afscp-direct-v1/pending-cleanups/<cleanup_id>/payload`. This
backup location is outside HOME, so later saves do not capture old HOME content
as payload. Failures during this backup phase are rolled back by renaming
backed-up entries into HOME again, cleaning staging, and marking the journal
idle. After the replace boundary starts, failures leave the backup / staging
journal referenced and return `JVS_JOURNAL_RECOVERY_REQUIRED` so an
operator-safe recovery path can inspect or repair state.

On restore success, JVS does not recursively delete the old HOME backup in the
save / restore hot path. The pending cleanup remains metadata-referenced under
the control root. `status` reports `recovery=cleanup_pending`, and `doctor`
reports a warning finding without exposing the internal cleanup path.
Out-of-band cleanup / GC must remove these pending cleanups; save / restore
must not perform recursive backup cleanup, payload sync, capacity pre-scan,
content digesting, compression, or copy fallback. Restore preserves the HOME
root directory identity.
