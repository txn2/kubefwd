package hostess

import (
	"flag"
	"os"
	"testing"

	"github.com/codegangsta/cli"
	"github.com/stretchr/testify/assert"
)

func TestStrPadRight(t *testing.T) {
	assert.Equal(t, "", StrPadRight("", 0), "Zero-length no padding")
	assert.Equal(t, "          ", StrPadRight("", 10), "Zero-length 10 padding")
	assert.Equal(t, "string", StrPadRight("string", 0), "6-length 0 padding")
}

func TestLs(t *testing.T) {
	os.Setenv("HOSTESS_PATH", "test-fixtures/hostfile1")
	defer os.Setenv("HOSTESS_PATH", "")
	c := cli.NewContext(cli.NewApp(), &flag.FlagSet{}, nil)
	Ls(c)
}
