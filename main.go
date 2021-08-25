package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/scanner"
	"go/token"
	"log"
	"os"
	"os/exec"
)

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), "stripblankimports [flags] path [path...]\n")
	flag.PrintDefaults()
}

// For writeback:
//		For each file:
//			read file
//			stripblanks transform
//			write file
//		Goimports batch run with writeback
//
// For nonwriteback:
//		For each file:
//			read file
//			stripblanks transform
//			pass to goimports
//			Run goimports with stdin as output of previous transform
//

func main() {
	flag.Usage = usage
	local := flag.String("local", "", "local grouping flag to goimports")
	verbose := flag.Bool("v", false, "verbose logging")
	writeBack := flag.Bool("w", false, "write back to file")
	goimportsPath := flag.String("p", "goimports", "path to goimports executable")
	flag.Parse()

	filenames := flag.Args()

	if *writeBack {
		writeBackDriver(filenames, format, *local, *goimportsPath, *verbose)
	} else {
		stdOutDriver(filenames, stitchXform(format, goImportsStdIO(*local, *goimportsPath)), *verbose)
	}
}

type xformFn func([]byte) ([]byte, error)

func stitchXform(fns ...xformFn) xformFn {
	return func(content []byte) ([]byte, error) {
		for _, fn := range fns {
			contentOut, err := fn(content)
			if err != nil {
				return content, err
			}
			content = contentOut
		}
		return content, nil
	}
}

func stdOutDriver(filenames []string, xform xformFn, verbose bool) {
	for _, filename := range filenames {
		content, err := os.ReadFile(filename)
		if err != nil {
			if verbose {
				log.Printf("error reading file %s: %s", filename, err)
			}
			continue
		}

		contentOut, err := xform(content)
		if err != nil {
			if verbose {
				log.Printf("error processing file %s: %s", filename, err)
			}
			if contentOut == nil {
				continue
			}
		}

		fmt.Fprint(os.Stdout, string(contentOut))
	}
}

func writeBackDriver(filenames []string, xform xformFn, local, goimportsPath string, verbose bool) {
	for _, filename := range filenames {
		content, err := os.ReadFile(filename)
		if err != nil {
			if verbose {
				log.Printf("error reading file %s: %s", filename, err)
			}
			continue
		}

		contentOut, err := xform(content)
		if err != nil {
			if verbose {
				log.Printf("error processing file %s: %s", filename, err)
			}
			if contentOut == nil {
				continue
			}
		}

		err = os.WriteFile(filename, contentOut, 0644 /* this shouldn't have effect, since the file exists */)
		if err != nil && verbose {
			log.Printf("error writing file %s: %s", filename, err)
		}
	}

	if err := goImportsWriteBack(local, goimportsPath, filenames...); err != nil && verbose {
		log.Printf("goimports error: %s", err)
	}
}

// Read the file, do the formatting, and truncate and recreate it with the new
// content.
func fileIO(filename string, xform func([]byte) ([]byte, error)) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	out, err := xform(content)
	if err != nil {
		return fmt.Errorf("formatting: %w", err)
		// Todo: log
	}

	fw, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("truncate file: %w", err)
	}
	defer fw.Close()

	if _, err := fw.Write(out); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// goImports runs the local goimports command on the provided filenames.
// The local flag corresponds to goimports local flag. It's okay for it to be
// empty.
func goImportsWriteBack(local, goimportsPath string, fname ...string) error {
	cmd := exec.Command(
		goimportsPath,
		append([]string{"-local", local, "-w"}, fname...)...,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("goimports: %w", err)
	}
	return nil
}

func goImportsStdIO(local, goimportsPath string) xformFn {
	return func(content []byte) ([]byte, error) {
		cmd := exec.Command(
			goimportsPath,
			[]string{"-local", local}...,
		)
		cmd.Stdin = bytes.NewReader(content)
		out := bytes.Buffer{}
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("goimports: %w", err)
		}
		return out.Bytes(), nil
	}
}

