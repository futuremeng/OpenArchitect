package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/update"
)

// updateChecker is the seam tests repoint at a fake GitHub; production
// always gets update.Default.
var updateChecker = &update.Default

// runUpdate closes the release loop: it asks GitHub Releases for the newest
// version and, unless --check was given, replaces the running binary with
// the official build (checksum-verified, atomic swap). User-triggered only;
// nothing automated ever calls it (I2′).
func runUpdate(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(out)
	checkOnly := fs.Bool("check", false, "只查询，不安装")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			fmt.Fprint(out, "usage: oac update [--check]\n")
			return nil
		}
		return err
	}
	rel, err := updateChecker.Latest(ctx)
	if err != nil {
		return err
	}
	if !update.Newer(rel.Version(), Version) {
		fmt.Fprintf(out, "oac %s — 已是最新\n", Version)
		return nil
	}
	fmt.Fprintf(out, "发现新版本: oac %s -> %s (%s)\n", Version, rel.Version(), rel.TagName)
	if *checkOnly {
		fmt.Fprintln(out, "运行 `oac update` 安装")
		return nil
	}
	if err := updateChecker.Apply(ctx, rel, update.ApplyOptions{}); err != nil {
		return err
	}
	fmt.Fprintf(out, "已安装 %s，重启后生效\n", rel.Version())
	return nil
}
