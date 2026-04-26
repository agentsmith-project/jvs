package jsonutil_test

import (
	"testing"

	"github.com/agentsmith-project/jvs/pkg/jsonutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalMarshal_SortedKeys(t *testing.T) {
	input := map[string]any{
		"zebra": 1,
		"alpha": 2,
		"mid":   3,
	}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	assert.Equal(t, `{"alpha":2,"mid":3,"zebra":1}`, string(out))
}

func TestCanonicalMarshal_Nested(t *testing.T) {
	input := map[string]any{
		"b": map[string]any{"z": 1, "a": 2},
		"a": 0,
	}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	assert.Equal(t, `{"a":0,"b":{"a":2,"z":1}}`, string(out))
}

func TestCanonicalMarshal_NullValue(t *testing.T) {
	input := map[string]any{"key": nil}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	assert.Equal(t, `{"key":null}`, string(out))
}

func TestCanonicalMarshal_NoWhitespace(t *testing.T) {
	input := map[string]any{"a": []any{1, 2, 3}}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	assert.Equal(t, `{"a":[1,2,3]}`, string(out))
}

func TestCanonicalMarshal_Unicode(t *testing.T) {
	input := map[string]any{"名前": "テスト"}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	assert.Contains(t, string(out), "名前")
}

func TestCanonicalMarshal_StructSortsFields(t *testing.T) {
	type sample struct {
		Zebra int    `json:"zebra"`
		Alpha string `json:"alpha"`
	}
	input := sample{Zebra: 1, Alpha: "a"}
	out, err := jsonutil.CanonicalMarshal(input)
	require.NoError(t, err)
	// Keys must be sorted alphabetically regardless of struct field order
	assert.Equal(t, `{"alpha":"a","zebra":1}`, string(out))
}

func TestCanonicalMarshal_Deterministic(t *testing.T) {
	input := map[string]any{"c": 3, "a": 1, "b": 2}
	out1, _ := jsonutil.CanonicalMarshal(input)
	out2, _ := jsonutil.CanonicalMarshal(input)
	assert.Equal(t, string(out1), string(out2))
}