// Format takes in a file's content and returns the same file's content with
// the blank lines in import blocks removed. Returns an error when it can't
// handle something -- to be logged & handled by calling code.
func format(content []byte) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	if len(file.Imports) <= 1 {
		// No point
		return nil, fmt.Errorf("doesn't contain multiple imports")
	}

	// Find the start and end position of the import statement so that we only
	// handle things in that range.
	impStart, impEnd, err := findImportBounds(fset.File(1), content, file.Imports)
	if err != nil {
		return nil, err
	}

	// Do it.
	squashBlankImportLines(fset, impStart, impEnd, file.Imports, file.Comments)

	buf := bytes.Buffer{}
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Find the IMPORT token, look for the first LPAREN after it, and the first
// RPAREN after that. That's the area we're looking for.
//
// Not handling cases of IMPORT followed by STRING, or multiple IMPORT tokens in
// a file.
func findImportBounds(
	file *token.File,
	content []byte,
	imp []*ast.ImportSpec,
) (start, end token.Pos, err error) {
	// At this point, we've verified that there are imports in the file, so we
	// don't risk scanning the whole file if there are no imports.
	var s scanner.Scanner
	s.Init(file, content, nil, scanner.ScanComments)
	var found bool
	for {
		pos, tok, _ := s.Scan()
		if found {
			if tok == token.STRING && start == 0 {
				// saw a quote before a paren, we don't do that
				return start, end, fmt.Errorf("non-block import")
			}
			if tok == token.LPAREN {
				start = pos
			} else if tok == token.RPAREN {
				end = pos
				return
			}
		} else {
			if tok == token.IMPORT {
				found = true
			}
		}
	}
}

// Let's reslice the comment slice so we only handle the relevant ones.
func resliceComments(posStart, posEnd token.Pos, cm []*ast.CommentGroup) []*ast.CommentGroup {
	cmLower, cmUpper := -1, -1
	for i := 0; i < len(cm); i++ {
		if cmLower == -1 && cm[i].Pos() > posStart && cm[i].Pos() < posEnd {
			cmLower = i
		}

		if cmLower != -1 && cm[i].Pos() < posEnd {
			cmUpper = i
		}

		if cm[i].Pos() > posEnd {
			break
		}
	}

	if cmLower == -1 {
		// There's no comments in the import block -- return empty comment slice
		return nil
	}

	return cm[cmLower : cmUpper+1]
}

func squashBlankImportLines(
	fset *token.FileSet,
	posStart, posEnd token.Pos,
	imp []*ast.ImportSpec,
	cm []*ast.CommentGroup,
) {
	if len(imp) < 2 {
		// Why bother?
		return
	}

	cm = resliceComments(posStart, posEnd, cm)

	// Merge two sorted lists
	impIdx, cmIdx := 0, 0
	for impIdx < len(imp) || cmIdx < len(cm) {
		curr := chooseNext(imp, cm, &impIdx, &cmIdx, true)
		if impIdx == len(imp) && cmIdx == len(cm) {
			// If we're at a point where the thing we're considering is the last
			// thing (i.e. both pointers point to the end of their respective
			// lists), we're done.
			break
		}

		// Not incrementing here because we want the item we choose for next
		// here to be curr in the next iteration.
		next := chooseNext(imp, cm, &impIdx, &cmIdx, false)

		// Take a couple steps to find the line number for the last line of curr
		// and the first line of next. Note that both ImportSpecs and
		// CommentGroups can be multiple lines.
		currEnd, nextStart := curr.End(), next.Pos()
		currFile, nextFile := fset.File(currEnd), fset.File(nextStart)
		if currFile != nextFile {
			panic("files unequal")
		}
		currEndLine, nextStartLine := currFile.Line(currEnd), currFile.Line(nextStart)

		// For each additional line over the 1 line of difference allowed,
		// sqwashit.
		for i := 0; i < nextStartLine-currEndLine-1; i++ {
			currFile.MergeLine(currEndLine)
		}
	}
}

// Pick the next element at the head of imp or cm based on whether either is
// already exhausted or whose head element has an earlier Pos().
//
// If inc is set, increment the index pointer for the chosen slice.
func chooseNext(imp []*ast.ImportSpec, cm []*ast.CommentGroup, impIdx, cmIdx *int, inc bool) ast.Node {
	var out ast.Node
	var incVar *int // Easy way to only check value of inc param once
	if *impIdx >= len(imp) {
		out = cm[*cmIdx]
		incVar = cmIdx
	} else if *cmIdx >= len(cm) {
		out = imp[*impIdx]
		incVar = impIdx
	} else if imp[*impIdx].Pos() < cm[*cmIdx].Pos() {
		out = imp[*impIdx]
		incVar = impIdx
	} else {
		out = cm[*cmIdx]
		incVar = cmIdx
	}
	if inc {
		*incVar++
	}
	return out
}
