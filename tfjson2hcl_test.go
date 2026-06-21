package tfjson2hcl_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/winebarrel/tfjson2hcl"
)

func TestConvert_Golden(t *testing.T) {
	cases := []string{
		"basic-policy",
		"nested-block",
		"multiple",
		"indented",
		"unwrap-interp",
		"not-json",
		"directive-in-json",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			tmp := copyInputToTemp(t, filepath.Join("testdata", name, "input"))
			c := tfjson2hcl.NewConverter(tmp)
			require.NoError(t, c.Convert(true))
			compareDir(t, tmp, filepath.Join("testdata", name, "expected"))
		})
	}
}

func TestConvert_EscapeMode(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/escape-mode/input")
	c := tfjson2hcl.NewConverter(tmp)
	c.Escape = true
	require.NoError(t, c.Convert(true))
	compareDir(t, tmp, "testdata/escape-mode/expected")
}

func TestConvert_StdoutMode(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/basic-policy/input")
	var buf bytes.Buffer
	c := tfjson2hcl.NewConverter(tmp)
	c.Out = &buf
	require.NoError(t, c.Convert(false))
	assert.Contains(t, buf.String(), "jsonencode({")
	// stdout mode must not touch the file on disk.
	got, err := os.ReadFile(filepath.Join(tmp, "main.tf"))
	require.NoError(t, err)
	want, err := os.ReadFile("testdata/basic-policy/input/main.tf")
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func TestConvert_StdoutNoChange(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/not-json/input")
	var buf bytes.Buffer
	c := tfjson2hcl.NewConverter(tmp)
	c.Out = &buf
	require.NoError(t, c.Convert(false))
	assert.Empty(t, buf.String())
}

func TestConvert_StdoutWriteError(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/basic-policy/input")
	c := tfjson2hcl.NewConverter(tmp)
	c.Out = failingWriter{}
	require.Error(t, c.Convert(false))
}

func TestConvert_GlobError(t *testing.T) {
	c := tfjson2hcl.NewConverter("[invalid")
	err := c.Convert(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "glob")
}

func TestConvert_LoadReadError(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(tmp, "trap.tf"), 0o755))
	c := tfjson2hcl.NewConverter(tmp)
	require.Error(t, c.Convert(true))
}

func TestConvert_ParseError(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "bad.tf"), []byte("resource \"x\" {\n"), 0o644))
	c := tfjson2hcl.NewConverter(tmp)
	require.Error(t, c.Convert(true))
}

func TestConvert_InPlaceWriteError(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/basic-policy/input")
	require.NoError(t, os.Chmod(filepath.Join(tmp, "main.tf"), 0o444))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(tmp, "main.tf"), 0o644) })
	c := tfjson2hcl.NewConverter(tmp)
	require.Error(t, c.Convert(true))
}

// ----------------- unit tests for internal helpers -----------------

func TestHeredocContent(t *testing.T) {
	// Heredoc value.
	tokens := exprTokens(t, "x = <<EOF\n{\"a\": 1}\nEOF\n")
	got, ok := tfjson2hcl.HeredocContent(tokens)
	require.True(t, ok)
	assert.Equal(t, "{\"a\": 1}\n", got)

	// Quoted string is not a heredoc.
	_, ok = tfjson2hcl.HeredocContent(exprTokens(t, "x = \"plain\"\n"))
	assert.False(t, ok)

	// Heredoc with an interpolation is reconstructed verbatim.
	got, ok = tfjson2hcl.HeredocContent(exprTokens(t, "x = <<EOF\n{\"a\": \"${b}\"}\nEOF\n"))
	require.True(t, ok)
	assert.Equal(t, "{\"a\": \"${b}\"}\n", got)
}

func TestJSONToHCL(t *testing.T) {
	// NoEscape keeps interpolations.
	got, err := tfjson2hcl.JSONToHCL(`{"a":"${b}"}`, false)
	require.NoError(t, err)
	assert.Contains(t, got, "${b}")

	// Escape mode doubles the dollar sign.
	got, err = tfjson2hcl.JSONToHCL(`{"a":"${b}"}`, true)
	require.NoError(t, err)
	assert.Contains(t, got, "$${b}")

	// Invalid JSON errors.
	_, err = tfjson2hcl.JSONToHCL(`{not json}`, false)
	require.Error(t, err)
}

