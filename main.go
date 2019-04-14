package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	baseRef  = flag.String("base", "head", "git reference to compare against")
	exitCode = flag.Int("exit-code", 0, "exit code to use when failing")
	verbose  = flag.Bool("v", false, "enable verbose mode")
	vverbose = flag.Bool("vv", false, "print everything (not recommended)")
)

type checker struct {
	fset    *token.FileSet
	fsetold *token.FileSet
}

func main() {
	flag.Parse()
	if *vverbose {
		*verbose = true
	}
	stats, err := gitStats(*baseRef)
	if err != nil {
		log.Fatal(err)
	}
	pkgs := make(map[string]struct{})
	for _, stat := range stats {
		if stat.mode == modeAdded {
			// skip; file not in base
			continue
		}
		path := stat.oldPath
		if filepath.Ext(path) != ".go" {
			continue
		}
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		dir := removePrivatePathSegments(filepath.Dir(path))
		if len(dir) == 0 {
			// this was a root internal or vendor folder
			continue
		}
		if dir == "." {
			dir = "./"
		}
		pkgs[dir] = struct{}{}
	}

	var failed bool
	fsetHead := token.NewFileSet()
	fsetBase := token.NewFileSet()
	for dir := range pkgs {
		fd, err := os.Open(dir)
		if err != nil {
			// package was removed
			fmt.Printf("%s: package removed\n\n", dir)
			continue
		}
		list, err := fd.Readdir(-1)
		if err != nil {
			log.Fatal("fd.Readdir: ", err)
		}
		fd.Close()
		if *verbose {
			fmt.Println(dir)
		}

		// create working path summary
		summary := newPackageSummary(dir)
		for _, d := range list {
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				continue
			}
			fullpath := filepath.Join(dir, d.Name())
			src, err := parser.ParseFile(fsetHead, fullpath, nil, parser.DeclarationErrors)
			if err != nil {
				log.Fatal("parsefile: ", err)
			}
			if !ast.FileExports(src) {
				continue
			}
			ast.Walk(summary, src)
		}

		// scan base, everything found there should exist in working path
		comparer := newDeclComparer(summary)
		blobs, err := gitLsTreeGoBlobs(*baseRef, dir)
		if err != nil {
			log.Fatal(err)
		}
		for _, file := range blobs {
			fullpath := filepath.Join(dir, file)
			r, err := gitBlob(*baseRef, fullpath)
			if err != nil {
				log.Fatal(err)
			}
			src, err := parser.ParseFile(fsetBase, fullpath, r, parser.DeclarationErrors)
			if err != nil {
				log.Fatal(err)
			}
			if !ast.FileExports(src) {
				continue
			}
			ast.Walk(comparer, src)
		}
		if comparer.report.Len() > 0 {
			fmt.Println(comparer.report.String())
			failed = true
		}
	}
	if failed {
		os.Exit(*exitCode)
	}
}

func removePrivatePathSegments(dir string) string {
	if i := strings.Index(dir, "internal/"); i > -1 {
		dir = dir[:i]
	}
	if i := strings.Index(dir, "/internal"); i > -1 {
		dir = dir[:i]
	}
	if i := strings.Index(dir, "vendor/"); i > -1 {
		dir = dir[:i]
	}
	if i := strings.Index(dir, "/vendor"); i > -1 {
		dir = dir[:i]
	}
	return dir
}