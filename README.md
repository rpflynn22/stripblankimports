# stripblankimports

A simple CLI tool that wraps goimports to better format import blocks.

## Usage

```
$ go get github.com/rpflynn22/stripblankimports
$ stripblankimports -h
stripblankimports [flags] path [path...]
  -local string
        local grouping flag to goimports
  -v    verbose logging stripblankimports [-v] [-local ]filename [filename ...]
```

It modifies the file directly.

## Why?

Goimports has an [issue](https://github.com/golang/go/issues/20818) where
imports are not grouped correctly when there are blank lines in the import
block. To fix this, stripblankimports preprocesses go files, removing all the
blank lines from the import block. Once this is done, it runs goimports (if it's
available in the $PATH).

## Couldn't it just be a simple sed command?

IDK, maybe. But it's fun to learn about the Go language packages.

## Short Example

Before:

```go
package main

import (
	"github.com/pkg/errors"

	"fmt"

	"os"
)
```

After running regular goimports:
```go
package main

import (
	"github.com/pkg/errors"

	"fmt"

	"os"
)
```

After running stripblankimports:

```go
package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
)
```

