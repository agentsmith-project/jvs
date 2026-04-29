package jvs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewCleanupExposesProtectionGroups(t *testing.T) {
	dir := t.TempDir()
	client, err := Init(dir, InitOptions{Name: "client-test", EngineType: model.EngineCopy})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.txt"), []byte("baseline"), 0644))

	savePoint, err := client.Save(context.Background(), SaveOptions{Message: "baseline"})
	require.NoError(t, err)

	plan, err := client.PreviewCleanup(context.Background(), CleanupOptions{})
	require.NoError(t, err)

	group := clientProtectionGroupByReason(plan.ProtectionGroups, "history")
	require.NotNil(t, group)
	assert.Equal(t, 1, group.Count)
	assert.Equal(t, []SavePointID{savePoint.SavePointID}, group.SavePoints)
}

func clientProtectionGroupByReason(groups []CleanupProtectionGroup, reason string) *CleanupProtectionGroup {
	for i := range groups {
		if groups[i].Reason == reason {
			return &groups[i]
		}
	}
	return nil
}
