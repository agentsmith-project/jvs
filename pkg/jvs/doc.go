// Package jvs provides a high-level library API for JVS (Juicy Versioned Workspaces).
//
// This package is the primary integration point for external consumers such as
// sandbox-manager. It wraps internal packages into a clean, stable public API.
//
// # Concurrency Safety
//
// JVS operations are filesystem-based and follow these concurrency rules:
//
//   - Save() is safe when no concurrent writes to the payload directory.
//     Always save AFTER the agent pod has been deleted (process stopped).
//
//   - Restore() is safe when no concurrent reads from the payload directory.
//     Always restore BEFORE the agent pod is created.
//
//   - Multiple Client instances for DIFFERENT repositories are fully independent
//     and safe to use concurrently.
//
//   - Multiple Client instances for the SAME repository must NOT call
//     mutating operations (Save, Restore, PreviewCleanup, RunCleanup)
//     concurrently.
//
// # Recommended Usage Pattern (sandbox-manager)
//
//	// Pod startup: restore workspace before creating pod
//	client, err := jvs.OpenOrInit(repoPath, jvs.InitOptions{Name: "agent-ws"})
//	payloadPath := client.WorkspacePath("main")
//	if latest, _ := client.LatestSavePoint(ctx, "main"); latest != nil {
//	    client.Restore(ctx, jvs.RestoreOptions{Target: latest.SavePointID.String()})
//	}
//	// Mount payloadPath as /workspace in pod via JuiceFS subPath
//
//	// Pod shutdown: save after pod is deleted
//	client.Save(ctx, jvs.SaveOptions{
//	    Message: "auto: pod shutdown",
//	    Tags: []string{"auto", "shutdown"},
//	})
package jvs
