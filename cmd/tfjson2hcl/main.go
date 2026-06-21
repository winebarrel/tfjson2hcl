package main

import (
	"log"
	"os"

	"github.com/alecthomas/kong"
	"github.com/winebarrel/tfjson2hcl"
)

var version string

func init() {
	log.SetFlags(0)
}

type options struct {
	Dir     string `arg:"" optional:"" default:"." help:"Directory containing *.tf files (default: \".\")."`
	InPlace bool   `short:"i" help:"Write changes back to files instead of stdout."`
	Escape  bool   `short:"e" help:"Escape ${...} and %{...} to $${...} / %%{...} instead of keeping them as template interpolations."`
	Version kong.VersionFlag
}

func parseArgs() *options {
	opts := &options{}
	parser := kong.Must(opts,
		kong.Name("tfjson2hcl"),
		kong.Description("Convert heredoc JSON in *.tf files into jsonencode({...}) expressions."),
		kong.Vars{"version": version},
	)
	parser.Model.HelpFlag.Help = "Show help."
	if _, err := parser.Parse(os.Args[1:]); err != nil {
		parser.FatalIfErrorf(err)
	}
	return opts
}

func main() {
	opts := parseArgs()
	c := tfjson2hcl.NewConverter(opts.Dir)
	c.Escape = opts.Escape
	if err := c.Convert(opts.InPlace); err != nil {
		log.Fatalf("error: %v", err)
	}
}
