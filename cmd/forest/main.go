package main

import (
	"github.com/alecthomas/kong"
	"github.com/svenjigrun/forest/internal/cmd"
)

// cli is the root kong grammar.
var cli struct {
	Init    initCmd    `cmd:"" help:"Initialise a new Forest directory."`
	Reindex reindexCmd `cmd:"" help:"Rebuild the DuckDB index from node files."`
}

type initCmd struct {
	Dir string `arg:"" optional:"" default:"." help:"Directory to initialise (default: current directory)."`
}

func (c *initCmd) Run() error {
	return cmd.Init(c.Dir)
}

type reindexCmd struct {
	Dir string `arg:"" optional:"" default:"." help:"Forest root directory (default: current directory)."`
}

func (c *reindexCmd) Run() error {
	return cmd.Reindex(c.Dir)
}

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("forest"),
		kong.Description("A pageless, context-driven information space."),
		kong.UsageOnError(),
	)
	ctx.FatalIfErrorf(ctx.Run())
}
