# JVS Quickstart: Game Development Folders

**Status:** Release-facing domain entry

Use this page as a short domain guide. The full user workflow is in
[docs/user/examples.md](user/examples.md).

## Why Game Teams Use JVS

Game projects contain large binary assets, generated editor state, and build
outputs. JVS is useful around risky asset work because it saves the folder,
lets you inspect earlier saves, and restores with a preview before files are
changed.

JVS does not provide editor locking, asset merge, source hosting, or a Unity or
Unreal plugin. Keep using your normal source control and team coordination
tools for those jobs.

## Setup

Create a folder that contains the files you want JVS to manage:

```bash
mkdir mygame-assets
cd mygame-assets
jvs init

cp -r ~/UnityProjects/MyGame/Assets .
cp -r ~/UnityProjects/MyGame/ProjectSettings .
jvs save -m "initial Unity asset import"
jvs history
```

For Unreal, copy `Content/` and `Config/` instead:

```bash
cp -r ~/UnrealProjects/MyGame/Content .
cp -r ~/UnrealProjects/MyGame/Config .
jvs save -m "initial Unreal asset import"
```

Keep generated caches and editor-local files outside the managed folder when
you do not want them saved.

## Daily Asset Work

```bash
jvs save -m "before character model work"

# Work in Unity or Unreal.

jvs save -m "character model armor pass"
jvs history --grep "character"
```

Before replacing the folder with an earlier save, inspect it:

```bash
jvs view <save> Assets/Characters/Hero
jvs view close <view-id>
```

Then preview and run restore:

```bash
jvs restore <save> --discard-unsaved
jvs restore --run <plan-id>
```

## Recover One Asset

For one file or directory:

```bash
jvs restore --path Assets/Characters/Hero
jvs restore <save> --path Assets/Characters/Hero
jvs restore --run <plan-id>
```

This restores only that managed path and keeps history unchanged.

## Try A Variant In Another Folder

Create another real folder from a save point:

```bash
jvs workspace new hero-variant --from <save>
```

JVS prints the new folder path:

```bash
cd <printed-folder>
# Work on the variant.
jvs save -m "hero armored variant"
```

The original folder is unchanged.

## Build Script Hook

```bash
#!/usr/bin/env bash
set -euo pipefail

jvs save -m "pre-build ${BUILD_ID}"

./run-game-build.sh

jvs save -m "build ${BUILD_ID} complete"
```

If a build damages generated or managed files, restore from the pre-build save:

```bash
jvs restore <pre-build-save> --discard-unsaved
jvs restore --run <plan-id>
```

## Health And Recovery

```bash
jvs doctor
jvs doctor --strict
```

If a restore is interrupted:

```bash
jvs recovery status
jvs recovery resume <recovery-plan>
```

or:

```bash
jvs recovery rollback <recovery-plan>
```