func TestUnwrapSoleInterp(t *testing.T) {
	// key: expression source -> expected formatted output after unwrapping.
	cases := map[string]string{
		`"${var.a}"`:            "var.a",                 // sole interpolation: unwrapped
		`"${var.a}xxx${var.b}"`: `"${var.a}xxx${var.b}"`, // literal between interps: kept
		`"${var.a}${var.b}"`:    `"${var.a}${var.b}"`,    // two interps, no literal: kept
		`"pre-${var.a}"`:        `"pre-${var.a}"`,        // leading literal: kept
		`"${var.a}-suf"`:        `"${var.a}-suf"`,        // trailing literal: kept
		`"plain"`:               `"plain"`,               // no interpolation: kept
		`"${lookup(m, "k")}"`:   `lookup(m, "k")`,        // nested quotes inside: unwrapped
	}
	for src, want := range cases {
		t.Run(src, func(t *testing.T) {
			f, diags := hclwrite.ParseConfig([]byte("x = "+src+"\n"), "t.tf", hcl.Pos{Line: 1, Column: 1})
			require.False(t, diags.HasErrors())
			tokens := f.Body().GetAttribute("x").Expr().BuildTokens(nil)
			got := string(hclwrite.Format(tfjson2hcl.UnwrapSoleInterp(tokens).Bytes()))
			assert.Equal(t, want, got)
		})
	}
}

func TestConvertBody_NotJSONSkipped(t *testing.T) {
	src := []byte("x = <<EOF\nnot json\nEOF\n")
	f, diags := hclwrite.ParseConfig(src, "t.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	assert.False(t, tfjson2hcl.ConvertBody(f.Body(), false))
	assert.Equal(t, string(src), string(f.Bytes()))
}

func TestHeredocContent_NoCloser(t *testing.T) {
	// A heredoc opener with no closing marker is reported as "not a heredoc".
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOHeredoc, Bytes: []byte("<<EOF\n")},
		{Type: hclsyntax.TokenStringLit, Bytes: []byte("{}\n")},
	}
	_, ok := tfjson2hcl.HeredocContent(tokens)
	assert.False(t, ok)
}

func TestJSONencodeTokens_BadHCL(t *testing.T) {
	_, err := tfjson2hcl.JSONencodeTokens(`{ a = "${" }`)
	require.Error(t, err)
}

func TestUnwrapSoleInterp_UnterminatedInterp(t *testing.T) {
	// An OQuote immediately followed by ${ but with no closing } must not be
	// unwrapped; the tokens pass through unchanged.
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOQuote, Bytes: []byte(`"`)},
		{Type: hclsyntax.TokenTemplateInterp, Bytes: []byte("${")},
		{Type: hclsyntax.TokenIdent, Bytes: []byte("x")},
		{Type: hclsyntax.TokenCQuote, Bytes: []byte(`"`)},
	}
	out := tfjson2hcl.UnwrapSoleInterp(tokens)
	assert.Equal(t, len(tokens), len(out))
	assert.Equal(t, hclsyntax.TokenOQuote, out[0].Type)
}

// ----------------- helpers -----------------

func exprTokens(t *testing.T, src string) hclwrite.Tokens {
	t.Helper()
	f, diags := hclwrite.ParseConfig([]byte(src), "t.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	return f.Body().GetAttribute("x").Expr().BuildTokens(nil)
}

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) { return 0, errors.New("boom") }

func copyInputToTemp(t *testing.T, srcDir string) string {
	t.Helper()
	tmp := t.TempDir()
	entries, err := os.ReadDir(srcDir)
	require.NoError(t, err)
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, ent.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(tmp, ent.Name()), data, 0o644))
	}
	return tmp
}

func compareDir(t *testing.T, gotDir, wantDir string) {
	t.Helper()
	entries, err := os.ReadDir(wantDir)
	require.NoError(t, err)
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		got, err := os.ReadFile(filepath.Join(gotDir, ent.Name()))
		require.NoError(t, err)
		want, err := os.ReadFile(filepath.Join(wantDir, ent.Name()))
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), "file %s", ent.Name())
	}
}
