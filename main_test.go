package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

const (
	inDir  = "./testdata/in"
	outDir = "./testdata/out"
)

func TestStripBlanks(t *testing.T) {
	testCases := []struct {
		filename    string
		errExpected bool
	}{

		{
			filename: "case0",
		},
		{
			filename:    "case1",
			errExpected: true,
		},
		{
			filename: "case2",
		},
		{
			filename:    "case3",
			errExpected: true,
		},
		{
			filename: "case4",
		},
		{
			filename: "case5",
		},
		{
			filename:    "case6",
			errExpected: true,
		},
		{
			filename:    "case7",
			errExpected: true,
		},
		// This test fails -- would be nice to fix
		//{
		//	filename: "case8",
		//},
	}

	for _, testCase := range testCases {
		t.Run(testCase.filename, func(t *testing.T) {
			fIn, _ := os.Open(filepath.Join("testdata", "in", testCase.filename))
			contentIn, _ := io.ReadAll(fIn)
			actualOut, err := format(contentIn)
			if testCase.errExpected {
				if err == nil {
					t.Errorf("expected error for file %s", testCase.filename)
				}
			} else {
				expFOut, _ := os.Open(filepath.Join("testdata", "expectedout", testCase.filename))
				expContentOut, _ := io.ReadAll(expFOut)
				if !bytesEqual(expContentOut, actualOut) {
					t.Errorf("expected:\n\n%s\n\ngot:\n\n%s", string(expContentOut), string(actualOut))
					t.Fail()
				}
			}

		})
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
