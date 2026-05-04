// Package jvs provides a high-level Go facade for JVS file-system version control.
//
// It wraps internal packages into a clean, stable public API for applications
// that need to save, inspect, restore, and clean up versioned folder state.
//
// # Concurrency Safety
//
// JVS operations are filesystem-based and follow these concurrency rules:
//
//   - Save() is safe when no concurrent writes to the workspace folder.
//     Quiesce writers before saving a folder state.
//
//   - Restore() is safe when no concurrent reads from the workspace folder.
//     Quiesce readers and writers before restoring a folder state.
//
//   - Multiple Client instances for DIFFERENT repositories are fully independent
//     and safe to use concurrently.
//
//   - Multiple Client instances for the SAME repository must NOT call
//     mutating operations (Save, Restore, PreviewCleanup, RunCleanup)
//     concurrently.
//
// # Recommended Usage Pattern
//
//	// Open or initialize a managed folder.
//	client, err := jvs.OpenOrInit(repoPath, jvs.InitOptions{Name: "workspace"})
//	workspacePath := client.WorkspacePath("main")
//
//	// Restore a chosen save point before using the folder.
//	if latest, _ := client.LatestSavePoint(ctx, "main"); latest != nil {
//	    client.Restore(ctx, jvs.RestoreOptions{Target: latest.SavePointID.String()})
//	}
//
//	// Use workspacePath as the application content folder, then save completed work.
//	client.Save(ctx, jvs.SaveOptions{
//	    Message: "completed workspace update",
//	})
package jvs
