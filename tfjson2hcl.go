package tfjson2hcl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/winebarrel/json2hcl"
)

// Converter rewrites heredoc-embedded JSON in *.tf files into jsonencode({...})
// expressions. Heredocs whose content is not valid JSON are left untouched.
type Converter struct {
	Dir string
	Out io.Writer

	// Escape, when true, escapes ${...} and %{...} sequences to $${...} / %%{...}
	// so they are treated as literal text. By default they are preserved as
	// Terraform template interpolations/directives, matching the behaviour of the
	// original heredoc.
	Escape bool

	files map[string]*hclwrite.File
}

func NewConverter(dir string) *Converter {
	return &Converter{
		Dir:   dir,
		Out:   os.Stdout,
		files: map[string]*hclwrite.File{},
	}
}

func (c *Converter) Convert(inPlace bool) error {
	if err := c.load(); err != nil {
		return err
	}
	changed := map[string]bool{}
	for _, path := range sortedKeys(c.files) {
		if convertBody(c.files[path].Body(), c.Escape) {
			changed[path] = true
		}
	}
	return c.writeOut(inPlace, changed)
}

func (c *Converter) load() error {
	pattern := filepath.Join(c.Dir, "*.tf")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob: %w", err)
	}
	sort.Strings(matches)
	var diags hcl.Diagnostics
	for _, path := range matches {
		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		f, parseDiags := hclwrite.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
		if parseDiags.HasErrors() {
			diags = append(diags, parseDiags...)
			continue
		}
		c.files[path] = f
	}
	if diags.HasErrors() {
		return diags
	}
	return nil
}

// convertBody walks a body recursively and rewrites every attribute whose value
// is a heredoc holding valid JSON. It returns whether anything changed. A
// heredoc that is not valid JSON, or whose conversion does not round-trip back
// into a parseable expression, is left untouched.
func convertBody(body *hclwrite.Body, escape bool) bool {
	changed := false
	for name, attr := range body.Attributes() {
		jsonSrc, ok := heredocContent(attr.Expr().BuildTokens(nil))
		if !ok {
			continue
		}
		hclSrc, err := jsonToHCL(jsonSrc, escape)
		if err != nil {
			continue // not valid JSON
		}
		repl, err := jsonencodeTokens(hclSrc)
		if err != nil {
			continue // converted HCL did not round-trip (e.g. a bare "${" string)
		}
		body.SetAttributeRaw(name, repl)
		changed = true
	}
	for _, blk := range body.Blocks() {
		if convertBody(blk.Body(), escape) {
			changed = true
		}
	}
	return changed
}

// heredocContent returns the raw text between a heredoc's opener and its closing
// marker. The second return value is false when the expression is not a heredoc.
// Template interpolations are reconstructed verbatim, so ${...} sequences inside
// the JSON survive for json2hcl to handle.
func heredocContent(tokens hclwrite.Tokens) (string, bool) {
	if len(tokens) < 2 || tokens[0].Type != hclsyntax.TokenOHeredoc {
		return "", false
	}
	var sb strings.Builder
	for _, t := range tokens[1:] {
		if t.Type == hclsyntax.TokenCHeredoc {
			return sb.String(), true
		}
		sb.Write(t.Bytes)
	}
	// No closing marker found; treat as not a heredoc rather than guess.
	return "", false
}

// jsonToHCL converts a JSON document into the body of an HCL object expression
// using json2hcl.
func jsonToHCL(jsonSrc string, escape bool) (string, error) {
	var opts []json2hcl.Option
	if !escape {
		opts = append(opts, json2hcl.NoEscape())
	}
	return json2hcl.UnmarshalString(jsonSrc, opts...)
}

// jsonencodeTokens wraps an HCL expression in jsonencode(...) and returns its
// tokens. The snippet is round-tripped through the parser so the result is
// correctly tokenised and hclwrite can re-indent it in context.
func jsonencodeTokens(hclSrc string) (hclwrite.Tokens, error) {
	src := []byte("__tfjson2hcl__ = jsonencode(" + hclSrc + ")\n")
	f, diags := hclwrite.ParseConfig(src, "<jsonencode>", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, diags
	}
	tokens := f.Body().GetAttribute("__tfjson2hcl__").Expr().BuildTokens(nil)
	return unwrapSoleInterp(tokens), nil
}

// unwrapSoleInterp rewrites string values that consist of nothing but a single
// "${...}" interpolation into the bare expression, e.g. "${foo.bar}" -> foo.bar.
// A string with any literal text around the interpolation ("${x}/*") is left
// untouched. This mirrors `terraform fmt`'s removal of redundant interpolation.
func unwrapSoleInterp(tokens hclwrite.Tokens) hclwrite.Tokens {
	out := make(hclwrite.Tokens, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Type == hclsyntax.TokenOQuote &&
			i+1 < len(tokens) && tokens[i+1].Type == hclsyntax.TokenTemplateInterp {
			if j, ok := matchInterpEnd(tokens, i+1); ok &&
				j+1 < len(tokens) && tokens[j+1].Type == hclsyntax.TokenCQuote {
				// The whole quoted string is one interpolation: keep only the
				// expression between ${ and }, recursing to unwrap nested cases.
				out = append(out, unwrapSoleInterp(tokens[i+2:j])...)
				i = j + 1 // skip the trailing close quote
				continue
			}
		}
		out = append(out, tokens[i])
	}
	return out
}

// matchInterpEnd returns the index of the TokenTemplateSeqEnd that closes the
// interpolation opened at start (which must be a TokenTemplateInterp), handling
// nested interpolations/directives.
func matchInterpEnd(tokens hclwrite.Tokens, start int) (int, bool) {
	depth := 0
	for k := start; k < len(tokens); k++ {
		switch tokens[k].Type {
		case hclsyntax.TokenTemplateInterp, hclsyntax.TokenTemplateControl:
			depth++
		case hclsyntax.TokenTemplateSeqEnd:
			depth--
			if depth == 0 {
				return k, true
			}
		}
	}
	return 0, false
}

func (c *Converter) writeOut(inPlace bool, changed map[string]bool) error {
	for _, path := range sortedKeys(c.files) {
		if !changed[path] {
			continue
		}
		body := c.files[path].Bytes()
		if !inPlace {
			if _, err := fmt.Fprintf(c.Out, "### %s ###\n%s", path, body); err != nil {
				return err
			}
			continue
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
